//go:build vhsroute

// router_criteria_test.go —— FASTSLOW-001 三层路由的硬要求（先红 → 绿）。
package route

import (
	"context"
	"errors"
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
