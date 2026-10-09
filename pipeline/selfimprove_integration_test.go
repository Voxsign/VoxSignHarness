package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/provider"
	"voicesign-harness/tools"
)

func selfImproveTestOptions(t *testing.T, actions []string) (*Options, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []contract.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		for i := 1; i < len(req.Messages); i++ {
			if req.Messages[i].Role == "user" && req.Messages[i-1].Role == "user" {
				t.Error("consecutive user messages")
			}
		}
		if len(req.Messages) < 2 || !strings.Contains(req.Messages[1].Content, "PERSISTENT GOAL") {
			t.Error("missing persistent plan context")
		}
		action := `{"done":true}`
		if calls < len(actions) {
			action = actions[calls]
		}
		calls++
		if action == "LLM_ERROR" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": action}}}})
	}))
	t.Cleanup(srv.Close)
	cfg := &config.Config{Global: config.Global{LLMTimeoutMs: 5000, FastResponseMs: 5000}, Providers: []config.Provider{{Name: "fast", Kind: config.OpenAIKind, Endpoint: srv.URL, Model: "test", APIKey: "test"}}}
	reg, err := provider.NewRegistry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &Options{Cfg: cfg, Providers: reg, Tools: &tools.Registry{Contracts: map[string]contract.ToolContract{}}, Exec: &tools.Executor{}}, &calls
}

func selfImproveTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testrepo\n\ngo 1.22\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSelfImproveMainLoop(t *testing.T) {
	plan := `{"tool":"plan","steps":[{"step":"Work","status":"pending"}]}`
	complete := `{"tool":"plan","steps":[{"step":"Work","status":"completed"}]}`
	write := `{"tool":"write","path":"x.go","content":"package testrepo\nvar X = 1\n"}`
	for _, scenario := range []string{"budget", "done-continues", "repair", "llm-error", "parse-failures", "done-cap"} {
		t.Run(scenario, func(t *testing.T) {
			dir := selfImproveTestRepo(t)
			t.Setenv("VHS_SELF_REPO", dir)
			actions := []string{plan, write}
			switch scenario {
			case "budget":
				for len(actions) < selfImproveMaxSteps {
					actions = append(actions, `{"tool":"read","path":"x.go"}`)
				}
			case "done-continues":
				actions = []string{plan, `{"done":true}`, complete, `{"done":true}`}
			case "repair":
				// Two free plan updates plus one repair exceed the old two-action budget.
				paddedPlan := strings.Replace(complete, `"plan"`, `" plan"`, 1)
				actions = []string{plan, `{"tool":"write","path":"x.go","content":"package testrepo\nvar X =\n"}`, complete, `{"done":true}`, complete, paddedPlan, `{"tool":"replace","path":"x.go","old":"var X =","new":"var X = 1"}`}
			case "parse-failures":
				actions = append(actions, "bad", "bad", "bad")
			case "done-cap":
				actions = []string{plan, `{"done":true}`, `{"done":true}`}
			case "llm-error":
				actions = append(actions, "LLM_ERROR")
			}
			o, calls := selfImproveTestOptions(t, actions)
			receipts := o.execSelfImprove(context.Background(), contract.Intent{CorrectedText: "research"}, t.TempDir())
			final := receipts[len(receipts)-1]
			wantOK := scenario == "done-continues" || scenario == "repair"
			if final.OK != wantOK {
				t.Fatalf("final=%+v receipts=%+v", final, receipts)
			}
			if scenario == "budget" || scenario == "parse-failures" || scenario == "llm-error" {
				verified := false
				for _, r := range receipts {
					if r.Tool == "selfimprove.test" && r.OK {
						verified = true
					}
				}
				if !verified || !strings.Contains(final.Err, "plan:") {
					t.Fatalf("unverified edits or missing plan result: %+v", receipts)
				}
			}
			if scenario == "done-cap" && *calls != 3 {
				t.Fatalf("done rejection calls=%d", *calls)
			}
			if scenario == "repair" && *calls != 7 {
				t.Fatalf("repair plan consumed budget: %d", *calls)
			}
			if scenario == "repair" {
				builds := 0
				for _, r := range receipts {
					if r.Tool == "selfimprove.build" {
						builds++
					}
				}
				if builds != 2 {
					t.Fatalf("plan triggered rebuild: got %d builds, want initial and repaired", builds)
				}
			}
		})
	}
}

