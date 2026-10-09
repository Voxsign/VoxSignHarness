package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// R13-P2 (2026-10-09): recentResumableGoal — cross-session long-task resume.
// A goal left Active/Paused in another conversation must be found and returned
// as the most recently updated resumable one; Completed/Canceled are skipped.
func TestRecentResumableGoal(t *testing.T) {
	dir := t.TempDir()
	slots := filepath.Join(dir, "context_slots")
	if err := os.MkdirAll(slots, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, status, updated string) {
		g := Goal{Status: status, UpdatedAt: updated, Objective: name, IntentKind: "INSTALL"}
		if b, err := json.Marshal(g); err != nil {
			t.Fatal(err)
		} else if err := os.WriteFile(filepath.Join(slots, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("goal-a.json", GoalActive, "2026-10-09T10:00:00Z")
	write("goal-b.json", GoalPaused, "2026-10-09T11:00:00Z") // newer + resumable -> winner
	write("goal-c.json", GoalCompleted, "2026-10-09T12:00:00Z")
	write("goal-d.json", GoalCanceled, "2026-10-09T13:00:00Z")

	got := recentResumableGoal(dir)
	if got == nil {
		t.Fatal("expected a resumable goal")
	}
	if got.Objective != "goal-b.json" {
		t.Fatalf("expected most-recent Active/Paused goal-b.json, got %q", got.Objective)
	}
	// Empty dir -> nil.
	empty := t.TempDir()
	if got := recentResumableGoal(empty); got != nil {
		t.Fatalf("expected nil for empty dir, got %+v", got)
	}
}
