package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"voicesign-harness/contract"
)

func TestAuditOverflowPreservesUnfinished(t *testing.T) {
	thread := &reactThread{goal: "implement"}
	for i := 0; i < 8; i++ {
		thread.plan = append(thread.plan, reactPlanStep{fmt.Sprint(i), "pending"})
	}
	before := append([]reactPlanStep(nil), thread.plan...)
	if err := thread.updatePlan([]reactPlanStep{{"replacement", "completed"}}); err == nil {
		t.Fatal("overflow erased blocking steps")
	}
	if !reflect.DeepEqual(before, thread.plan) {
		t.Fatal("rejected update mutated plan")
	}
	thread.plan[0].Status = "completed"
	if err := thread.updatePlan([]reactPlanStep{{"replacement", "pending"}}); err == nil {
		t.Fatal("one completed step cannot free capacity")
	}
}

func TestAuditCosmeticCoreAndMissingBaseline(t *testing.T) {
	dir := t.TempDir()
	original := "package p\nfunc F() int { return 1 }\n"
	abs := filepath.Join(dir, "x.go")
	testAbs := filepath.Join(dir, "x_test.go")
	if err := os.WriteFile(abs, []byte("package p\nfunc F() int {return 1}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testAbs, []byte("package p\nvar TestOnly = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	snap := map[string]string{abs: original, testAbs: ""}
	if !cosmeticOnly(dir, []string{"x.go", "x_test.go"}, snap) {
		t.Fatal("test change plus formatting qualified as logic")
	}
	if err := os.WriteFile(abs, []byte("package p\nfunc F() int { return 2 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	delete(snap, testAbs)
	if !cosmeticOnly(dir, []string{"x.go", "x_test.go"}, snap) {
		t.Fatal("missing baseline accepted after real edit")
	}
}

func TestAuditGateRequiresDoneAndCompiledChange(t *testing.T) {
	for _, kind := range []string{"no-done", "ignored"} {
		t.Run(kind, func(t *testing.T) {
			dir := selfImproveTestRepo(t)
			name := "x.go"
			if kind == "ignored" {
				name = "_ignored.go"
			}
			abs := filepath.Join(dir, name)
			if err := os.WriteFile(abs, []byte("package testrepo\nvar X = 1\n"), 0600); err != nil {
				t.Fatal(err)
			}
			// Keep a normal file so a build succeeds even when the change is ignored.
			if err := os.WriteFile(filepath.Join(dir, "base.go"), []byte("package testrepo\n"), 0600); err != nil {
				t.Fatal(err)
			}
			o, _ := selfImproveTestOptions(t, []string{"LLM_ERROR"})
			thread := &reactThread{system: contract.Message{Role: "system", Content: "test"}, recon: contract.Message{Role: "user", Content: "test"}, goal: "implement", plan: []reactPlanStep{{"Work", "completed"}}}
			var recv []contract.Receipt
			seq := 0
			result := o.selfImproveBuildTestGate(context.Background(), dir, t.TempDir(), []string{name}, true, map[string]string{abs: "package testrepo\nvar X = 0\n"}, thread, &recv, &seq, kind != "no-done")
			if result.passed {
				t.Fatalf("%s passed: %+v", kind, result)
			}
		})
	}
}

func TestAuditShellTracksTestWeakening(t *testing.T) {
	dir := selfImproveTestRepo(t)
	abs := filepath.Join(dir, "x_test.go")
	source := "package testrepo\nimport \"testing\"\nfunc TestX(t *testing.T) { t.Fatal(\"fail\") }\n"
	if err := os.WriteFile(abs, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	o, _ := selfImproveTestOptions(t, nil)
	snap := map[string]string{abs: source}
	_, ok, touched := o.executeReactAction(dir, t.TempDir(), reactAction{Tool: "run", Command: []string{"sh", "-c", "printf 'package testrepo\\n' > x_test.go"}}, snap)
	if !ok || !reflect.DeepEqual(touched, []string{"x_test.go"}) {
		t.Fatalf("shell edit not tracked: %v %v", ok, touched)
	}
	if err := validateChangedForGate(dir, append(touched, "x.go"), snap); err == nil {
		t.Fatal("shell test weakening passed")
	}
}

func TestAuditUnknownObjectiveRequiresCode(t *testing.T) {
	for _, goal := range []string{"add retries", "optimize parser", "build a cache", "extend backend", "research then add retries"} {
		if !wantsCodeChange(goal) {
			t.Fatalf("%q permits zero edits", goal)
		}
	}
}

func TestAuditBlankAssignIsNotLogic(t *testing.T) {
	dir := t.TempDir()
	original := "package p\nfunc F() int { return 1 }\n"
	abs := filepath.Join(dir, "x.go")
	if err := os.WriteFile(abs, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	snap := map[string]string{abs: original}
	// A token-only dead-code edit must not qualify as a logic change (remaining P1).
	if err := os.WriteFile(abs, []byte(original+"var _ = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if !cosmeticOnly(dir, []string{"x.go"}, snap) {
		t.Fatal("blank assignment qualified as logic change")
	}
	// A real behavior change still qualifies.
	if err := os.WriteFile(abs, []byte("package p\nfunc F() int { return 2 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if cosmeticOnly(dir, []string{"x.go"}, snap) {
		t.Fatal("real logic change was treated as cosmetic")
	}
}

func TestStripBlankAssigns(t *testing.T) {
	// A pure dead-code blank assignment is removed.
	if got := stripBlankAssigns("package p\nvar _ = 1\n"); got != "package p\n" {
		t.Fatalf("pure var blank should be stripped, got %q", got)
	}
	// A blank assignment whose RHS calls something may have a side effect: keep it.
	if got := stripBlankAssigns("package p\nfunc f(){ _ = g() }\n"); !strings.Contains(got, "g()") {
		t.Fatalf("side-effecting blank call must be kept, got %q", got)
	}
	if got := stripBlankAssigns("package p\nfunc f(){ _ = a[0]() }\n"); !strings.Contains(got, "a[0]()") {
		t.Fatalf("index-call blank must be kept, got %q", got)
	}
	// A pure blank statement mid-line is removed but a later statement on the same
	// line is preserved (final-review blocker #3: never mask a real adjacent edit).
	got := stripBlankAssigns("package p\nfunc f() error { if true { _ = 1; return nil } }\n")
	if strings.Contains(got, "_ = 1") {
		t.Fatalf("pure blank should be removed, got %q", got)
	}
	if !strings.Contains(got, "return nil") {
		t.Fatalf("later statement on the same line must be kept, got %q", got)
	}
}
