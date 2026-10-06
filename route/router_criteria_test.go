//go:build vhsroute

// router_criteria_test.go -- FASTSLOW-001   routeby  needrequire(first  ->  ). 
package route

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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

// ① L0  ini.e.returnback, **     type**. 
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

// ② ambiguous ⇒ clarificationuseuser, **   L1**. 
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

// ③   200/ time ⇒ cur JEV   use ⇒    L0 + degraded(     to). 
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

// ④  disconnectin      tgt judged_by=jev, **  and    **. 
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

// ①  scenariosplit : kind bysignalword  ,  again as custom. 
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
	//  nameclasstypekeep :   value   periodwrite out ( disconnectlangonlyispreventback as string)
	var k Kind = KindPermission
	if string(k) != "permission" {
		t.Errorf("Kind 常量值不符: %q", k)
	}
}

// ①     -> situation close  connect(J2:   close ize,  is  base). 
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
	//  disconnect obj   ( disconnect)tgt ,  and    
	w2 := &plan.WorkingMemory{}
	w2.Remember("working_set", plan.BoardItem{Element: "判断:allow", Source: "jev", JudgedBy: "jev"})
	if s := SituationFromMemory(w2); !strings.Contains(s.Candidates[0].Why, "判断") {
		t.Errorf("[桥接] 判断条目未标记: %+v", s.Candidates)
	}
}

// ① kind   be to JEV( is  basely). 
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

//     :  close /api/route unique in ⇒ L0 returnback, ** call JEV**(    ). 
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

//       use ⇒ fail-open  to L0.5, and**  **(  use !=  has). 
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

// kind  empty see: split  to custom ⇒       kind_fallback=true. 
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

//   onlyhas  triggersend of : calluse voice "needneed    "; ** because" changeapprove"  **. 
func TestEscalationOnlyOnReasoningNeed(t *testing.T) {
	l1 := &fakeL1{out: "多步方案：先 A 再 B"}
	jev := &fakeJEV{resp: JEVResponse{Choice: "x", Confidence: 0.99, ModelID: "jev-v1-rules"}}
	// (a)   needneed     ⇒   L1
	r1 := &Router{Hot: hotCache(t), JEV: jev, L1: l1, NeedsReasoning: true}
	d1 := r1.Route(context.Background(), "先查库存再改报价最后提交", "多步目标", Situation{})
	if d1.Level != LevelL1 || l1.calls != 1 {
		t.Fatalf("[L1] 需要多步推理未升级: %+v calls=%d", d1, l1.calls)
	}
	if d1.Level == LevelL1 && d1.Ledger[len(d1.Ledger)-1].Escalated != true {
		t.Errorf("[L1] 升级未记台账: %+v", d1.Ledger)
	}
	// (b)  needneed     ⇒ i.e.  L1  usealso  ( allow" changeapprovethen ")
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

//      emptyalso   see:   in/  in ⇒ route_ambiguous=true(     L0.5). 
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

//     : append-only,   charseg,    out    refertgt. 
func TestLedgerAppendOnlyAndAggregatable(t *testing.T) {
	dir := t.TempDir()
	l := &Ledger{Path: filepath.Join(dir, "ledger.jsonl")}
	// 1) L0  in
	r0 := &Router{Hot: hotCache(t)}
	d0 := r0.Route(context.Background(), "爱ops", "q", Situation{})
	if err := l.Write(d0, "t0", KindFor("爱ops")); err != nil {
		t.Fatal(err)
	}
	// 2) L0.5 ambiguous -> clarification
	r1 := &Router{Hot: hotCache(t), JEV: &fakeJEV{resp: JEVResponse{Choice: "ambiguous", Confidence: 0.5}}}
	d1 := r1.Route(context.Background(), "讲个笑话", "q", Situation{})
	if err := l.Write(d1, "t1", KindFor("讲个笑话")); err != nil {
		t.Fatal(err)
	}
	// 3) L1   
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
	// empty    
	if empty, err := Aggregate(filepath.Join(dir, "nope.jsonl")); err != nil || empty.Total != 0 {
		t.Errorf("[台账] 空/缺失台账应返回空摘要: %+v err=%v", empty, err)
	}
}

//   connect  flow: **    routebyofafter, ledger.jsonl    out  **. 
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

// P4④:   routeby  after, WorkingMemory    emptyand base  body. 
func TestRouterAutoBuildsWorkingMemory(t *testing.T) {
	wm := &plan.WorkingMemory{}
	r := &Router{Hot: hotCache(t), WM: wm}
	d := r.Route(context.Background(), "爱ops", "爱ops 是什么", Situation{})
	if len(wm.Items()) == 0 {
		t.Fatal("[P4④] 路由后工作记忆仍为空（永远是被喂的）")
	}
	found := false
	for _, it := range wm.Items() {
		if strings.Contains(it.Element, d.Choice) || it.Element == "爱ops 是什么" {
			found = true
		}
	}
	if !found {
		t.Errorf("[P4④] 工作记忆未含本轮实体: %+v", wm.Items())
	}
}

//         (fail-open but    ). 
func TestLedgerWriteFailureLeavesTrace(t *testing.T) {
	// use  **no   ** pathtriggersendwrite  ( pathisfilebut obj ). 
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Router{Hot: hotCache(t), Ledger: &Ledger{Path: filepath.Join(blocker, "ledger.jsonl")}}
	d := r.Route(context.Background(), "爱ops", "q", Situation{})
	found := false
	for _, e := range d.Ledger {
		if strings.Contains(e.Reason, "ledger_write_failed") {
			found = true
		}
	}
	if !found {
		t.Errorf("[台账] 写失败未留痕（静默）: %+v", d.Ledger)
	}
}

//     : kindbase    e.g.  "no   ", and  pt③needreferoutcharseg  . 
func TestReportIsHonestAboutInsufficientSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	l := &Ledger{Path: path}
	r := &Router{Hot: hotCache(t), Ledger: l}
	r.Route(context.Background(), "爱ops", "q", Situation{})
	out, err := Report(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "样本不足") {
		t.Errorf("[报告] 样本 1 条却未标样本不足:\n%s", out)
	}
	if !strings.Contains(out, "样本量: 1") {
		t.Errorf("[报告] 未标明样本量:\n%s", out)
	}
	if !strings.Contains(out, "考察点③") || !strings.Contains(out, "无法判定") {
		t.Errorf("[报告] 考察点③ 应如实说无法判定（缺字段）:\n%s", out)
	}
}

