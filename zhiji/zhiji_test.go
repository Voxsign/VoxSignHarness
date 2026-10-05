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
