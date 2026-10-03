//go:build vhsroute

// router_criteria_test.go —— FASTSLOW-001 三层路由的硬要求（先红 → 绿）。
package route

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"voicesign-harness/hotcache"
	"voicesign-harness/plan"
)

type fakeJEV struct {
	calls int
	resp  JEVResponse
	err   error
}

func (f *fakeJEV) Decide(ctx context.Context, req JEVRequest) (JEVResponse, error) {
	f.calls++
	return f.resp, f.err
}

func hotCache(t *testing.T) *hotcache.Cache {
	t.Helper()
	c := hotcache.New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	c.PutAlias("爱ops", "aiops-portal", "local")
	return c
}

// ① L0 命中即返回，**不问任何模型**。
func TestL0HitNeverCallsModel(t *testing.T) {
	jev := &fakeJEV{resp: JEVResponse{Choice: "other", Confidence: 0.99}}
	r := &Router{Hot: hotCache(t), JEV: jev}
	d := r.Route(context.Background(), "爱ops", "q", Situation{})
	if jev.calls != 0 {
		t.Fatalf("[L0] 本地命中却调了 JEV %d 次（红线：命中即返回）", jev.calls)
	}
	if d.Level != LevelL0 || d.Action != ActionAnswer || d.Choice != "aiops-portal" {
		t.Fatalf("[L0] 决策不符: %+v", d)
	}
	if len(d.Ledger) == 0 {
		t.Error("[L0] 缺台账")
	}
}

// ② ambiguous ⇒ 回问用户，**不升 L1**。
func TestAmbiguousAsksUserNotEscalate(t *testing.T) {
	jev := &fakeJEV{resp: JEVResponse{Choice: "ambiguous", Confidence: 0.55, Reason: "候选区分度不足", ModelID: "jev-v1-rules"}}
	r := &Router{Hot: hotCache(t), JEV: jev}
	d := r.Route(context.Background(), "那个东西", "q", Situation{Candidates: []Candidate{{ID: "a"}, {ID: "b"}}})
	if d.Action != ActionAskUser {
		t.Fatalf("[L0.5] ambiguous 应回问用户: %+v", d)
	}
	if d.Action == ActionEscalate {
		t.Fatal("[L0.5] ambiguous 不得升级 L1")
	}
	for _, e := range d.Ledger {
		if e.Escalated {
			t.Errorf("[L0.5] ambiguous 不应记为已升级: %+v", e)
		}
	}
}

// ③ 非 200/超时 ⇒ 当 JEV 不可用 ⇒ 降级 L0 + degraded（不假设它总对）。
func TestJEVFailureDegradesToL0(t *testing.T) {
	jev := &fakeJEV{err: errors.New("HTTP 502（按不可用处理）")}
	r := &Router{Hot: hotCache(t), JEV: jev}
	d := r.Route(context.Background(), "那个东西", "q", Situation{})
	if !d.Degraded || d.DegradedReason == "" {
		t.Fatalf("[降级] 未标 degraded: %+v", d)
	}
	if d.Level != LevelL0 {
		t.Errorf("[降级] 应降回 L0: %+v", d)
	}
	if d.Action != ActionAskUser {
		t.Errorf("[降级] 降级后应回问而非硬答: %+v", d)
	}
}

// ④ 判断入工作记忆必须标 judged_by=jev，**不得与事实混放**。
func TestJudgementMarkedJudgedBy(t *testing.T) {
	jev := &fakeJEV{resp: JEVResponse{Choice: "allow", Confidence: 0.88, ModelID: "jev-v1-rules"}}
	r := &Router{Hot: hotCache(t), JEV: jev}
	d := r.Route(context.Background(), "能不能写 vault", "q", Situation{Candidates: []Candidate{{ID: "allow"}, {ID: "deny"}}})
	w := &plan.WorkingMemory{}
	RememberJudgement(w, d)
	text := w.Render()
	if !strings.Contains(text, "judged_by=jev") {
		t.Fatalf("[WM-4] JEV 判断未标 judged_by: %q", text)
	}
	if !strings.Contains(text, "判断:allow") {
		t.Errorf("[WM-4] 判断内容缺失: %q", text)
	}
}

