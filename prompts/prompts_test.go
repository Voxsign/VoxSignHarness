package prompts

import (
	"strings"
	"testing"
)

func TestBuilderOrderAndInterpolation(t *testing.T) {
	b := New().
		Add("identity", -1000, "身份：你是 VoxSign 助手。").
		Add("persona", 0, "人设：口语化。").
		AddDynamic("runtime", 900, func() string { return "上下文：本快照取代早期快照。" }).
		Add("rules", 1000, "规则：不要说没理解。")
	out := b.Render()
	if !strings.HasPrefix(out, "身份：你是 VoxSign 助手。") {
		t.Fatalf("identity must lead, got: %s", out)
	}
	if strings.Index(out, "身份") > strings.Index(out, "人设") {
		t.Fatalf("order violated: %s", out)
	}
	if strings.Index(out, "规则") < strings.Index(out, "上下文") {
		t.Fatalf("rules must follow runtime context: %s", out)
	}
}

func TestBuilderDropsEmptySections(t *testing.T) {
	b := New().
		Add("identity", -1000, "身份。").
		Add("empty", 0, "   \n  ").
		AddDynamic("nil-dynamic", 100, func() string { return "" }).
		Add("tail", 200, "尾部。")
	out := b.Render()
	if strings.Contains(out, "empty") || strings.Contains(out, "nil-dynamic") {
		t.Fatalf("empty sections must be dropped: %q", out)
	}
	if !strings.Contains(out, "尾部。") {
		t.Fatalf("non-empty tail section missing: %q", out)
	}
	if strings.Count(out, "\n\n") != 1 {
		t.Fatalf("sections must join with single \\n\\n separators: %q", out)
	}
}

func TestBuilderAllDynamicEmpty(t *testing.T) {
	b := New().AddDynamic("only", 0, func() string { return "" })
	if b.Render() != "" {
		t.Fatalf("all-empty assembly must render empty string")
	}
}
