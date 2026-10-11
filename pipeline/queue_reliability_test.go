package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/config"
	"voicesign-harness/tools"
)

// queueTestOptions builds an Options that can run the real, offline actions the
// queue drains use: NOTE appends via the built-in "file" tool; EMAIL fetch fails
// fast without AIOPS_KEY (before any HTTP); a fresh INSTALL always opens a
// confirmation gate. No network, host mutation or real mail is involved.
func queueTestOptions(t *testing.T, convID, logDir string) *Options {
	t.Helper()
	cfg := config.Default()
	cfg.Global.LogDir = logDir
	cfg.Memory.Dir = filepath.Join(logDir, "mem")
	cfg.Spaces.Dir = filepath.Join(logDir, "spaces")
	cfg.Contracts.Dir = filepath.Join(logDir, "contracts")
	cfg.Cache.Dir = filepath.Join(logDir, "cache")
	for _, d := range []string{cfg.Memory.Dir, cfg.Spaces.Dir, cfg.Contracts.Dir, cfg.Cache.Dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	reg, err := tools.LoadContracts(cfg.Contracts.Dir)
	if err != nil {
		t.Fatalf("load contracts: %v", err)
	}
	return &Options{
		Cfg:    &cfg,
		ConvID: convID,
		Tools:  reg,
		Exec:   &tools.Executor{BaseDir: logDir},
	}
}

func noteLines(t *testing.T, logDir string) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(logDir, "notes.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read notes.md: %v", err)
	}
	n := 0
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "- ") {
			n++
		}
	}
	return n
}

// TestQueueFailureParksStepAndRestartDoesNotReplay: the first step succeeds and is
// durably consumed; the second (email fetch) fails and stays parked at the queue
// head. A simulated restart/re-drain must retry the parked failure but must never
// replay the already-succeeded side effect (exactly-once for the success step).
func TestQueueFailureParksStepAndRestartDoesNotReplay(t *testing.T) {
	convID := "default"
	logDir := t.TempDir()
	o := queueTestOptions(t, convID, logDir)

	successStep := "记一下 队列甲"
	failStep := "读取收件箱最新邮件"
	saveGoal(logDir, convID, &Goal{Objective: "队列可靠性", Status: GoalActive, Queue: []string{successStep, failStep}})

	recvs := o.advanceGoalQueue(context.Background(), logDir, nil)

	g := loadGoal(logDir, convID)
	if len(g.Queue) != 1 || !strings.Contains(g.Queue[0], "收件箱") {
		t.Fatalf("failed step must stay parked at queue head; queue=%v", g.Queue)
	}
	if !hasFailure(recvs) {
		t.Fatalf("expected a failure receipt from the email step; got %+v", recvs)
	}
	if n := noteLines(t, logDir); n != 1 {
		t.Fatalf("the successful NOTE side effect must run exactly once, got %d note lines", n)
	}

	// Simulate a restart / next-turn resume: re-drain from the persisted goal.
	recvs2 := o.advanceGoalQueue(context.Background(), logDir, nil)
	g2 := loadGoal(logDir, convID)
	if len(g2.Queue) != 1 || !strings.Contains(g2.Queue[0], "收件箱") {
		t.Fatalf("parked failure must remain at head after retry; queue=%v", g2.Queue)
	}
	if !hasFailure(recvs2) {
		t.Fatalf("resume should retry the parked step and fail again; got %+v", recvs2)
	}
	if n := noteLines(t, logDir); n != 1 {
		t.Fatalf("succeeded step replayed on resume: note lines=%d (want 1)", n)
	}
}

// TestQueueStopsAtConfirmationGate: a confirmation gate consumes nothing and the
// steps behind it never execute in the same turn.
func TestQueueStopsAtConfirmationGate(t *testing.T) {
	convID := "default"
	logDir := t.TempDir()
	o := queueTestOptions(t, convID, logDir)

	gateStep := "帮我安装 codex 到后台"
	behind := "记一下 门后步骤"
	saveGoal(logDir, convID, &Goal{Objective: "安装门", Status: GoalActive, Queue: []string{gateStep, behind}})

	recvs := o.advanceGoalQueue(context.Background(), logDir, nil)

	g := loadGoal(logDir, convID)
	if len(g.Queue) != 2 || !strings.Contains(g.Queue[0], "安装") {
		t.Fatalf("gate step must not be consumed and must stay at head; queue=%v", g.Queue)
	}
	if !hasConfirmAsk(recvs) {
		t.Fatalf("expected a confirmation gate; got %+v", recvs)
	}
	if n := noteLines(t, logDir); n != 0 {
		t.Fatalf("steps behind a confirmation gate must not execute; note lines=%d", n)
	}
}

// TestQueueNeverDrainsForNonActiveGoal: a canceled, paused, blocked or completed
// goal must not execute any queued side effect and must leave the queue intact.
// Without this guard a cancel that leaves residual queue entries would still fire.
func TestQueueNeverDrainsForNonActiveGoal(t *testing.T) {
	for _, status := range []string{GoalCanceled, GoalPaused, GoalBlocked, GoalCompleted} {
		logDir := t.TempDir()
		convID := "default"
		o := queueTestOptions(t, convID, logDir)
		saveGoal(logDir, convID, &Goal{Objective: "不应继续", Status: status, Queue: []string{"记一下 不该执行"}})

		recvs := o.advanceGoalQueue(context.Background(), logDir, nil)
		if len(recvs) != 0 {
			t.Fatalf("%s goal must not drain its queue; recvs=%+v", status, recvs)
		}
		if n := noteLines(t, logDir); n != 0 {
			t.Fatalf("%s goal must not execute queued side effects; note lines=%d", status, n)
		}
		g := loadGoal(logDir, convID)
		if len(g.Queue) != 1 {
			t.Fatalf("%s goal queue must be preserved; queue=%v", status, g.Queue)
		}
	}
}
