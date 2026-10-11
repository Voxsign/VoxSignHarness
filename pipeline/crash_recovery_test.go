package pipeline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/contract"
)

// Crash-window recovery tests (P0 round 3). These use a REAL second OS process
// (os/exec of the compiled test binary) that hard-exits via the
// CrashAfterQueuedStep fault seam (exit code CrashExitCode=42) at the precise
// window "side effect done, durable dequeue not yet written". A third/fourth
// process then starts from ONLY the on-disk goal state (no in-memory state is
// shared), which is genuine process-crash recovery — not an in-process re-drain.
//
// Coverage:
//   - Local, reconcilable step (NOTE append): after crash the note exists but the
//     step was never dequeued; recovery must NOT append it twice (exactly-once).
//   - Externally uncertain step (EMAIL): after crash the outcome is unknowable
//     offline; with no operator decision recovery must NOT re-run it (no duplicate
//     send) and must hold at need_confirm; explicit "redo" re-runs it once,
//     explicit "accept" treats it as done and continues without re-running it.

const (
	childEnv    = "VHS_CRASH_CHILD"
	childLogDir = "VHS_CRASH_LOGDIR"
	childConv   = "VHS_CRASH_CONV"
	noteA       = "记一下 崩溃步A"
	noteB       = "记一下 崩溃步B"
	emailStep   = "回复最新邮件"
	tailNote    = "记一下 外部收尾"
)

// runChild re-executes the test binary so only TestCrashChild runs in the child.
func runChild(t *testing.T, mode, logDir string) (int, string) {
	t.Helper()
	bin, binErr := os.Executable()
	if binErr != nil {
		t.Fatalf("locate test binary: %v", binErr)
	}
	cmd := exec.Command(bin, "-test.run=^TestCrashChild$", "-test.v")
	env := os.Environ()
	env = append(env, childEnv+"="+mode, childLogDir+"="+logDir, childConv+"=default")
	cmd.Env = env
	out, runErr := cmd.CombinedOutput()
	code := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run child %s: %v\n%s", mode, runErr, out)
		}
	}
	return code, string(out)
}

// TestCrashChild is the child-process entry; it skips under a normal test run.
func TestCrashChild(t *testing.T) {
	mode := os.Getenv(childEnv)
	if mode == "" {
		t.Skip("child process entry only")
	}
	logDir := os.Getenv(childLogDir)
	conv := os.Getenv(childConv)
	if conv == "" {
		conv = "default"
	}
	ctx := context.Background()

	switch mode {
	case "note-phase1":
		o := queueTestOptions(t, conv, logDir)
		saveGoal(logDir, conv, &Goal{Objective: "崩溃序列", Status: GoalActive, Queue: []string{noteA, noteB}})
		o.CrashAfterQueuedStep = func(text string, _ contract.Intent, _ []contract.Receipt) bool {
			return strings.Contains(text, "崩溃步A")
		}
		o.advanceGoalQueue(ctx, logDir, nil)
		t.Fatal("phase1 should have hard-exited, not returned")

	case "note-phase2":
		// Brand-new Options: simulate a fresh process recovering from disk only.
		o := queueTestOptions(t, conv, logDir)
		o.advanceGoalQueue(ctx, logDir, nil)

	case "ext-setup":
		// Reproduce the on-disk state of "EMAIL side effect happened, process died
		// before dequeue": lease present, head step still queued. We cannot send a
		// real mail offline; the lease state is exactly the crash point and the
		// no-replay logic does not depend on whether the send actually landed.
		saveGoal(logDir, conv, &Goal{
			Objective:  "外部崩溃序列",
			Status:     GoalActive,
			Queue:      []string{emailStep, tailNote},
			StepSeq:    1,
			StepLease:  &StepLease{Seq: 1, Text: emailStep, Intent: contract.IntentEmail, StartedAt: "2026-10-11T00:00:00Z"},
		})

	case "ext-nodecision":
		o := queueTestOptions(t, conv, logDir)
		recvs := o.advanceGoalQueue(ctx, logDir, nil)
		g := loadGoal(logDir, conv)
		if g == nil || g.StepLease == nil {
			t.Fatal("external lease must be retained when no operator decision is made")
		}
		if len(g.Queue) != 2 || !strings.Contains(g.Queue[0], "邮件") {
			t.Fatalf("email step must stay queued (no replay); queue=%v", g.Queue)
		}
		if !hasConfirmAsk(recvs) {
			t.Fatalf("external uncertain step must hold at need_confirm; recvs=%+v", recvs)
		}
		for _, r := range recvs {
			if r.Tool == "email" && !r.OK {
				t.Fatalf("must NOT re-run the external step without a decision, but got email failure: %+v", r)
			}
		}

	case "ext-redo":
		o := queueTestOptions(t, conv, logDir)
		g := loadGoal(logDir, conv)
		g.LeaseDecision = "redo"
		saveGoal(logDir, conv, g)
		recvs := o.advanceGoalQueue(ctx, logDir, nil)
		g2 := loadGoal(logDir, conv)
		if g2.StepLease != nil {
			t.Fatal("redo must clear the lease")
		}
		// EMAIL has no AIOPS_KEY offline, so an authorized re-run is attempted and
		// fails fast; the proof is that an email tool receipt now exists (it ran),
		// versus nodecision where no email receipt is allowed.
		sawEmail := false
		for _, r := range recvs {
			if r.Tool == "email" {
				sawEmail = true
			}
		}
		if !sawEmail {
			t.Fatalf("explicit redo must re-attempt the email step; recvs=%+v", recvs)
		}

	case "ext-accept":
		o := queueTestOptions(t, conv, logDir)
		g := loadGoal(logDir, conv)
		g.LeaseDecision = "accept"
		saveGoal(logDir, conv, g)
		recvs := o.advanceGoalQueue(ctx, logDir, nil)
		for _, r := range recvs {
			if r.Tool == "email" {
				t.Fatalf("accept must NOT re-run the email step; got %+v", r)
			}
		}
		g2 := loadGoal(logDir, conv)
		if g2.StepLease != nil {
			t.Fatal("accept must clear the lease")
		}
		if noteLines(t, logDir) != 1 {
			t.Fatalf("accept should dequeue the email step and run the tail note once; notes=%d", noteLines(t, logDir))
		}

	default:
		t.Fatalf("unknown child mode %q", mode)
	}
}

