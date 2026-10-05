package zhiji

import (
	"context"
	"os"
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
	// 旧版标记 superseded，不静默覆盖
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
	// 模拟 10 天未访问 → 分数低于阈值
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

	// 反馈族
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

	// 节律内有输入活动 → 正常检查（不跳过）
	s.MarkInput()
	skip, deep, err := r.Tick(context.Background(), time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if skip || deep {
		t.Fatalf("30s 内有输入应正常检查，got skip=%v deep=%v", skip, deep)
	}

	// 超过一个节律（90s）无新输入 → 跳过
	s.MarkInput()
	skip, deep, err = r.Tick(context.Background(), time.Now().Add(90*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !skip || deep {
		t.Fatalf("90s 无输入应跳过，got skip=%v deep=%v", skip, deep)
	}

	// 超 MaxIdle（2h）→ 保底强制一次（不深反思）
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

	// 重要性累计到阈值（输入活动 → 正常检查 → 深反思）
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

	// 再次触发 → 写前验证去重拒绝
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
	// 弱模型默认保守压缩
	p, err := r.CompressionPolicy("deepseek-chat")
	if err != nil {
		t.Fatal(err)
	}
	if p != DensityConservative {
		t.Fatalf("弱模型应保守压缩，got %v", p)
	}
	// 强模型显式激进压缩
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

	// 线上模式：低复杂度 → cheap
	d, err := rt.Decide(TaskProfile{Complexity: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	if d.ModelID != "cheap" || d.Shadow {
		t.Fatalf("低复杂度应路由 cheap，got %+v", d)
	}
	// 高复杂度 → premium
	d, _ = rt.Decide(TaskProfile{Complexity: 0.9})
	if d.ModelID != "premium" {
		t.Fatalf("高复杂度应路由 premium，got %+v", d)
	}

	// 影子模式：推荐 premium，但生产仍走默认 mid
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
	// 落盘后清空
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

	// 输入 → 决策注入 → 任务结束 → 路由
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

	// 反思阈值 → 深反思 → 外化
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

	// 压缩
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

// ---- 持久化 ----

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
