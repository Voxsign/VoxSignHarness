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

// Crash-window recovery tests (P0 rounds 3-4). These use a REAL second OS process
// (os/exec of the compiled test binary) that hard-exits via the
// CrashAfterQueuedStep fault seam (exit code CrashExitCode=42) at the precise
// window "side effect done, durable dequeue not yet written". A third process
// then starts from ONLY the on-disk goal state (no in-memory state is shared),
// which is genuine process-crash recovery — not an in-process re-drain.
//
// Round 4 proves exactly-once via the STABLE (GoalID, StepID, Attempt) token
// written on the note line, never via note-text substring matching: the two
// queued steps are deliberately IDENTICAL text, so a text-based check could not
// tell them apart, while stable ids can.
//
// Coverage:
//   - Two identical local NOTE steps: after crashing on step 1, recovery pops it
//     from its (g,s1,a1) token and runs step 2 under (g,s2,a1); each lands once.
//   - Externally uncertain EMAIL step: held with no decision (no replay),
//     redo_confirmed re-runs once, human accept dequeues without re-sending.

const (
	childEnv    = "VHS_CRASH_CHILD"
	childLogDir = "VHS_CRASH_LOGDIR"
	childConv   = "VHS_CRASH_CONV"
	crashNote   = "记一下 崩溃同文步"
	emailStep   = "回复最新邮件"
	tailNote    = "记一下 外部收尾"
)

// countAttemptTokens counts note lines carrying the exact (goal,step,attempt)
// correlation marker — the stable-identity proof used by crash recovery.
func countAttemptTokens(t *testing.T, logDir, goalID, stepID string, attempt int) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(logDir, "notes.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read notes.md: %v", err)
	}
	n := 0
	for _, line := range strings.Split(string(b), "\n") {
		g, s, a, ok := parseAttemptToken(line)
		if ok && g == goalID && s == stepID && a == attempt {
			n++
		}
	}
	return n
}

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
		saveGoal(logDir, conv, &Goal{
			Objective: "崩溃序列", Status: GoalActive, GoalID: "gn", StepCounter: 2,
			Queue:    []string{crashNote, crashNote},
			QueueIDs: []string{"gn.s1", "gn.s2"},
		})
		o.CrashAfterQueuedStep = func(text string, _ contract.Intent, _ []contract.Receipt) bool {
			return strings.Contains(text, "崩溃同文步") // crash on the FIRST step
		}
		o.advanceGoalQueue(ctx, logDir, nil)
		t.Fatal("phase1 should have hard-exited, not returned")

	case "note-phase2":
		// Brand-new Options: a fresh process recovering from disk only.
		o := queueTestOptions(t, conv, logDir)
		o.advanceGoalQueue(ctx, logDir, nil)

	case "ext-setup":
		// On-disk state of "EMAIL side effect in flight, process died before
		// dequeue": external lease present with stable ids, head still queued.
		saveGoal(logDir, conv, &Goal{
			Objective: "外部崩溃序列", Status: GoalActive, GoalID: "ge", StepCounter: 2,
			Queue:     []string{emailStep, tailNote},
			QueueIDs:  []string{"ge.s1", "ge.s2"},
			HeadAttempt: 1, StepSeq: 1,
			StepLease: &StepLease{GoalID: "ge", StepID: "ge.s1", Attempt: 1, Seq: 1,
				Text: emailStep, Intent: contract.IntentEmail, StartedAt: "2026-10-11T00:00:00Z"},
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
		if sawEmailAttempt(recvs) {
			t.Fatalf("must NOT re-run the external step without a decision; recvs=%+v", recvs)
		}

	case "ext-redorequested":
		// A re-send was requested but the explicit risk gate is not passed: held,
		// no replay.
		o := queueTestOptions(t, conv, logDir)
		g := loadGoal(logDir, conv)
		g.LeaseDecision = "redo_requested"
		saveGoal(logDir, conv, g)
		recvs := o.advanceGoalQueue(ctx, logDir, nil)
		if !hasConfirmAsk(recvs) {
			t.Fatalf("redo_requested must show the risk gate; recvs=%+v", recvs)
		}
		if sawEmailAttempt(recvs) {
			t.Fatalf("redo_requested must NOT re-send before explicit confirmation; recvs=%+v", recvs)
		}
		g2 := loadGoal(logDir, conv)
		if g2.StepLease == nil || g2.LeaseDecision != "redo_requested" {
			t.Fatalf("redo_requested lease/decision must be retained; g=%+v", g2)
		}

	case "ext-redo":
		// Both voice gates passed (redo_confirmed): reconcile re-runs the head once.
		o := queueTestOptions(t, conv, logDir)
		g := loadGoal(logDir, conv)
		g.LeaseDecision = "redo_confirmed"
		saveGoal(logDir, conv, g)
		recvs := o.advanceGoalQueue(ctx, logDir, nil)
		if !sawEmailAttempt(recvs) {
			t.Fatalf("redo_confirmed must re-attempt the email step once; recvs=%+v", recvs)
		}
		if n := countEmailAttempts(recvs); n != 1 {
			t.Fatalf("redo_confirmed must attempt email exactly once, got %d; recvs=%+v", n, recvs)
		}

	case "ext-accept":
		o := queueTestOptions(t, conv, logDir)
		g := loadGoal(logDir, conv)
		g.LeaseDecision = "accept"
		g.LeaseManualAccept = "2026-10-11T00:05:00Z 我确认已手动发送"
		saveGoal(logDir, conv, g)
		recvs := o.advanceGoalQueue(ctx, logDir, nil)
		if sawEmailAttempt(recvs) {
			t.Fatalf("accept must NOT re-run the email step; recvs=%+v", recvs)
		}
		g2 := loadGoal(logDir, conv)
		if g2.StepLease != nil {
			t.Fatal("accept must clear the lease")
		}
		foundAtt := false
		for _, s := range g2.Steps {
			if strings.Contains(s, "human-verified") {
				foundAtt = true
			}
		}
		if !foundAtt {
			t.Fatalf("accept must record a human-verified attestation in Steps; steps=%v", g2.Steps)
		}
		if noteLines(t, logDir) != 1 {
			t.Fatalf("accept should dequeue the email step and run the tail note once; notes=%d", noteLines(t, logDir))
		}

	default:
		t.Fatalf("unknown child mode %q", mode)
	}
}