func TestReactPlanMalformedAndMigration(t *testing.T) {
	for _, raw := range []string{`{"tool":"plan","steps":["a"]}`, `{"tool":"plan","steps":[{"step":"a"}]}`, `{"tool":"plan","steps":[{"step":"a","status":"Pending"}]}`} {
		if _, err := parseReactAction(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	a, err := parseReactAction(`{"tool":"write","path":"leaked","steps":["bad"]} {"done":true}`)
	if err != nil || a.Path != "" || a.Tool != "" || !a.Done {
		t.Fatalf("candidate leakage: %+v %v", a, err)
	}
	thread := &reactThread{}
	if err := thread.updatePlan([]reactPlanStep{{"A", "completed"}, {"B", "pending"}}); err != nil {
		t.Fatal(err)
	}
	for _, steps := range [][]reactPlanStep{
		{{"A", "pending"}, {"B", "pending"}}, {{"A", "completed"}}, {{"A", "completed"}, {"B", "pending"}, {" b ", "pending"}}, {{"A", "completed"}, {"B\t", "pending"}},
	} {
		if err := thread.updatePlan(steps); err == nil {
			t.Errorf("accepted %+v", steps)
		}
	}
	if err := (&reactThread{}).updatePlan([]reactPlanStep{{"A", "pending"}, {"B", "pending"}}); err != nil {
		t.Fatal(err)
	}
}

func TestSelfImproveGuardAndNilSnapshot(t *testing.T) {
	dir := selfImproveTestRepo(t)
	o := &Options{Tools: &tools.Registry{Contracts: map[string]contract.ToolContract{}}, Exec: &tools.Executor{}}
	for _, tool := range []string{"write", "replace"} {
		obs, ok, _ := o.executeReactAction(dir, "", reactAction{Tool: tool, Path: "pipeline/selfimprove_guard.go", Old: "package", Content: "package p"}, nil)
		if ok || !strings.Contains(obs.Err, "护栏") {
			t.Fatalf("guard %s: %+v", tool, obs)
		}
	}
	copied := filepath.Join(dir, "copied.go")
	if err := os.WriteFile(copied, []byte("package testrepo\nfunc\twithinRepo () {}"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"write", "replace"} {
		_, ok, _ := o.executeReactAction(dir, "", reactAction{Tool: tool, Path: "copied.go", Old: "withinRepo", New: "other"}, nil)
		if ok {
			t.Fatal("guard definition copy accepted")
		}
	}
	for _, path := range []string{"selfimprove.go", "selfimprove_plan.go"} {
		if _, err := os.ReadFile(path); err != nil {
			t.Fatal(err)
		}
		if protectedGuardFile(path) || editSafetyError(path) != "" {
			t.Fatalf("engine frozen: %s", path)
		}
	}
	if _, err := os.ReadFile("selfimprove_guard.go"); err != nil {
		t.Fatal(err)
	}
	if !protectedGuardFile("selfimprove_guard.go") {
		t.Fatal("real guard file unprotected")
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, ok := withinRepo(dir, "escape/new.go"); ok {
		t.Fatal("symlink escape")
	}
	for _, tool := range []string{"write", "replace"} {
		a := reactAction{Tool: tool, Path: "x.go", Content: "package testrepo\nvar X=1", Old: "X=1", New: "X=2"}
		if obs, ok, _ := o.executeReactAction(dir, "", a, nil); !ok {
			t.Fatal(fmt.Sprintf("nil snapshot %s: %+v", tool, obs))
		}
	}
}

func TestSelfImproveGuardShellWrites(t *testing.T) {
	o := &Options{}
	for _, command := range []string{
		`sed -i 's/package/other/' pipeline/selfimprove_guard.go`,
		`echo x | tee pipeline/selfimprove_guard.go`,
		`cat >pipeline/selfimprove_guard.go <<EOF`,
		`mv replacement.go pipeline/selfimprove_guard.go`,
		`dd of=pipeline/selfimprove_guard.go`,
		`perl -i -pe 's/package/other/' pipeline/selfimprove_guard.go`,
	} {
		t.Run(command, func(t *testing.T) {
			for _, argv := range [][]string{{"sh", "-c", command}, strings.Fields(command)} {
				obs, ok, _ := o.executeReactAction(t.TempDir(), "", reactAction{Tool: "run", Command: argv}, nil)
				if ok || !strings.Contains(obs.Err, "write to selfimprove_guard.go") {
					t.Fatalf("guard write accepted: %v: %+v", argv, obs)
				}
			}
		})
	}
	for _, argv := range [][]string{
		{"cat", "pipeline/selfimprove_guard.go"},
		{"sh", "-c", `grep -n dangerousCommand pipeline/selfimprove_guard.go`},
		{"sed", "-i", "s/X/Y/", "pipeline/selfimprove.go"},
	} {
		if bad := dangerousCommand(argv); bad != "" {
			t.Fatalf("legitimate command rejected: %v: %s", argv, bad)
		}
	}
}

func TestSelfImproveRepairOutcomes(t *testing.T) {
	for _, scenario := range []string{"pending-plan", "failed-repair", "abandoned-repair", "parse-repair"} {
		t.Run(scenario, func(t *testing.T) {
			dir := selfImproveTestRepo(t)
			abs := filepath.Join(dir, "x.go")
			if err := os.WriteFile(abs, []byte("package testrepo\nvar X =\n"), 0600); err != nil {
				t.Fatal(err)
			}
			thread := &reactThread{goal: "repair", recon: contract.Message{Role: "user"}, system: contract.Message{Role: "system"}}
			status := "completed"
			if scenario == "pending-plan" {
				status = "pending"
			}
			if err := thread.updatePlan([]reactPlanStep{{"Work", status}}); err != nil {
				t.Fatal(err)
			}
			repair := `{"tool":"replace","path":"x.go","old":"var X =","new":"var X = 1"}`
			actions := []string{`{"tool":"plan","steps":[{"step":"Work","status":"pending"}]}`, repair}
			switch scenario {
			case "failed-repair":
				actions = []string{`{"tool":"replace","path":"x.go","old":"not found","new":"x"}`, `{"tool":"read","path":"x.go"}`}
			case "abandoned-repair":
				actions = []string{`{"done":true,"summary":"blocked"}`}
			case "parse-repair":
				actions = []string{"bad", repair}
			}
			o, calls := selfImproveTestOptions(t, actions)
			seq := 0
			var receipts []contract.Receipt
			result := o.selfImproveBuildTestGate(context.Background(), dir, t.TempDir(), []string{"x.go"}, true, map[string]string{abs: "package testrepo\nvar X = 0\n"}, thread, &receipts, &seq)
			if result.passed != (scenario == "parse-repair") {
				t.Fatalf("unexpected result %+v", result)
			}
			if scenario == "pending-plan" {
				if *calls != 2 || !strings.Contains(result.detail, "plan:") || !strings.Contains(result.detail, "go test") {
					t.Fatalf("plan blocked repair: %+v calls=%d", result, *calls)
				}
			}
		})
	}
}
