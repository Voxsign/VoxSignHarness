package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"voicesign-harness/contract"
)

// Stable-identity evidence and persistence hard-gate tests (P0 round 4). These
// pin the exact failure Codex found in round 3: raw note-text substring matching
// could attribute an earlier legitimate identical note (same goal's second
// identical step, or a different goal) to the crashed step, and a swallowed
// persistence write could either skip the side effect silently or claim success
// after the side effect when the dequeue was not saved. All side effects are
// confined to t.TempDir(); the real global notebook is guarded.

// seedNoteHead persists an active goal with one NOTE head step, optionally with
// a write-ahead lease already on disk (the post-crash state).
func seedNoteHead(t *testing.T, conv, logDir, goalID string, withLease bool) {
	t.Helper()
	g := &Goal{
		Objective: "证据序列", Status: GoalActive, GoalID: goalID, StepCounter: 1,
		Queue: []string{crashNote}, QueueIDs: []string{goalID + ".s1"}, HeadAttempt: 1,
	}
	if withLease {
		g.StepSeq = 1
		g.StepLease = &StepLease{
			GoalID: goalID, StepID: goalID + ".s1", Attempt: 1, Seq: 1,
			Text: crashNote, Intent: contract.IntentNote, StartedAt: "2026-10-11T00:00:00Z",
		}
	}
	saveGoal(logDir, conv, g)
}

func writeRawNoteLine(t *testing.T, logDir, line string) {
	t.Helper()
	path := filepath.Join(logDir, "notes.md")
	if err := os.WriteFile(path, []byte("\n"+line+"\n"), 0o644); err != nil {
		t.Fatalf("seed notes.md: %v", err)
	}
}

// A pre-existing IDENTICAL note with NO current-step token must not be taken as
// evidence that the crashed step landed: the step is re-run under a fresh
// attempt token exactly once, and the historical line is left intact.
func TestEvidence_PreexistingIdenticalNoteNotClaimed(t *testing.T) {
	conv, logDir := "default", t.TempDir()
	gEx, gSize, gMtime := globalNotesStat(t)
	o := queueTestOptions(t, conv, logDir)
	defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)

	seedNoteHead(t, conv, logDir, "g1", true)
	// Historical, legitimate identical note, no current-step token.
	writeRawNoteLine(t, logDir, "- 2026-10-11 08:00 "+crashNote)

	recvs := o.advanceGoalQueue(context.Background(), logDir, nil)
	if hasConfirmAsk(recvs) {
		t.Fatalf("a local NOTE crash must reconcile, not hold a gate; recvs=%+v", recvs)
	}
	if c := countAttemptTokens(t, logDir, "g1", "g1.s1", 1); c != 0 {
		t.Fatalf("no a1 token may exist: the historical line must not be claimed; got %d", c)
	}
	if c := countAttemptTokens(t, logDir, "g1", "g1.s1", 2); c != 1 {
		t.Fatalf("step must be re-run exactly once under a2; got %d a2 tokens", c)
	}
	if n := noteLines(t, logDir); n != 2 {
		t.Fatalf("historical line kept + one fresh line = 2, got %d", n)
	}
	g := loadGoal(logDir, conv)
	if g.StepLease != nil || len(g.Queue) != 0 {
		t.Fatalf("step must finish consumed; lease=%+v queue=%v", g.StepLease, g.Queue)
	}
}

// An identical note carrying a DIFFERENT goal's token must never satisfy this
// goal's crashed step; the foreign line is preserved, and only the local step's
// own fresh token proves completion.
func TestEvidence_ForeignGoalTokenNotClaimed(t *testing.T) {
	conv, logDir := "default", t.TempDir()
	gEx, gSize, gMtime := globalNotesStat(t)
	o := queueTestOptions(t, conv, logDir)
	defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)

	seedNoteHead(t, conv, logDir, "g3", true)
	writeRawNoteLine(t, logDir,
		"- 2026-10-11 08:00 "+crashNote+"  "+attemptToken("gOTHER", "gOTHER.s1", 1))

	o.advanceGoalQueue(context.Background(), logDir, nil)

	if c := countAttemptTokens(t, logDir, "gOTHER", "gOTHER.s1", 1); c != 1 {
		t.Fatalf("foreign goal's note must be preserved untouched; got %d", c)
	}
	if c := countAttemptTokens(t, logDir, "g3", "g3.s1", 1); c != 0 {
		t.Fatalf("foreign token must not satisfy g3's a1; got %d", c)
	}
	if c := countAttemptTokens(t, logDir, "g3", "g3.s1", 2); c != 1 {
		t.Fatalf("g3 step must run once under a2; got %d", c)
	}
	g := loadGoal(logDir, conv)
	if len(g.Queue) != 0 || g.StepLease != nil {
		t.Fatalf("g3 step must be consumed on its own token; queue=%v lease=%+v", g.Queue, g.StepLease)
	}
}