// empty    , ande.g.  "no   ". 
func TestReportOnEmptyLedger(t *testing.T) {
	out, err := Report(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "台账为空") {
		t.Errorf("[报告] 空台账应如实说明:\n%s", out)
	}
}

//   pt③:  beforeafter   ⇒   out"is change ";  charseg ⇒ unknown( cur 0)+ no   . 
func TestQualityComparisonAndBackwardCompat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	//   form obj(no quality charseg)--   ,   cur 0
	old := `{"at":"2026-10-03T00:00:00Z","level":"L1","reason":"legacy","escalated":true,"outcome":"answered"}`
	if err := os.WriteFile(path, []byte(old+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, after := 0.4, 0.9
	//    value:  at MinSamplesForVerdict   kindbasetime**  underclose ** ⇒   give  10  . 
	var recs []Record
	for i := 0; i < MinSamplesForVerdict; i++ {
		recs = append(recs, Record{
			At: "2026-10-03T00:01:00Z", Level: LevelL1, Reason: "escalated", Escalated: true,
			Outcome: "answered", TaskID: "t" + strconv.Itoa(i), QualityBefore: &before, QualityAfter: &after,
		})
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	for _, r := range recs {
		b, _ := json.Marshal(r)
		_, _ = f.Write(append(b, '\n'))
	}
	_ = f.Close()

	s, err := Aggregate(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.UnknownQuality != 1 {
		t.Errorf("[兼容] 旧条目应计 unknown，实际 %d", s.UnknownQuality)
	}
	if s.ComparableQuality != MinSamplesForVerdict || s.Improved != MinSamplesForVerdict {
		t.Errorf("[考察点③] 可比/改善统计不符: %+v", s)
	}
	out, err := Report(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "改善 10") {
		t.Errorf("[考察点③] 报告未给出改善结论:\n%s", out)
	}
	if !strings.Contains(out, "unknown 计") {
		t.Errorf("[兼容] 旧条目应按 unknown 计数:\n%s", out)
	}
}
