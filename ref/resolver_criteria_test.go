//go:build vhsref

// resolver_criteria_test.go —— 代指三层硬判据（VHS-ZHIJI-001 R1–R8，先红纪律）。
package ref

import (
	"context"
	"testing"

	"voicesign-harness/plan"
)

type fakeZhiji struct {
	events []Event
	err    error
}

func (f *fakeZhiji) Events(ctx context.Context) ([]Event, error) { return f.events, f.err }

// ① 会话内唯一 ⇒ 消解，并留依据。
func TestR1SessionUniqueResolves(t *testing.T) {
	s := &SessionMemory{}
	s.Mention("那个模块", "报价模块")
	r := &Resolver{Session: s}
	res := r.Resolve(context.Background(), "那个模块")
	if !res.Ok || res.Canonical != "报价模块" {
		t.Fatalf("[R1] 会话内唯一未消解: %+v", res)
	}
	if len(res.Evidence) == 0 || res.Evidence[0].Layer != "session" {
		t.Errorf("[R] 未留依据: %+v", res.Evidence)
	}
}

// ② 多候选 ⇒ 回问，且**必须给候选**。
func TestR2MultiCandidateAsksBackWithCandidates(t *testing.T) {
	s := &SessionMemory{}
	s.Mention("那个模块", "报价模块")
	s.Mention("那个模块", "库存模块")
	r := &Resolver{Session: s}
	res := r.Resolve(context.Background(), "那个模块")
	if res.Ok {
		t.Fatalf("[R2] 多候选却消解了（脑补）: %+v", res)
	}
	if !res.AskBack {
		t.Errorf("[R2] 多候选未回问: %+v", res)
	}
	if len(res.Candidates) != 2 {
		t.Errorf("[ASK-3b] 回问必须给候选: %+v", res.Candidates)
	}
}

// ③ 无候选 ⇒ 回问（不脑补），不编造候选。
func TestR3NoCandidateAsksBack(t *testing.T) {
	r := &Resolver{Session: &SessionMemory{}}
	res := r.Resolve(context.Background(), "那个东西")
	if res.Ok || !res.AskBack {
		t.Fatalf("[R3] 无候选应回问: %+v", res)
	}
	if len(res.Candidates) != 0 {
		t.Errorf("[R3] 无候选时不得编造候选: %+v", res.Candidates)
	}
}

// ④ 工作记忆层：唯一 ⇒ 消解，依据标 working_memory。
func TestR4WorkingMemoryLayer(t *testing.T) {
	w := &plan.WorkingMemory{}
	w.Remember("working_set", plan.BoardItem{Element: "模块:报价模块", Source: "session"})
	r := &Resolver{Session: &SessionMemory{}, WM: w}
	res := r.Resolve(context.Background(), "报价模块")
	if !res.Ok || res.Canonical != "模块:报价模块" {
		t.Fatalf("[R4] 工作记忆层未消解: %+v", res)
	}
	if res.Evidence[0].Layer != "working_memory" {
		t.Errorf("[R4] 依据层标错: %+v", res.Evidence)
	}
}

// ⑤ 知己层（只读）：会话/记忆都空时用它，依据标 zhiji。
func TestR5ZhijiLayerReadOnly(t *testing.T) {
	r := &Resolver{Session: &SessionMemory{}, Zhiji: &fakeZhiji{events: []Event{
		{ID: "1", Source: "thread-a", Text: "上次说要把那个报表改成中文"},
	}}}
	res := r.Resolve(context.Background(), "那个报表")
	if !res.Ok || res.Canonical != "thread-a" {
		t.Fatalf("[R5] 知己层未消解: %+v", res)
	}
	if res.Evidence[0].Layer != "zhiji" {
		t.Errorf("[R5] 依据层标错: %+v", res.Evidence)
	}
}

// ⑥ 来源冲突 ⇒ 中断（不自行裁决）。
func TestR6SourceConflictInterrupts(t *testing.T) {
	s := &SessionMemory{}
	s.Mention("那个报表", "报表A")
	r := &Resolver{Session: s, Zhiji: &fakeZhiji{events: []Event{
		{ID: "1", Source: "thread-b", Text: "那个报表要改成中文"},
	}}}
	res := r.Resolve(context.Background(), "那个报表")
	if !res.Conflict || !res.AskBack {
		t.Fatalf("[R6] 来源冲突未中断: %+v", res)
	}
	if res.Ok {
		t.Errorf("[R6] 冲突时不得给出唯一答案: %+v", res)
	}
}
