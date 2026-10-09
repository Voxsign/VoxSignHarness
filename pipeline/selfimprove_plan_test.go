package pipeline

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"voicesign-harness/contract"
)

func TestReactPlanActionSurvivesWindow(t *testing.T) {
	thread := &reactThread{goal: "implement persistent planning"}
	a, err := parseReactAction(`{"tool":"plan","steps":[{"step":"Research","status":"completed"},{"step":"Implement","status":"in_progress"},{"step":"Test","status":"pending"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	o := &Options{}
	obs, ok, touched := o.executeThreadAction("", "", a, nil, thread)
	if !ok || len(touched) != 0 || obs.Obs != "Plan updated." {
		t.Fatalf("plan action: %+v, ok=%v, touched=%v", obs, ok, touched)
	}
	a.Steps[0].Step = "mutated caller slice"
	for i := 0; i < selfImproveWindowK+3; i++ {
		thread.push(contract.Message{Role: "assistant", Content: "old action"}, contract.Message{Role: "user", Content: "observation"}, "summary")
	}
	messages := thread.messages()
	last := messages[1].Content
	for _, want := range []string{thread.goal, "[completed] Research", "[in_progress] Implement", "[pending] Test", "1/3 completed"} {
		if !strings.Contains(last, want) {
			t.Errorf("persistent context missing %q: %s", want, last)
		}
	}
	if len(thread.recent) != selfImproveWindowK {
		t.Fatal("test did not evict original exchanges")
	}
}

func TestReactPlanInvalidUpdatesAreAtomic(t *testing.T) {
	thread := &reactThread{goal: "fixed goal"}
	valid := []reactPlanStep{{"Research", "completed"}, {"Implement", "in_progress"}}
	if err := thread.updatePlan(valid); err != nil {
		t.Fatal(err)
	}
	cases := [][]reactPlanStep{
		nil,
		make([]reactPlanStep, 9),
		{{"", "pending"}},
		{{strings.Repeat("界", 161), "pending"}},
		{{"line\nbreak", "pending"}},
		{{"A", "unknown"}},
		{{"A", "pending"}, {"A", "pending"}},
		{{"A", "in_progress"}, {"B", "in_progress"}},
		{{"A", "pending"}, {"B", "completed"}},
		{{"A", "pending"}, {"B", "in_progress"}},
	}
	for _, steps := range cases {
		if err := thread.updatePlan(steps); err == nil {
			t.Errorf("accepted invalid plan: %+v", steps)
		}
		if !reflect.DeepEqual(thread.plan, valid) || thread.goal != "fixed goal" {
			t.Fatalf("invalid update changed persistent state: %+v", thread)
		}
	}
}

func TestReactPlanCompletionGate(t *testing.T) {
	thread := &reactThread{goal: "research"}
	o := &Options{}
	var receipts []contract.Receipt
	seq := 0
	gate := func() gateResult {
		return o.selfImproveBuildTestGate(context.Background(), selfImproveTestRepo(t), "", nil, false, nil, thread, &receipts, &seq)
	}
	if thread.planCompletionError() == nil || gate().passed {
		t.Fatal("missing plan must reject completion, including research-only tasks")
	}
	if err := thread.updatePlan([]reactPlanStep{{"Research", "in_progress"}}); err != nil {
		t.Fatal(err)
	}
	if thread.planCompletionError() == nil || gate().passed {
		t.Fatal("unfinished plan passed gate")
	}
	if err := thread.updatePlan([]reactPlanStep{{"Research", "completed"}}); err != nil {
		t.Fatal(err)
	}
	if thread.planCompletionError() != nil || !gate().passed {
		t.Fatal("completed research plan should pass")
	}
	result := o.selfImproveBuildTestGate(context.Background(), selfImproveTestRepo(t), "", nil, true, nil, thread, &receipts, &seq)
	if result.passed {
		t.Fatal("plan completion must not bypass code-change gate")
	}
}
