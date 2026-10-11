package adapter

import (
	"strings"
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/pipeline"
	"voicesign-harness/tools"
)

func baseTestOptions(t *testing.T) *pipeline.Options {
	t.Helper()
	reg, err := tools.LoadContracts("") // built-in contracts only, no on-disk dir
	if err != nil {
		t.Fatalf("LoadContracts: %v", err)
	}
	return &pipeline.Options{Tools: reg, Exec: &tools.Executor{BaseDir: t.TempDir()}}
}

// The template adapter binds its tool and the handler runs end to end through
// the same Exec path as a built-in tool.
func TestExampleAdapterApplyAndExec(t *testing.T) {
	o := baseTestOptions(t)
	a := NewExampleAdapter()
	if err := Apply(o, a); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	c, ok := o.Tools.Get("customer_echo")
	if !ok {
		t.Fatal("customer_echo contract not bound after Apply")
	}
	if c.Source != "customer" {
		t.Fatalf("Source=%q, want customer", c.Source)
	}
	recv, err := o.Exec.Exec("customer_echo", map[string]any{"text": "hello"}, c)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !recv.OK || !strings.Contains(recv.Stdout, "echo: hello") {
		t.Fatalf("unexpected receipt: %+v", recv)
	}
}

// Apply must fail closed on every malformed input.
func TestApplyRejects(t *testing.T) {
	good := NewExampleAdapter()

	if err := Apply(nil, good); err == nil {
		t.Fatal("want error for nil Options")
	}
	if err := Apply(baseTestOptions(t), nil); err == nil {
		t.Fatal("want error for nil adapter")
	}
	if err := Apply(baseTestOptions(t), fakeAdapter{meta: Meta{CustomerID: "  "}}); err == nil {
		t.Fatal("want error for empty customer id")
	}
	if err := Apply(&pipeline.Options{}, good); err == nil {
		t.Fatal("want error when Tools/Exec are not initialized")
	}

	noHandler := fakeAdapter{meta: Meta{CustomerID: "c"}, tools: []ToolBinding{{
		Contract: contract.ToolContract{Name: "nh", Version: "1.0", Caps: []string{"c"},
			Params: map[string]string{"p": "string"}, Risk: map[string]string{"c": "low"}},
	}}}
	if err := Apply(baseTestOptions(t), noHandler); err == nil {
		t.Fatal("want error for nil handler")
	}

	dup := fakeAdapter{meta: Meta{CustomerID: "c"}, tools: []ToolBinding{
		bindingForTest("dup_tool"), bindingForTest("dup_tool"),
	}}
	if err := Apply(baseTestOptions(t), dup); err == nil {
		t.Fatal("want error for duplicate tool")
	}

	shadow := fakeAdapter{meta: Meta{CustomerID: "c"}, tools: []ToolBinding{{
		Contract: contract.ToolContract{Name: "git", Version: "1.0", Caps: []string{"c"},
			Params: map[string]string{"p": "string"}, Risk: map[string]string{"c": "low"}},
		Handler: func(map[string]any) (string, error) { return "", nil },
	}}}
	if err := Apply(baseTestOptions(t), shadow); err == nil {
		t.Fatal("want error when shadowing built-in tool git")
	}
}

func TestExampleAdapterHasAcceptanceCase(t *testing.T) {
	if cases := NewExampleAdapter().AcceptanceCases(); len(cases) != 1 {
		t.Fatalf("AcceptanceCases=%d, want 1 template case", len(cases))
	}
}

// fakeAdapter is a malformed/ad-hoc adapter for rejection tests.
type fakeAdapter struct {
	meta  Meta
	tools []ToolBinding
}

func (f fakeAdapter) Meta() Meta                       { return f.meta }
func (f fakeAdapter) Tools() []ToolBinding             { return f.tools }
func (f fakeAdapter) AcceptanceCases() []AcceptanceCase { return nil }

func bindingForTest(name string) ToolBinding {
	return ToolBinding{
		Contract: contract.ToolContract{
			Name: name, Version: "1.0", Caps: []string{"c"},
			Params: map[string]string{"p": "string"}, Risk: map[string]string{"c": "low"},
		},
		Handler: func(map[string]any) (string, error) { return "ok", nil },
	}
}