// ① 场景分派：kind 按信号词选择，不再恒为 custom。
func TestKindDispatchByScenario(t *testing.T) {
	cases := map[string]Kind{
		"能不能写入 vault-creds": KindPermission,
		"帮我把那个模块改了":         KindReferent,
		"记住这次的口音偏好":         KindLearnability,
		"这件事该找谁":            KindGapClass,
		"讲个笑话":              KindCustom,
	}
	for text, want := range cases {
		if got := KindFor(text); got != want {
			t.Errorf("[kind 分派] %q → %q，期望 %q", text, got, want)
		}
	}
	// 具名类型保证：非法值在编译期写不出来（此断言只是防回退为 string）
	var k Kind = KindPermission
	if string(k) != "permission" {
		t.Errorf("Kind 常量值不符: %q", k)
	}
}

// ① 四块板 → situation 结构桥接（J2：紧凑结构化，不是长文本）。
func TestSituationFromMemoryBridgesBoards(t *testing.T) {
	w := &plan.WorkingMemory{}
	w.Remember("working_set", plan.BoardItem{Element: "文件:plan/planner.go", Source: "上一轮"})
	w.Remember("constraints", plan.BoardItem{Element: "域=vault-creds 只读", Source: "space"})
	w.Remember("open_items", plan.BoardItem{Element: "尚未确认目标文件", Source: "session"})
	w.Remember("situation", plan.BoardItem{Element: "用户在改规划器", Source: "session"})
	sit := SituationFromMemory(w)
	if len(sit.Candidates) != 1 || sit.Candidates[0].ID != "文件:plan/planner.go" {
		t.Errorf("[桥接] candidates 未来自活跃实体板: %+v", sit.Candidates)
	}
	if len(sit.Constraints) != 1 || !strings.Contains(sit.Constraints[0], "vault-creds") {
		t.Errorf("[桥接] constraints 未来自约束板: %+v", sit.Constraints)
	}
	if len(sit.Memory) != 2 {
		t.Errorf("[桥接] memory 应含情景板+待决板: %+v", sit.Memory)
	}
	// 判断条目必须带（判断）标记，不与事实混放
	w2 := &plan.WorkingMemory{}
	w2.Remember("working_set", plan.BoardItem{Element: "判断:allow", Source: "jev", JudgedBy: "jev"})
	if s := SituationFromMemory(w2); !strings.Contains(s.Candidates[0].Why, "判断") {
		t.Errorf("[桥接] 判断条目未标记: %+v", s.Candidates)
	}
}

// ① kind 真的被送到 JEV（不是留在本地）。
func TestKindIsSentToJEV(t *testing.T) {
	var got Kind
	jev := &recordingJEV{record: func(r JEVRequest) { got = r.Kind }}
	r := &Router{Hot: hotCache(t), JEV: jev}
	r.Route(context.Background(), "能不能写入 vault", "能不能写入 vault-creds", Situation{})
	if got != KindPermission {
		t.Errorf("[kind] 送到 JEV 的 kind=%q，期望 permission", got)
	}
}

type recordingJEV struct{ record func(JEVRequest) }

func (j *recordingJEV) Decide(ctx context.Context, req JEVRequest) (JEVResponse, error) {
	if j.record != nil {
		j.record(req)
	}
	return JEVResponse{Choice: "ambiguous", Confidence: 0.5}, nil
}

type fakeRoute struct {
	name string
	ok   bool
	err  error
}

func (f *fakeRoute) Lookup(ctx context.Context, q string) (string, bool, error) {
	return f.name, f.ok, f.err
}

type fakeL1 struct {
	out   string
	err   error
	calls int
}

func (f *fakeL1) Complete(ctx context.Context, prompt string) (string, error) {
	f.calls++
	return f.out, f.err
}

