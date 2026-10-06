package zhiji

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tmpDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "zhiji-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(tmpDir(t))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// ---- model.go ----

func TestRelevance(t *testing.T) {
	m := MemoryItem{Text: "沙特市场需要本地合作伙伴与运营商资质"}
	if r := m.Relevance("沙特 合作伙伴"); r <= 0 {
		t.Fatalf("期望相关度>0，got %v", r)
	}
	if r := m.Relevance(""); r != 0 {
		t.Fatalf("空 query 相关度应为 0，got %v", r)
	}
}

// ---- store.go ----

func TestUpsertSelfVersioned(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.UpsertSelf(SelfItem{Layer: LayerGoal, Text: "主目标：服务沙特客户", Confidence: 0.9})
	upd, err := s.UpsertSelf(SelfItem{Layer: LayerGoal, Text: "主目标：服务沙特客户", Confidence: 0.95})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Version != 2 {
		t.Fatalf("版本应递增到 2，got %d", upd.Version)
	}
	active := s.SelfModel(LayerGoal)
	if len(active) != 1 || active[0].Version != 2 {
		t.Fatalf("应只保留最新版本，got %+v", active)
	}
	//   tgt  superseded,    overwrite
	hist := 0
	s.mu.RLock()
	for _, it := range s.selfModel {
		if it.Status == StatusSuperseded {
			hist++
		}
	}
	s.mu.RUnlock()
	if hist != 1 {
		t.Fatalf("应有 1 条 superseded 历史，got %d", hist)
	}
}

func TestSearchThreeFactor(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.WriteMemory(MemoryItem{Text: "沙特运营商资质办理流程", Layer: LayerMechanism, Domain: DomainAgent, Importance: 8})
	_, _ = s.WriteMemory(MemoryItem{Text: "普通闲聊记录", Layer: LayerBehavior, Domain: DomainSession, Importance: 1})
	time.Sleep(10 * time.Millisecond)
	items := s.Search("沙特 运营商 资质", 3, time.Now())
	if len(items) == 0 || items[0].Text != "沙特运营商资质办理流程" {
		t.Fatalf("高相关+高重要性条目应排第一，got %+v", items)
	}
}

func TestDecayRecoverable(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.WriteMemory(MemoryItem{Text: "过期事件", Domain: DomainSession, Importance: 1})
	//    10 day    -> splitnum at value
	old := time.Now().Add(-240 * time.Hour)
	s.mu.Lock()
	s.ltm[0].LastSeen = old
	s.mu.Unlock()
	n := s.DecayAndEvict(time.Now(), 0.15)
	if n != 1 {
		t.Fatalf("应降权 1 条，got %d", n)
	}
	s.mu.RLock()
	st := s.ltm[0].Status
	s.mu.RUnlock()
	if st != StatusDecayed {
		t.Fatalf("应为 decayed（可恢复），got %v", st)
	}
}

func TestSurvivalProbe(t *testing.T) {
	s := newTestStore(t)
	s.Probe("call:retry:reason", "重试 3 次后仍 429，走降级通道")
	s.Probe("call:retry:reason", "超时 8s，切换备用模型")
	s.mu.RLock()
	first := s.probes[0].ID
	s.mu.RUnlock()
	s.ProbeRecall(first)
	if got := s.SurvivalRate(); got != 0.5 {
		t.Fatalf("survival rate 应 0.5，got %v", got)
	}
}

// ---- contract.go ----

func TestContractInjectAndRetrieve(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.UpsertSelf(SelfItem{Layer: LayerGoal, Text: "主目标：沙特市场落地", Confidence: 0.9})
	_, _ = s.UpsertSelf(SelfItem{Layer: LayerRule, Text: "资质未确认前不承诺交付日期", Confidence: 0.9})
	_, _ = s.WriteMemory(MemoryItem{Text: "沙特需要运营商资质", Layer: LayerMechanism, Domain: DomainAgent, Importance: 7})

	log, err := NewCallLogStore(tmpDir(t))
	if err != nil {
		t.Fatal(err)
	}
	c := NewContract(s, log)
	ctx := context.Background()

	base, err := c.InjectBaseline(ctx, "沙特 资质")
	if err != nil {
		t.Fatal(err)
	}
	if base.Goals == "" || base.Rules == "" {
		t.Fatal("基线应含目标+规则")
	}
	if base.TokenBudget > BaselineMaxTokens {
		t.Fatalf("预算提示越界：%d", base.TokenBudget)
	}
	if len(base.Retrieved) == 0 {
		t.Fatal("检索应返回机制层命中")
	}

	items, err := c.RetrieveContext(ctx, "沙特 资质", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("retrieve-context 应非空")
	}

	// rev  
	if err := c.LogTrajectory(ctx, CallLog{TaskProfile: "t1", Model: "deepseek-chat", Outcome: "success", Cost: 1.2}); err != nil {
		t.Fatal(err)
	}
	if err := c.LogFeedback(ctx, "t1", "success", 1.0); err != nil {
		t.Fatal(err)
	}
	_ = log.Close()
}

