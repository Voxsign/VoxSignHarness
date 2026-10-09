package pipeline

// R13 (2026-10-09): unit coverage for the declarative governance constitution
// (defaults, JSON override merge, confirm-mode derivation) and allow-once memory.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConstitutionModes(t *testing.T) {
	c := defaultConstitution()
	cases := []struct{ key, want string }{
		{"install", "always"},
		{"email:reply", "once"},
		{"email:forward", "once"},
		{"unknown-tool", "always"}, // safe default: never silently drop a gate
	}
	for _, cse := range cases {
		if got := c.confirmMode(cse.key); got != cse.want {
			t.Fatalf("confirmMode(%q)=%q want %q", cse.key, got, cse.want)
		}
	}
}

func TestLoadConstitutionOverrideMerge(t *testing.T) {
	logDir := t.TempDir()
	// override email:reply -> always; defaults must still fill install.
	over := map[string]any{
		"version": "1",
		"tools":   map[string]any{"email:reply": map[string]any{"risk": "high", "confirm": "always"}},
	}
	b, _ := json.Marshal(over)
	if err := os.WriteFile(filepath.Join(logDir, "constitution.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	c := loadConstitution(logDir)
	if got := c.confirmMode("email:reply"); got != "always" {
		t.Fatalf("override failed: got %q want always", got)
	}
	if got := c.confirmMode("install"); got != "always" {
		t.Fatalf("default not preserved: got %q want always", got)
	}
	if got := c.confirmMode("email:forward"); got != "once" {
		t.Fatalf("default not preserved: got %q want once", got)
	}
}

func TestAllowOnceLifecycle(t *testing.T) {
	logDir := t.TempDir()
	conv := "a1"
	if isAllowed(logDir, conv, "email:reply:Sofia") {
		t.Fatal("fresh session must not be allowed")
	}
	recordAllow(logDir, conv, "email:reply:Sofia")
	if !isAllowed(logDir, conv, "email:reply:Sofia") {
		t.Fatal("allow must be visible after record")
	}
	if isAllowed(logDir, conv, "email:reply:Abdullah") {
		t.Fatal("different target must not be allowed")
	}
}

func TestAllowFileFallbackConvID(t *testing.T) {
	logDir := t.TempDir()
	recordAllow(logDir, "", "email:reply:X")
	if _, err := os.Stat(allowedPath(logDir, "default")); err != nil {
		t.Fatalf("empty convID must fall back to default file: %v", err)
	}
}