// 第二梯队：网关 /api/route 唯一命中 ⇒ L0 返回，**不调 JEV**（顺序铁律）。
func TestGatewayRouteSecondTierAvoidsJEV(t *testing.T) {
	jev := &fakeJEV{resp: JEVResponse{Choice: "x", Confidence: 0.9}}
	r := &Router{Hot: hotCache(t), ServiceRouter: &fakeRoute{name: "cicd", ok: true}, JEV: jev}
	d := r.Route(context.Background(), "部署到生产", "部署到生产", Situation{})
	if jev.calls != 0 {
		t.Fatalf("[L0-2] 第二梯队命中却调了 JEV %d 次", jev.calls)
	}
	if d.Level != LevelL0 || d.Choice != "cicd" {
		t.Fatalf("[L0-2] 决策不符: %+v", d)
	}
}

// 第二梯队不可用 ⇒ fail-open 落到 L0.5，且**留痕**（不可用 ≠ 没有）。
func TestGatewayRouteUnavailableFallsThroughWithTrace(t *testing.T) {
	jev := &fakeJEV{resp: JEVResponse{Choice: "cicd", Confidence: 0.9, ModelID: "jev-v1-rules"}}
	r := &Router{Hot: hotCache(t), ServiceRouter: &fakeRoute{err: errors.New("HTTP 502（按不可用处理）")}, JEV: jev}
	d := r.Route(context.Background(), "部署到生产", "部署到生产", Situation{})
	if jev.calls != 1 {
		t.Fatalf("[L0-2] 不可用时应继续走 L0.5: calls=%d", jev.calls)
	}
	found := false
	for _, e := range d.Ledger {
		if strings.Contains(e.Reason, "route-unavailable") {
			found = true
		}
	}
	if !found {
		t.Errorf("[L0-2] 第二梯队不可用未留痕: %+v", d.Ledger)
	}
}

// kind 落空可见：分派落到 custom ⇒ 台账必须记 kind_fallback=true。
func TestKindFallbackIsVisible(t *testing.T) {
	jev := &fakeJEV{resp: JEVResponse{Choice: "x", Confidence: 0.9}}
	r := &Router{Hot: hotCache(t), JEV: jev}
	d := r.Route(context.Background(), "讲个笑话吧", "讲个笑话吧", Situation{})
	found := false
	for _, e := range d.Ledger {
		if e.Reason == "kind_fallback=true" {
			found = true
		}
	}
	if !found {
		t.Errorf("[kind] 落空未留痕（分派不准会静默成泛泛地问）: %+v", d.Ledger)
	}
}

// 升级只有两个触发器之一：调用方声明"需要多步推理"；**不因"想更准"升级**。
func TestEscalationOnlyOnReasoningNeed(t *testing.T) {
	l1 := &fakeL1{out: "多步方案：先 A 再 B"}
	jev := &fakeJEV{resp: JEVResponse{Choice: "x", Confidence: 0.99, ModelID: "jev-v1-rules"}}
	// (a) 明确需要多步推理 ⇒ 升 L1
	r1 := &Router{Hot: hotCache(t), JEV: jev, L1: l1, NeedsReasoning: true}
	d1 := r1.Route(context.Background(), "先查库存再改报价最后提交", "多步目标", Situation{})
	if d1.Level != LevelL1 || l1.calls != 1 {
		t.Fatalf("[L1] 需要多步推理未升级: %+v calls=%d", d1, l1.calls)
	}
	if d1.Level == LevelL1 && d1.Ledger[len(d1.Ledger)-1].Escalated != true {
		t.Errorf("[L1] 升级未记台账: %+v", d1.Ledger)
	}
	// (b) 不需要多步推理 ⇒ 即使 L1 可用也不升（不许"想更准就升"）
	l1b := &fakeL1{out: "不该被调用"}
	r2 := &Router{Hot: hotCache(t), JEV: jev, L1: l1b, NeedsReasoning: false}
	d2 := r2.Route(context.Background(), "库存还有多少", "查库存", Situation{})
	if l1b.calls != 0 {
		t.Fatalf("[L1] 非多步任务却升级了（退化成什么都问大模型）: calls=%d", l1b.calls)
	}
	if d2.Level != LevelL05 {
		t.Errorf("[L1] 应在 L0.5 停住: %+v", d2)
	}
}