// ---- reflect.go ----

func TestReflectInputGate(t *testing.T) {
	s := newTestStore(t)
	r := NewReflector(s)
	r.InputGate = true
	r.MaxIdle = 3600 * time.Second

	// node inhas in   -> pos   (  ed)
	s.MarkInput()
	skip, deep, err := r.Tick(context.Background(), time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if skip || deep {
		t.Fatalf("30s 内有输入应正常检查，got skip=%v deep=%v", skip, deep)
	}

	//  ed  node (90s)nonew in ->  ed
	s.MarkInput()
	skip, deep, err = r.Tick(context.Background(), time.Now().Add(90*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !skip || deep {
		t.Fatalf("90s 无输入应跳过，got skip=%v deep=%v", skip, deep)
	}

	//   MaxIdle(2h)-> keepbot restrict  (  rev )
	s.MarkInput()
	skip, deep, err = r.Tick(context.Background(), time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if skip || deep {
		t.Fatalf("超 1h 应保底触发，got skip=%v deep=%v", skip, deep)
	}
	st := r.Stats()
	if st.IdleForced != 1 {
		t.Fatalf("应记录 1 次保底，got %+v", st)
	}
}

func TestReflectThresholdAndDedup(t *testing.T) {
	s := newTestStore(t)
	r := NewReflector(s)
	r.Threshold = 30

	// heavyneedity  to value( in   -> pos    ->  rev )
	for i := 0; i < 5; i++ {
		s.TouchSTM(MemoryItem{Text: "沙特客户偏好本地语言回复", Layer: LayerBehavior, Importance: 7}, 10)
	}
	s.MarkInput()
	skip, deep, err := r.Tick(context.Background(), time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if skip || !deep {
		t.Fatalf("累计超阈值应深反思，got skip=%v deep=%v", skip, deep)
	}
	if r.Stats().Written == 0 {
		t.Fatal("深反思应写入记忆")
	}

	// again triggersend -> writebefore   heavyreject
	before := r.Stats().Written
	for i := 0; i < 5; i++ {
		s.TouchSTM(MemoryItem{Text: "沙特客户偏好本地语言回复", Layer: LayerBehavior, Importance: 7}, 10)
	}
	s.MarkInput()
	_, _, _ = r.Tick(context.Background(), time.Now().Add(30*time.Second))
	if r.Stats().Written != before {
		t.Fatalf("重复内容应被去重拒绝，before=%d after=%d", before, r.Stats().Written)
	}
	if r.Stats().Rejected == 0 {
		t.Fatal("应有拒绝计数")
	}
}

func TestReflectorSetInterval(t *testing.T) {
	r := NewReflector(newTestStore(t))
	r.SetInterval(5 * time.Second)
	if r.Interval != MinInterval {
		t.Fatalf("5s 应 clamp 到 60s，got %v", r.Interval)
	}
	r.SetInterval(30 * time.Minute)
	if r.Interval != MaxInterval {
		t.Fatalf("30min 应 clamp 到 600s，got %v", r.Interval)
	}
	r.SetInterval(3 * time.Minute)
	if r.Interval != 180*time.Second {
		t.Fatalf("3min 应生效，got %v", r.Interval)
	}
}

// ---- registry.go ----

func TestRegistryCompressionPolicy(t *testing.T) {
	r, err := NewRegistry(tmpDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Register(ModelProfile{ID: "deepseek-chat", NominalWindow: 64000, EffectiveWindow: 32000, InstructionFollow: 0.8, PriceClass: "cheap"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(ModelProfile{ID: "gpt-5", NominalWindow: 400000, EffectiveWindow: 300000, InstructionFollow: 0.95, PriceClass: "premium", Density: DensityAggressive}); err != nil {
		t.Fatal(err)
	}
	//   typedefaultkeep   
	p, err := r.CompressionPolicy("deepseek-chat")
	if err != nil {
		t.Fatal(err)
	}
	if p != DensityConservative {
		t.Fatalf("弱模型应保守压缩，got %v", p)
	}
	//   type form    
	p, _ = r.CompressionPolicy("gpt-5")
	if p != DensityAggressive {
		t.Fatalf("强模型应激进压缩，got %v", p)
	}
	if _, err := r.CompressionPolicy("unknown"); err != ErrNotFound {
		t.Fatalf("未知模型应 ErrNotFound，got %v", err)
	}
}

// ---- router.go ----

func TestRouterRuleTableAndShadow(t *testing.T) {
	dir := tmpDir(t)
	reg, _ := NewRegistry(dir)
	_ = reg.Register(ModelProfile{ID: "cheap"})
	_ = reg.Register(ModelProfile{ID: "mid"})
	_ = reg.Register(ModelProfile{ID: "premium"})
	_ = reg.SetDefault("mid")
	rt, err := NewRouter(reg, dir)
	if err != nil {
		t.Fatal(err)
	}

	// lineon form:      -> cheap
	d, err := rt.Decide(TaskProfile{Complexity: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	if d.ModelID != "cheap" || d.Shadow {
		t.Fatalf("低复杂度应路由 cheap，got %+v", d)
	}
	//      -> premium
	d, _ = rt.Decide(TaskProfile{Complexity: 0.9})
	if d.ModelID != "premium" {
		t.Fatalf("高复杂度应路由 premium，got %+v", d)
	}

	//    form:    premium, butoccurproduce  default mid
	rt.SetShadow(true)
	d, _ = rt.Decide(TaskProfile{Complexity: 0.9})
	if d.ModelID != "premium" || d.Production != "mid" || !d.Shadow {
		t.Fatalf("影子模式应推荐 premium 生产 mid，got %+v", d)
	}
	if len(rt.ShadowLog()) != 1 {
		t.Fatal("影子决策应记录 1 条")
	}
	if err := rt.FlushShadow(); err != nil {
		t.Fatal(err)
	}
	//   after empty
	if len(rt.ShadowLog()) != 0 {
		t.Fatal("FlushShadow 后影子日志应清空")
	}
}

// ---- compress.go ----

func TestCompressKeepsDecisionEssentials(t *testing.T) {
	s := newTestStore(t)
	c := NewCompressor(s)
	full := "[goal] 主目标：沙特市场落地\n[rule] 资质未确认不承诺日期\n[pending] 客户名单待确认\n" +
		"tool_result: {\"status\":\"ok\"}\n中间过程文本 A\n中间过程文本 B\n[goal] 主目标：沙特市场落地（重复无害）"
	out, err := c.Compress(context.Background(), CompressInput{Full: full})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Goals) == 0 || len(out.Rules) == 0 || len(out.Pending) == 0 {
		t.Fatalf("决策要件应原样保留，got %+v", out)
	}
	if out.DroppedToolResult != 1 {
		t.Fatalf("tool-result 应被清除，got dropped=%d", out.DroppedToolResult)
	}
	if out.Summary == "" {
		t.Fatal("应有主体摘要")
	}
}

// ---- cloud.go ----

type fakeVault struct {
	written []VaultEntry
	read    []VaultEntry
}

func (f *fakeVault) Write(_ context.Context, e VaultEntry) error {
	f.written = append(f.written, e)
	return nil
}
func (f *fakeVault) Read(_ context.Context) ([]VaultEntry, error) { return f.read, nil }

func TestVaultClientWriteRejectsEmpty(t *testing.T) {
	c := NewVaultClient("")
	if err := c.Write(context.Background(), VaultEntry{Text: ""}); err == nil {
		t.Fatal("空文本应拒绝")
	}
	if err := c.Write(context.Background(), VaultEntry{Text: "x", Type: "rule"}); err == nil {
		t.Fatal("空 key + 网络应失败（非 2xx 或连接错误）")
	}
}

// ---- integration.go ----

func TestZhijiIntegrationFlow(t *testing.T) {
	vault := &fakeVault{}
	z, err := NewZhiji(tmpDir(t), vault)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = z.Close() }()
	_ = z.Registry.Register(ModelProfile{ID: "cheap"})
	_ = z.Registry.Register(ModelProfile{ID: "mid"})
	_ = z.Registry.Register(ModelProfile{ID: "premium"})

	//  in -> decide notein -> taskcloseend -> routeby
	// baseline=objtgt/rule      type(Baseline()), new store asempty, first   write  kind objtgt. 
	if _, err := z.Contract.UpdateSelfModel(context.Background(), SelfItem{Layer: LayerGoal, Text: "主目标：沙特市场落地", Confidence: 0.9}); err != nil {
		t.Fatal(err)
	}
	z.OnInput("用户询问沙特资质流程", 5)
	base, err := z.BeforeDecision(context.Background(), "沙特 资质")
	if err != nil {
		t.Fatal(err)
	}
	if base.Goals == "" {
		t.Fatal("决策注入应含目标基线")
	}
	if err := z.OnTaskEnd(context.Background(), CallLog{TaskProfile: "t-saudi", Model: "mid", Outcome: "success", Cost: 1.0}); err != nil {
		t.Fatal(err)
	}
	d, err := z.OnDecide(TaskProfile{Complexity: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	if d.ModelID != "cheap" {
		t.Fatalf("低复杂度应路由 cheap，got %+v", d)
	}

	// rev  value ->  rev  -> outize
	for i := 0; i < 5; i++ {
		z.OnInput("沙特客户偏好本地语言回复", 7)
	}
	_, _, _ = z.Reflector.Tick(context.Background(), time.Now().Add(30*time.Second))
	n, err := z.SyncVault(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("外化应写出至少 1 条")
	}
	if len(vault.written) == 0 {
		t.Fatal("vault 假实现应收到写入")
	}

	//   
	out, err := z.Compress(context.Background(), CompressInput{
		Full: "[goal] 主目标：沙特市场落地\n[rule] 不猜测用户未表达的意图\n中间过程文本",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Goals) == 0 {
		t.Fatal("压缩应保留目标")
	}
}

// ---- keep ize ----

func TestStorePersistRoundTrip(t *testing.T) {
	dir := tmpDir(t)
	s, _ := NewStore(dir)
	_, _ = s.UpsertSelf(SelfItem{Layer: LayerRule, Text: "不猜测用户未表达的意图", Confidence: 0.9})
	_, _ = s.WriteMemory(MemoryItem{Text: "教训：先确认再执行", Layer: LayerBehavior, Domain: DomainAgent, Importance: 6})
	if err := s.SaveAll(); err != nil {
		t.Fatal(err)
	}

	s2, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.SelfModel(LayerRule)) != 1 {
		t.Fatal("重载后自我模型应存在")
	}
}

// ---- v1.1 new  : graph / rawlog / systemone / budget search ----

func TestGraphAddNeighbors(t *testing.T) {
	g := NewGraph()
	g.Add(Edge{From: "mem-1", To: "mem-2", Rel: EdgeRelEntity, Weight: 0.8})
	g.Add(Edge{From: "mem-1", To: "mem-3", Rel: EdgeRelTemporal, Weight: 0.5})
	// same (From,To,Rel) heavy  ->   changenew heavy   
	g.Add(Edge{From: "mem-1", To: "mem-2", Rel: EdgeRelEntity, Weight: 0.9})
	if len(g.Edges) != 2 {
		t.Fatalf("去重后应 2 条边，got %d", len(g.Edges))
	}
	//   mem-1   entity   
	nb := g.Neighbors("mem-1", EdgeRelEntity)
	if len(nb) != 1 || nb[0].To != "mem-2" {
		t.Fatalf("应命中 1 条 entity 边，got %+v", nb)
	}
	//  ed  rel time toall 
	if all := g.Neighbors("mem-1"); len(all) != 2 {
		t.Fatalf("不过滤应命中 2 条边，got %+v", all)
	}
}

func TestGraphEmptyBudgetSearchSuperset(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.WriteMemory(MemoryItem{Text: "沙特运营商资质办理流程", Layer: LayerMechanism, Domain: DomainAgent, Importance: 8})
	_, _ = s.WriteMemory(MemoryItem{Text: "普通闲聊记录", Layer: LayerBehavior, Domain: DomainSession, Importance: 1})
	// empty  -> BudgetSearch  izeas Search   
	got, trace := s.BudgetSearch("沙特 运营商", RetrieveBudget{MaxItems: 24, MaxHops: 3, Deadline: 2 * time.Second, MinScore: 0.1}, time.Now())
	baseline := s.Search("沙特 运营商", 24, time.Now())
	if len(got) < len(baseline) {
		t.Fatalf("空图 BudgetSearch 应是 Search 超集：budget=%d search=%d trace=%+v", len(got), len(baseline), trace)
	}
	if len(got) > 0 && got[0].Text != baseline[0].Text {
		t.Fatalf("top1 应一致：budget=%q search=%q", got[0].Text, baseline[0].Text)
	}
	if trace.StopReason != "budget" {
		t.Fatalf("空图退化路径 StopReason 应为 budget，got %q", trace.StopReason)
	}
}

func TestRawLogAppendOnly(t *testing.T) {
	dir := tmpDir(t)
	path := filepath.Join(dir, "raw.jsonl")
	l := NewRawLog(path)
	if err := l.Append(RawObs{Text: "第一条观察", Provenance: Provenance{Origin: "user_input"}}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(RawObs{Text: "第二条观察"}); err != nil {
		t.Fatal(err)
	}
	if n := l.Len(); n != 2 {
		t.Fatalf("应有 2 条 raw，got %d", n)
	}
	// heavynew open -> nextRaw connectcontinue,     
	l2 := NewRawLog(path)
	if err := l2.Append(RawObs{Text: "第三条"}); err != nil {
		t.Fatal(err)
	}
	if n := l2.Len(); n != 3 {
		t.Fatalf("追加后应 3 条，got %d", n)
	}
}

func TestDefaultSystemOneClassify(t *testing.T) {
	d := &DefaultSystemOne{}
	// preference
	m := d.Classify(MemoryItem{Text: "我喜欢本地语言回复"})
	if m[MemKindPreference] < 0.8 {
		t.Fatalf("preference 应 0.8，got %+v", m)
	}
	// procedural
	m = d.Classify(MemoryItem{Text: "怎么办理运营商资质"})
	if m[MemKindProcedural] < 0.8 {
		t.Fatalf("procedural 应 0.8，got %+v", m)
	}
	// episodic
	m = d.Classify(MemoryItem{Text: "昨天签了合同"})
	if m[MemKindEpisodic] < 0.7 {
		t.Fatalf("episodic 应 0.7，got %+v", m)
	}
	// semantic  bot
	m = d.Classify(MemoryItem{Text: "沙特是中东国家"})
	if m[MemKindSemantic] < 0.6 {
		t.Fatalf("semantic 兜底应 0.6，got %+v", m)
	}
	// routeby: timetimeword + because word ->    sametime  
	views, hops := d.Route("为什么昨天签约失败")
	if hops != 3 {
		t.Fatalf("hops 应 3，got %d", hops)
	}
	if views[EdgeRelCausal] < 0.7 || views[EdgeRelTemporal] < 0.7 {
		t.Fatalf("应同时命中 causal+temporal，got %+v", views)
	}
	// defaultnosignal -> semantic   
	views, _ = d.Route("沙特")
	if views[EdgeRelSemantic] < 0.6 {
		t.Fatalf("无信号应走 semantic 兜底，got %+v", views)
	}
}

func TestBudgetSearchStopReason(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		_, _ = s.WriteMemory(MemoryItem{Text: "沙特客户偏好本地语言回复", Layer: LayerBehavior, Domain: DomainAgent, Importance: 6})
	}
	// empty  ->  izepath
	_, trace := s.BudgetSearch("沙特 偏好", RetrieveBudget{MaxItems: 24, MaxHops: 3, Deadline: 2 * time.Second, MinScore: 0.1}, time.Now())
	if trace.StopReason != "budget" {
		t.Fatalf("空图退化路径 StopReason 应为 budget，got %q", trace.StopReason)
	}
	// Noul/Choice hasboundaryverify
	if !AskNoul(0.5) || AskNoul(-0.1) || AskNoul(1.5) {
		t.Fatal("AskNoul 边界校验错误")
	}
	if !AskChoice(map[EdgeRel]float64{EdgeRelSemantic: 1.0}) {
		t.Fatal("归一分布应通过 AskChoice")
	}
	if AskChoice(map[EdgeRel]float64{EdgeRelSemantic: 0.5}) {
		t.Fatal("未归一分布应被 AskChoice 拒绝")
	}
}
