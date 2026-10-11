package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"voicesign-harness/config"
	"voicesign-harness/tools"
)

// queueTestOptions builds an Options that can run the real, offline actions the
// queue drains use: NOTE appends via the built-in "file" tool; EMAIL fetch fails
// fast without AIOPS_KEY (before any HTTP); a fresh INSTALL always opens a
// confirmation gate. No network, host mutation or real mail is involved.
//
// Isolation is asserted, not assumed: every side-effect path must resolve inside
// the per-test t.TempDir() (Options.logDir() and Executor.BaseDir), never the
// developer's real global home notebook (~/.voicesign/harness/notes.md).
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
	o := &Options{
		Cfg:    &cfg,
		ConvID: convID,
		Tools:  reg,
		Exec:   &tools.Executor{BaseDir: logDir},
	}
	// Hard isolation assertions: the pipeline resolves every side-effect path from
	// Options.logDir(); it must be exactly this test's temp dir, and the executor's
	// relative-path base must match. Otherwise NOTE would silently write the real
	// global notebook (the round-2 red-test leak this now prevents).
	if got := o.logDir(); got != logDir {
		t.Fatalf("test isolation broken: Options.logDir()=%q want temp %q", got, logDir)
	}
	if o.Exec == nil || o.Exec.BaseDir != logDir {
		t.Fatalf("test isolation broken: Executor.BaseDir must be temp %q", logDir)
	}
	if home, err := os.UserHomeDir(); err == nil {
		global := filepath.Join(home, ".voicesign", "harness")
		if strings.HasPrefix(filepath.Clean(logDir)+string(os.PathSeparator), filepath.Clean(global)+string(os.PathSeparator)) ||
			filepath.Clean(logDir) == filepath.Clean(global) {
			t.Fatalf("test isolation broken: temp logDir %q is inside the real global home %q", logDir, global)
		}
	}
	return o
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

// globalNotesStat snapshots ONLY the metadata (never the content) of the real
// home notebook, so a test can prove afterward that it did not touch it.
func globalNotesStat(t *testing.T) (bool, int64, time.Time) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		return false, 0, time.Time{}
	}
	fi, err := os.Stat(filepath.Join(home, ".voicesign", "harness", "notes.md"))
	if err != nil {
		return false, 0, time.Time{}
	}
	return true, fi.Size(), fi.ModTime()
}

// assertGlobalNotesUnchanged fails the test if the real home notebook changed
// (size or mtime) while the test ran — a direct guard against any NOTE leaking
// out of t.TempDir(). It never reads, deletes or modifies the user's notebook.
func assertGlobalNotesUnchanged(t *testing.T, before bool, beforeSize int64, beforeMtime time.Time) {
	t.Helper()
	ex, sz, mt := globalNotesStat(t)
	if ex != before || sz != beforeSize || !mt.Equal(beforeMtime) {
		t.Fatalf("test leaked a NOTE into the real global notebook ~/.voicesign/harness/notes.md "+
			"(before exists=%v size=%d mtime=%v; after exists=%v size=%d mtime=%v)",
			before, beforeSize, beforeMtime.Format(time.RFC3339), ex, sz, mt.Format(time.RFC3339))
	}
}

// TestQueueFailureParksStepAndRestartDoesNotReplay: the first step succeeds and is
// durably consumed; the second (email fetch) fails and stays parked at the queue
// head. A simulated restart/re-drain must retry the parked failure but must never
// replay the already-succeeded side effect (exactly-once for the success step).
func TestQueueFailureParksStepAndRestartDoesNotReplay(t *testing.T) {
	convID := "default"
	logDir := t.TempDir()
	gEx, gSize, gMtime := globalNotesStat(t)
	o := queueTestOptions(t, convID, logDir)
	defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)

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
		t.Fatalf("the successful NOTE side effect must run exactly once IN TEMP, got %d note lines", n)
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
	gEx, gSize, gMtime := globalNotesStat(t)
	o := queueTestOptions(t, convID, logDir)
	defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)

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
		gEx, gSize, gMtime := globalNotesStat(t)
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
		assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)
	}
}