// 第二梯队落空也必须可见：多命中/未命中 ⇒ route_ambiguous=true（不静默落 L0.5）。
func TestRouteAmbiguousIsVisible(t *testing.T) {
	jev := &fakeJEV{resp: JEVResponse{Choice: "x", Confidence: 0.9}}
	r := &Router{Hot: hotCache(t), ServiceRouter: &fakeRoute{ok: false}, JEV: jev}
	d := r.Route(context.Background(), "翻译一下", "翻译一下", Situation{})
	found := false
	for _, e := range d.Ledger {
		if e.Reason == "route_ambiguous=true" {
			found = true
		}
	}
	if !found {
		t.Errorf("[L0-2] 多命中/未命中未留痕（命中率将不可测）: %+v", d.Ledger)
	}
}

// 台账落盘：append-only、不丢字段、可聚合出三个考察指标。
func TestLedgerAppendOnlyAndAggregatable(t *testing.T) {
	dir := t.TempDir()
	l := &Ledger{Path: filepath.Join(dir, "ledger.jsonl")}
	// 1) L0 命中
	r0 := &Router{Hot: hotCache(t)}
	d0 := r0.Route(context.Background(), "爱ops", "q", Situation{})
	if err := l.Write(d0, "t0", KindFor("爱ops")); err != nil {
		t.Fatal(err)
	}
	// 2) L0.5 ambiguous → 回问
	r1 := &Router{Hot: hotCache(t), JEV: &fakeJEV{resp: JEVResponse{Choice: "ambiguous", Confidence: 0.5}}}
	d1 := r1.Route(context.Background(), "讲个笑话", "q", Situation{})
	if err := l.Write(d1, "t1", KindFor("讲个笑话")); err != nil {
		t.Fatal(err)
	}
	// 3) L1 升级
	r2 := &Router{Hot: hotCache(t), JEV: &fakeJEV{resp: JEVResponse{Choice: "x", Confidence: 0.9}}, L1: &fakeL1{out: "ok"}, NeedsReasoning: true}
	d2 := r2.Route(context.Background(), "先A再B", "q", Situation{})
	if err := l.Write(d2, "t2", KindFor("先A再B")); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(l.Path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), "\n"); n != 3 {
		t.Fatalf("[台账] append-only 应为 3 行，实际 %d", n)
	}
	s, err := Aggregate(l.Path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 3 || s.ByLevel[LevelL0] != 1 || s.ByLevel[LevelL05] != 1 || s.ByLevel[LevelL1] != 1 {
		t.Errorf("[台账] 层级分布不符: %+v", s)
	}
	if s.EscalationRate <= 0 || s.EscalationRate > 1 {
		t.Errorf("[台账] 升级率异常: %v", s.EscalationRate)
	}
	if s.AskedUserRate <= 0 {
		t.Errorf("[台账] 回问率应 >0: %v", s.AskedUserRate)
	}
	if s.L0Share <= 0 || s.L0Share > 1 {
		t.Errorf("[台账] L0 比例异常: %v", s.L0Share)
	}
	// 空台账不崩
	if empty, err := Aggregate(filepath.Join(dir, "nope.jsonl")); err != nil || empty.Total != 0 {
		t.Errorf("[台账] 空/缺失台账应返回空摘要: %+v err=%v", empty, err)
	}
}

// 台账接进主流程：**一次真实路由之后，ledger.jsonl 必须多出一条**。
func TestLedgerWiredIntoRouting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	r := &Router{Hot: hotCache(t), Ledger: &Ledger{Path: path}}
	r.Route(context.Background(), "爱ops", "q", Situation{})
	r.Route(context.Background(), "写点东西", "q", Situation{})
	s, err := Aggregate(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 2 {
		t.Fatalf("[台账接线] 两次路由后应有 2 条记录，实际 %d", s.Total)
	}
	if s.ByLevel[LevelL0] < 1 {
		t.Errorf("[台账接线] 层级分布缺失: %+v", s.ByLevel)
	}
}
