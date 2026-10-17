package pipeline

// TEMPORARY Phase 0 smoke test — verifies stage/LLM timing is actually produced.
// (kept out of the PR unless the Controller wants it as a permanent test)

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"voicesign-harness/cache"
	"voicesign-harness/config"
	"voicesign-harness/memory"
	"voicesign-harness/provider"
	"voicesign-harness/refer"
	"voicesign-harness/space"
	"voicesign-harness/tools"
	"voicesign-harness/trajectory"
	"voicesign-harness/verify"
)

func TestTimingSmoke(t *testing.T) {
	logDir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = logDir
	cfg.Memory.Dir = filepath.Join(logDir, "mem")
	cfg.Spaces.Dir = filepath.Join(logDir, "spaces")
	cfg.Contracts.Dir = filepath.Join(logDir, "contracts")
	cfg.Cache.Dir = filepath.Join(logDir, "cache")
	for _, d := range []string{cfg.Memory.Dir, cfg.Spaces.Dir, cfg.Contracts.Dir, cfg.Cache.Dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// mock-only registry: no external calls in this test.
	cfg.Providers = []config.Provider{{Name: "mock", Kind: config.MockKind, Model: "mock"}}
	reg, err := provider.NewRegistry(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	dict, _ := memory.LoadDictionary(filepath.Join(cfg.Memory.Dir, "dictionary.json"))
	spaces, _ := space.Load(cfg.Spaces.Dir)
	treg, _ := tools.LoadContracts(cfg.Contracts.Dir)
	tr, _ := trajectory.Open(cfg.Global.LogDir)
	st, _ := cache.Open(filepath.Join(cfg.Cache.Dir, "quad.json"), time.Hour)
	opts := &Options{
		Cfg:       &cfg,
		Dict:      dict,
		Spaces:    spaces,
		Refer:     refer.New(dict),
		Cache:     st,
		Providers: reg,
		Tools:     treg,
		Exec:      &tools.Executor{BaseDir: logDir},
		Verifier:  &verify.Verifier{BaseDir: logDir},
		ConfirmFn: func(string, string) (bool, error) { return true, nil },
		Trace:     tr,
		ConvID:    "smoke",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := Run(ctx, opts, "记录一条测试笔记：今天完成插桩验证")
	if err != nil {
		t.Fatal(err)
	}
	if out.Timing == nil {
		t.Fatal("out.Timing is nil — instrumentation not attached")
	}
	if len(out.Timing.Stages) == 0 {
		t.Fatal("no stages recorded")
	}
	if len(out.Timing.LLMCalls) == 0 {
		t.Fatal("no llm calls recorded")
	}
	t.Logf("timing ok: stages=%d llm_calls=%d total_ms=%d", len(out.Timing.Stages), len(out.Timing.LLMCalls), out.Timing.TotalMs)
	for _, s := range out.Timing.Stages {
		t.Logf("  stage %-12s %dms", s.Name, s.Ms)
	}
	for _, c := range out.Timing.LLMCalls {
		t.Logf("  llm %-10s %dms fin=%s", c.Provider, c.Ms, c.FinishReason)
	}
}

func TestWrappedRegistry(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = []config.Provider{{Name: "mock", Kind: config.MockKind, Model: "mock"}}
	reg, err := provider.NewRegistry(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	w := reg.Wrapped(func(name string, resp provider.ChatResponse, ms int64) {
		calls = append(calls, name)
	})
	wp, werr := w.Get("mock")
	if werr != nil {
		t.Fatal(werr)
	}
	if _, err := wp.Chat(context.Background(), provider.ChatRequest{}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "mock" {
		t.Fatalf("expected 1 observed call on mock, got %v", calls)
	}
	// receiver must not be mutated
	rp, rerr := reg.Get("mock")
	if rerr != nil {
		t.Fatal(rerr)
	}
	if _, err := rp.Chat(context.Background(), provider.ChatRequest{}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("Wrapped mutated the receiver registry: %v", calls)
	}
}