// TestQueuedStepCrash_IdenticalNotesExactlyOnce: two IDENTICAL local NOTE steps;
// the process hard-crashes after step 1's side effect and before its dequeue.
// Recovery must attribute the on-disk line to (gn,s1,a1), pop it without
// re-writing, and then run step 2 under (gn,s2,a1). Each token appears once.
func TestQueuedStepCrash_IdenticalNotesExactlyOnce(t *testing.T) {
	logDir := t.TempDir()

	code, out := runChild(t, "note-phase1", logDir)
	if code != CrashExitCode {
		t.Fatalf("phase1 should hard-crash with %d, got %d\n%s", CrashExitCode, code, out)
	}
	if n := noteLines(t, logDir); n != 1 {
		t.Fatalf("after phase1 crash exactly the first NOTE should be on disk, got %d lines\n%s", n, out)
	}
	g := loadGoal(logDir, "default")
	if g == nil || g.StepLease == nil || g.StepLease.StepID != "gn.s1" || g.StepLease.Attempt != 1 {
		t.Fatalf("crash lease must carry stable (gn,s1,a1); g=%+v lease=%+v", g, leaseOf(g))
	}
	if len(g.Queue) != 2 {
		t.Fatalf("crash point: dequeue must not be persisted, queue=%v", g.Queue)
	}

	code, out = runChild(t, "note-phase2", logDir)
	if code != 0 {
		t.Fatalf("phase2 recovery should exit 0, got %d\n%s", code, out)
	}
	// Stable-id exactly-once assertions (text is identical, so only ids prove it).
	if c := countAttemptTokens(t, logDir, "gn", "gn.s1", 1); c != 1 {
		t.Fatalf("step1 attempt1 token must appear exactly once, got %d", c)
	}
	if c := countAttemptTokens(t, logDir, "gn", "gn.s2", 1); c != 1 {
		t.Fatalf("step2 attempt1 token must appear exactly once, got %d", c)
	}
	if c := countAttemptTokens(t, logDir, "gn", "gn.s1", 2); c != 0 {
		t.Fatalf("step1 must not be re-run under a new attempt, found %d s1/a2 tokens", c)
	}
	if n := noteLines(t, logDir); n != 2 {
		b, _ := os.ReadFile(filepath.Join(logDir, "notes.md"))
		t.Fatalf("want 2 note lines (identical steps once each), got %d\nnotes:\n%s", n, b)
	}
	g2 := loadGoal(logDir, "default")
	if g2 != nil && g2.StepLease != nil {
		t.Fatalf("lease must be cleared after recovery; lease=%+v", g2.StepLease)
	}
	if len(g2.Queue) != 0 {
		t.Fatalf("both identical steps must be consumed; queue=%v", g2.Queue)
	}
}

func leaseOf(g *Goal) *StepLease {
	if g == nil {
		return nil
	}
	return g.StepLease
}

// TestQueuedStepCrash_ExternalNotReplayed: an EMAIL step crashed mid-flight;
// with no decision it is held (never re-sent), a redo_requested still waits at
// the risk gate, redo_confirmed re-runs once, and human accept dequeues safely.
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

	// redo requested but risk gate not yet confirmed -> still no send.
	dirReq := t.TempDir()
	if c, out := runChild(t, "ext-setup", dirReq); c != 0 {
		t.Fatalf("setup(redo_requested) exit %d\n%s", c, out)
	}
	if c, out := runChild(t, "ext-redorequested", dirReq); c != 0 {
		t.Fatalf("redorequested recovery exit %d\n%s", c, out)
	}
	if n := noteLines(t, dirReq); n != 0 {
		t.Fatalf("redo_requested must not advance to the tail note; notes=%d", n)
	}

	// redo_confirmed -> re-attempt once (offline it fails fast for no AIOPS_KEY).
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

	// Human accept -> skip re-send, dequeue with attestation, run tail once.
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