// If the pre-side-effect lease write fails, the side effect MUST NOT run at all:
// zero notes, a failure receipt, and the step remains queued with no lease.
func TestEvidence_LeaseWriteFailureBlocksSideEffect(t *testing.T) {
	conv, logDir := "default", t.TempDir()
	gEx, gSize, gMtime := globalNotesStat(t)
	o := queueTestOptions(t, conv, logDir)
	defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)

	seedNoteHead(t, conv, logDir, "gL", false) // no lease yet
	o.FailGoalSave = func(stage string) bool { return stage == "lease" }

	recvs := o.advanceGoalQueue(context.Background(), logDir, nil)
	if !hasFailure(recvs) {
		t.Fatalf("a failed lease write must return a failure receipt; recvs=%+v", recvs)
	}
	if n := noteLines(t, logDir); n != 0 {
		t.Fatalf("side effect must be blocked when the lease is not durable; notes=%d", n)
	}
	g := loadGoal(logDir, conv)
	if len(g.Queue) != 1 {
		t.Fatalf("step must remain queued; queue=%v", g.Queue)
	}
	if g.StepLease != nil {
		t.Fatalf("a non-durable lease must not be left on disk; lease=%+v", g.StepLease)
	}
}

// If the post-success dequeue write fails, the side effect has happened but
// completion is not durable: the run must report UNCERTAIN (never done), keep the
// leased head on disk, and a later process must reconcile from the token WITHOUT
// re-applying the side effect (no duplicate note).
func TestEvidence_CommitWriteFailureUncertainThenReconcileOnce(t *testing.T) {
	conv, logDir := "default", t.TempDir()
	gEx, gSize, gMtime := globalNotesStat(t)
	o := queueTestOptions(t, conv, logDir)
	defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)

	seedNoteHead(t, conv, logDir, "gC", false) // fresh head, drain will lease it
	o.FailGoalSave = func(stage string) bool { return stage == "commit" }

	recvs := o.advanceGoalQueue(context.Background(), logDir, nil)
	if !hasFailure(recvs) {
		t.Fatalf("a failed commit write must report a non-success/UNCERTAIN receipt; recvs=%+v", recvs)
	}
	if n := noteLines(t, logDir); n != 1 {
		t.Fatalf("the side effect has already landed: exactly 1 note, got %d", n)
	}
	g := loadGoal(logDir, conv)
	if g.StepLease == nil || len(g.Queue) != 1 {
		t.Fatalf("disk must still show the leased head (dequeue not durable); lease=%+v queue=%v",
			g.StepLease, g.Queue)
	}

	// Next process: no fault injection; reconcile from the a1 token, pop once,
	// and never write the note a second time.
	o2 := queueTestOptions(t, conv, logDir)
	recvs2 := o2.advanceGoalQueue(context.Background(), logDir, nil)
	if hasFailure(recvs2) || hasConfirmAsk(recvs2) {
		t.Fatalf("token-backed recovery should cleanly dequeue; recvs=%+v", recvs2)
	}
	if n := noteLines(t, logDir); n != 1 {
		t.Fatalf("recovery must not duplicate the landed note; notes=%d", n)
	}
	if c := countAttemptTokens(t, logDir, "gC", "gC.s1", 1); c != 1 {
		t.Fatalf("the original a1 token must be the single proof; got %d", c)
	}
	g2 := loadGoal(logDir, conv)
	if g2.StepLease != nil || len(g2.Queue) != 0 {
		t.Fatalf("step must be consumed exactly once after recovery; lease=%+v queue=%v",
			g2.StepLease, g2.Queue)
	}
}