// TestQueuedStepCrash_NoteExactlyOnce: a NOTE side effect that completed just
// before a hard crash is reconciled from disk and never appended a second time.
func TestQueuedStepCrash_NoteExactlyOnce(t *testing.T) {
	logDir := t.TempDir()

	code, out := runChild(t, "note-phase1", logDir)
	if code != CrashExitCode {
		t.Fatalf("phase1 should hard-crash with %d, got %d\n%s", CrashExitCode, code, out)
	}
	if n := noteLines(t, logDir); n != 1 {
		t.Fatalf("after phase1 crash exactly the first NOTE should be on disk, got %d lines\n%s", n, out)
	}
	g := loadGoal(logDir, "default")
	if g == nil || len(g.Queue) != 2 {
		t.Fatalf("crash point: dequeue must not be persisted, queue=%v", g)
	}

	code, out = runChild(t, "note-phase2", logDir)
	if code != 0 {
		t.Fatalf("phase2 recovery should exit 0, got %d\n%s", code, out)
	}
	// Exactly-once: A once (recovered, not replayed) and B once = 2 lines.
	if n := noteLines(t, logDir); n != 2 {
		b, _ := os.ReadFile(filepath.Join(logDir, "notes.md"))
		t.Fatalf("crashed NOTE must not be replayed: want 2 note lines (A,B once each), got %d\nnotes:\n%s", n, b)
	}
	g2 := loadGoal(logDir, "default")
	if g2 != nil && g2.StepLease != nil {
		t.Fatalf("lease must be cleared after recovery; lease=%+v", g2.StepLease)
	}
}

// TestQueuedStepCrash_ExternalNotReplayed: an EMAIL step crashed mid-flight;
// without an explicit decision it is held (need_confirm, never re-sent), and the
// explicit redo/accept decisions behave exactly once each.
func TestQueuedStepCrash_ExternalNotReplayed(t *testing.T) {
	// No decision -> hold, no replay.
	dirHold := t.TempDir()
	if c, out := runChild(t, "ext-setup", dirHold); c != 0 {
		t.Fatalf("setup exit %d\n%s", c, out)
	}
	if c, out := runChild(t, "ext-nodecision", dirHold); c != 0 {
		t.Fatalf("nodecision recovery should hold cleanly, exit %d\n%s", c, out)
	}
	if n := noteLines(t, dirHold); n != 0 {
		t.Fatalf("tail note must not run while the email step is held; notes=%d", n)
	}

	// Explicit redo -> re-attempt once (fresh crash state).
	dirRedo := t.TempDir()
	if c, out := runChild(t, "ext-setup", dirRedo); c != 0 {
		t.Fatalf("setup(redo) exit %d\n%s", c, out)
	}
	if c, out := runChild(t, "ext-redo", dirRedo); c != 0 {
		t.Fatalf("redo recovery exit %d\n%s", c, out)
	}
	if n := noteLines(t, dirRedo); n != 0 {
		t.Fatalf("redo of a failed email must not advance to the tail note; notes=%d", n)
	}

	// Explicit accept -> skip re-send, dequeue, run tail.
	dirAccept := t.TempDir()
	if c, out := runChild(t, "ext-setup", dirAccept); c != 0 {
		t.Fatalf("setup(accept) exit %d\n%s", c, out)
	}
	if c, out := runChild(t, "ext-accept", dirAccept); c != 0 {
		t.Fatalf("accept recovery exit %d\n%s", c, out)
	}
	if n := noteLines(t, dirAccept); n != 1 {
		t.Fatalf("accept should run the tail note exactly once; notes=%d", n)
	}
}
