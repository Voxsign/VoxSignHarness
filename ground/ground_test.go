package ground

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/space"
)

func TestRenderMissingDataSources(t *testing.T) {
	dir := t.TempDir()
	g := New(dir, nil)
	snap := g.Render()
	if snap.Block == "" {
		t.Fatal("empty data source should still render a placeholder block")
	}
	if !strings.Contains(snap.Block, "(no registered domain)") || !strings.Contains(snap.Block, "(none)") {
		t.Fatalf("missing data should render placeholders, not error: %q", snap.Block)
	}
}

func TestRecordAndRenderDecisions(t *testing.T) {
	dir := t.TempDir()
	proj := t.TempDir()
	_ = os.WriteFile(filepath.Join(proj, "main.go"), []byte("package main"), 0o644)

	spaces, _ := space.Load(t.TempDir())
	_ = spaces.Add(&space.Manifest{
		Name: "proj", Type: space.TypeProject, Scope: []string{proj},
		Tools: []string{"file"}, Perms: space.Perms{Read: true, Write: true},
	})

	g := New(dir, spaces)
	if err := g.RecordDecision(Decision{Ts: "2026-10-02T10:00:00+03:00", TaskID: "r1", Intent: "EDIT", Decision: "light", Confirm: "approved", Reason: "可逆+中"}); err != nil {
		t.Fatal(err)
	}
	if err := g.RecordDecision(Decision{Ts: "2026-10-02T10:01:00+03:00", TaskID: "r2", Intent: "COMMIT", Decision: "human", Confirm: "rejected", Reason: "不可逆"}); err != nil {
		t.Fatal(err)
	}

	snap := g.Render()
	if len(snap.Decisions) != 2 {
		t.Fatalf("应读出 2 条裁决, got %d", len(snap.Decisions))
	}
	if !strings.Contains(snap.Block, "project-map:proj") {
		t.Fatalf("project-map 应含 proj: %q", snap.Block)
	}
	if !strings.Contains(snap.Block, "main.go") {
		t.Fatalf("scope 清单应含 main.go: %q", snap.Block)
	}
	if !strings.Contains(snap.Block, "COMMIT human/rejected") {
		t.Fatalf("裁决块应含最近裁决: %q", snap.Block)
	}
}

func TestTruncateOldestDecisions(t *testing.T) {
	dir := t.TempDir()
	g := New(dir, nil)
	g.MaxDecisions = 3
	for i := 0; i < 5; i++ {
		_ = g.RecordDecision(Decision{Ts: "2026-10-02T10:0" + string(rune('0'+i)) + ":00+03:00", TaskID: "x"})
	}
	snap := g.Render()
	if len(snap.Decisions) != 3 {
		t.Fatalf("应截断到 3 条, got %d", len(snap.Decisions))
	}
	//     be disconnect, keep     
	if strings.Contains(snap.Block, "10:00") || strings.Contains(snap.Block, "10:01") {
		t.Fatalf("最旧裁决应被截断: %q", snap.Block)
	}
}

func TestByteCap(t *testing.T) {
	dir := t.TempDir()
	g := New(dir, nil)
	g.MaxBytes = 50
	_ = g.RecordDecision(Decision{Ts: "2026-10-02T10:00:00+03:00", TaskID: strings.Repeat("a", 200), Reason: strings.Repeat("x", 200)})
	snap := g.Render()
	if len(snap.Block) > 120 { //  disconnecttgt    
		t.Fatalf("块应按字节截断: len=%d", len(snap.Block))
	}
}
