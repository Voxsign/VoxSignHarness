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
		return o.selfImproveBuildTestGate(context.Background(), selfImproveTestRepo(t), "", nil, false, nil, thread, &receipts, &seq, true)
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
	result := o.selfImproveBuildTestGate(context.Background(), selfImproveTestRepo(t), "", nil, true, nil, thread, &receipts, &seq, true)
	if result.passed {
		t.Fatal("plan completion must not bypass code-change gate")
	}
}

// R21: when a model rewrites the full step list and omits an existing step, the
// update must not be rejected; omitted steps are retained with prior status (no
// deletion). Completed steps still cannot regress and new steps are appended.
func TestReactPlanMergeRetainsOmittedSteps(t *testing.T) {
	thread := &reactThread{goal: "g"}
	base := []reactPlanStep{{"Research", "completed"}, {"Distill", "completed"}, {"Implement", "in_progress"}}
	if err := thread.updatePlan(base); err != nil {
		t.Fatal(err)
	}
	// The model reports only the step it is closing; earlier steps are omitted.
	if err := thread.updatePlan([]reactPlanStep{{"Implement", "completed"}}); err != nil {
		t.Fatalf("omitting prior steps should merge, got: %v", err)
	}
	want := []reactPlanStep{{"Research", "completed"}, {"Distill", "completed"}, {"Implement", "completed"}}
	if !reflect.DeepEqual(thread.plan, want) {
		t.Fatalf("merged plan = %+v, want %+v", thread.plan, want)
	}
	// A completed step can never regress (this shape also fails status ordering).
	if err := thread.updatePlan([]reactPlanStep{{"Research", "in_progress"}, {"Distill", "completed"}, {"Implement", "completed"}}); err == nil {
		t.Fatal("completed step must not regress")
	}
	// A genuinely new step is appended at the end.
	if err := thread.updatePlan([]reactPlanStep{
		{"Research", "completed"}, {"Distill", "completed"}, {"Implement", "completed"}, {"Verify", "in_progress"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(thread.plan) != 4 || thread.plan[3].Step != "Verify" {
		t.Fatalf("new step not appended: %+v", thread.plan)
	}
}

func TestCanAutoClosePlan(t *testing.T) {
	cases := []struct {
		name string
		plan []reactPlanStep
		want bool
	}{
		{"empty", nil, false},
		{"all completed", []reactPlanStep{{"A", "completed"}, {"B", "completed"}}, true},
		{"final in_progress", []reactPlanStep{{"A", "completed"}, {"B", "in_progress"}}, true},
		{"final pending never started", []reactPlanStep{{"A", "completed"}, {"B", "pending"}}, false},
		{"middle pending", []reactPlanStep{{"A", "completed"}, {"B", "pending"}, {"C", "in_progress"}}, false},
		{"middle in_progress", []reactPlanStep{{"A", "in_progress"}, {"B", "pending"}}, false},
	}
	for _, c := range cases {
		if got := canAutoClosePlan(c.plan); got != c.want {
			t.Fatalf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	// markFinalStepComplete closes only the final in_progress step.
	thread := &reactThread{plan: []reactPlanStep{{"A", "completed"}, {"B", "in_progress"}}}
	thread.markFinalStepComplete()
	if thread.plan[0].Status != "completed" || thread.plan[1].Status != "completed" {
		t.Fatalf("markFinalStepComplete: %+v", thread.plan)
	}
}
