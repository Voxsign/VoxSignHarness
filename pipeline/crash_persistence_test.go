package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"voicesign-harness/contract"
)

// P0 round 5 — goal persistence atomicity and corrupt-state fail-closed.
//
// The durable goal (which carries the write-ahead step lease) must NEVER be
// observable as a truncated/half-written file, and a corrupt file must fail
// closed: no create-overwrite, no side effect, no queue drain. These tests use
// a real second OS process to hard-exit in the write window (temp file synced,
// atomic rename not yet done), proving the previous complete goal survives.

func goalTempFiles(t *testing.T, logDir string) []string {
	t.Helper()
	dir := filepath.Join(logDir, "context_slots")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read context_slots: %v", err)
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, ".goal-") && strings.HasSuffix(n, ".tmp") {
			out = append(out, n)
		}
	}
	return out
}

func hasCorruptReceipt(rs []contract.Receipt) bool {
	for _, r := range rs {
		if r.Blocked == "goal_state_corrupt" {
			return true
		}
	}
	return false
}

// A successful atomic write leaves a complete, parseable goal and no temp file.
func TestGoalAtomicWrite_NoTempLeakAndComplete(t *testing.T) {
	logDir := t.TempDir()
	conv := "default"
	g1 := &Goal{Objective: "o1", Status: GoalActive, GoalID: "g1",
		Queue: []string{"记一下 a"}, QueueIDs: []string{"g1.s1"}}
	if err := saveGoalChecked(logDir, conv, g1); err != nil {
		t.Fatalf("save g1: %v", err)
	}
	g2 := &Goal{Objective: "o2", Status: GoalActive, GoalID: "g1", Round: 1,
		Queue: []string{"记一下 b"}, QueueIDs: []string{"g1.s1"}}
	if err := saveGoalChecked(logDir, conv, g2); err != nil {
		t.Fatalf("save g2: %v", err)
	}
	got, err := loadGoalChecked(logDir, conv)
	if err != nil {
		t.Fatalf("load after writes: %v", err)
	}
	if got.Objective != "o2" || got.Round != 1 || len(got.Queue) != 1 || got.Queue[0] != "记一下 b" {
		t.Fatalf("goal must be the latest complete version, got %+v", got)
	}
	if n := goalTempFiles(t, logDir); len(n) != 0 {
		t.Fatalf("no temp files may remain after a successful write, found %v", n)
	}
}

// A crash BETWEEN temp-file sync and atomic rename leaves the previous complete
// goal file byte-intact (never truncated) and only an orphaned temp file, which
// loaders ignore. The next successful write cleans the orphan.
func TestGoalWriteCrashBeforeRename_KeepsPreviousIntact(t *testing.T) {
	logDir := t.TempDir()
	conv := "default"

	code, out := runChild(t, "goalwrite-crash", logDir)
	if code != CrashExitCode {
		t.Fatalf("child must hard-exit %d in the write window; got %d\n%s", CrashExitCode, code, out)
	}

	// The formal goal file is still the complete v1 (lease included), not v2 and
	// not a truncated file.
	got, err := loadGoalChecked(logDir, conv)
	if err != nil {
		t.Fatalf("after a mid-write crash the previous goal must still parse: %v", err)
	}
	if got.Objective != "v1 目标" || got.GoalID != "gv" {
		t.Fatalf("previous goal content lost; got %+v", got)
	}
	if got.StepLease == nil || got.StepLease.StepID != "gv.s1" {
		t.Fatalf("the surviving step lease must be intact after the crash; got %+v", got.StepLease)
	}
	// The crashed write left an orphaned temp file (the v2 bytes).
	if n := goalTempFiles(t, logDir); len(n) != 1 {
		t.Fatalf("expected exactly one orphaned temp from the crashed write, got %v", n)
	}
	// Cross-session resume ignores the orphan temp and still sees v1.
	if rg := recentResumableGoal(logDir); rg == nil || rg.GoalID != "gv" {
		t.Fatalf("recentResumableGoal must ignore the orphan and return v1; got %+v", rg)
	}

	// A fresh, successful write (new process) yields a complete v3. It must NOT
	// sweep the directory: the orphaned temp from the crashed write is left in
	// place (a concurrent/other-conversation writer could own a live temp there)
	// and simply ignored by every loader.
	if err := saveGoalChecked(logDir, conv, &Goal{Objective: "v3 目标", Status: GoalActive, GoalID: "gv", Round: 2}); err != nil {
		t.Fatalf("recovery write: %v", err)
	}
	got3, err := loadGoalChecked(logDir, conv)
	if err != nil {
		t.Fatalf("load v3: %v", err)
	}
	if got3.Objective != "v3 目标" || got3.Round != 2 {
		t.Fatalf("goal must be complete v3, got %+v", got3)
	}
	if n := goalTempFiles(t, logDir); len(n) != 1 {
		t.Fatalf("a successful save must not delete another (crashed) writer's temp; want 1 surviving orphan, got %v", n)
	}
	if rg := recentResumableGoal(logDir); rg == nil || rg.Objective != "v3 目标" {
		t.Fatalf("resume must read the complete v3 despite the surviving orphan; got %+v", rg)
	}
}

// A save for one conversation must never delete another conversation's LIVE
// temp file that is sitting in the shared context_slots directory waiting to be
// renamed (the concurrency hazard a directory-wide sweep would introduce).
func TestGoalSave_DoesNotDeleteOtherConversationTemp(t *testing.T) {
	logDir := t.TempDir()
	// Alice has a complete goal; Bob is mid-write (his temp exists, not renamed).
	if err := saveGoalChecked(logDir, "alice", &Goal{Objective: "alice v1", Status: GoalActive, GoalID: "ga"}); err != nil {
		t.Fatal(err)
	}
	bobLive := filepath.Join(logDir, "context_slots", ".goal-bob-live.tmp")
	bobBytes := []byte(`{"Objective":"bob partial","GoalID":"gb"}`)
	if err := os.WriteFile(bobLive, bobBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	// Alice saves again; Bob's live temp must survive untouched.
	if err := saveGoalChecked(logDir, "alice",
		&Goal{Objective: "alice v2", Status: GoalActive, GoalID: "ga", Round: 1}); err != nil {
		t.Fatal(err)
	}
	got, err := loadGoalChecked(logDir, "alice")
	if err != nil || got.Objective != "alice v2" {
		t.Fatalf("alice must be complete v2; got %+v err=%v", got, err)
	}
	cur, err := os.ReadFile(bobLive)
	if err != nil {
		t.Fatalf("bob's live temp was wrongly deleted by alice's save: %v", err)
	}
	if !bytes.Equal(cur, bobBytes) {
		t.Fatalf("bob's live temp content was altered: %q", cur)
	}
}

// Concurrent saves across distinct conversations must each land a complete,
// correct goal with no cross-talk and no leftover temp from successful writes.
func TestGoalSave_ConcurrentConversations(t *testing.T) {
	logDir := t.TempDir()
	const nConv, nRounds = 8, 5
	var wg sync.WaitGroup
	errs := make(chan error, nConv*nRounds)
	for c := 0; c < nConv; c++ {
		wg.Add(1)
		go func(c int) {
			defer wg.Done()
			conv := "conv-" + strconv.Itoa(c)
			id := "g" + strconv.Itoa(c)
			for r := 0; r < nRounds; r++ {
				g := &Goal{Objective: "obj-" + conv, Status: GoalActive, GoalID: id,
					Round: r, Queue: []string{"记一下 " + conv + " 步 " + strconv.Itoa(r)},
					QueueIDs: []string{id + ".s0"}}
				if err := saveGoalChecked(logDir, conv, g); err != nil {
					errs <- fmt.Errorf("conv %s round %d: %w", conv, r, err)
				}
			}
		}(c)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatalf("concurrent save error: %v", e)
	}
	for c := 0; c < nConv; c++ {
		conv := "conv-" + strconv.Itoa(c)
		id := "g" + strconv.Itoa(c)
		g, err := loadGoalChecked(logDir, conv)
		if err != nil || g == nil {
			t.Fatalf("conv %s goal missing/corrupt: %+v err=%v", conv, g, err)
		}
		if g.GoalID != id || g.Objective != "obj-"+conv || g.Round != nRounds-1 {
			t.Fatalf("conv %s cross-talk or torn write: %+v", conv, g)
		}
	}
	// No fault was injected, so every successful write renamed its temp away; no
	// leftover temps may remain (and none was deleted from another writer).
	if n := goalTempFiles(t, logDir); len(n) != 0 {
		t.Fatalf("successful concurrent writes must leave no temp files; found %v", n)
	}
}

// A truncated orphan temp (from an even older crash) never shadows the real goal.
func TestOrphanTruncatedTempIgnored(t *testing.T) {
	logDir := t.TempDir()
	conv := "default"
	if err := saveGoalChecked(logDir, conv, &Goal{Objective: "real", Status: GoalActive, GoalID: "gr"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "context_slots", ".goal-abc.tmp"),
		[]byte(`{"GoalID":"broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadGoalChecked(logDir, conv)
	if err != nil || got == nil || got.Objective != "real" {
		t.Fatalf("truncated orphan temp must be ignored; got=%+v err=%v", got, err)
	}
	if rg := recentResumableGoal(logDir); rg == nil || rg.GoalID != "gr" {
		t.Fatalf("resume must ignore truncated orphan; got %+v", rg)
	}
}

// A corrupt goal file (e.g. a torn write from a non-atomic older version) fails
// closed at EVERY execution/recovery entry point: no create-overwrite, no side
// effect, no queue drain; the corrupt bytes are left for manual inspection.
func TestCorruptGoalFailsClosed(t *testing.T) {
	ctx := context.Background()
	logDir := t.TempDir()
	conv := "default"
	gEx, gSize, gMtime := globalNotesStat(t)
	o := queueTestOptions(t, conv, logDir)
	defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)

	base := &Goal{Objective: "重要序列", Status: GoalActive, GoalID: "gc", StepCounter: 2,
		Queue:      []string{"从github下载代码编译测试", "记一下 收尾"},
		QueueIDs:   []string{"gc.s1", "gc.s2"},
		HeadAttempt: 1, StepSeq: 1,
		StepLease: &StepLease{GoalID: "gc", StepID: "gc.s1", Attempt: 1, Seq: 1,
			Text: "从github下载代码编译测试", Intent: contract.IntentBuildTest,
			StartedAt: "2026-10-11T00:00:00Z"}}
	if err := saveGoalChecked(logDir, conv, base); err != nil {
		t.Fatal(err)
	}
	p := goalPath(logDir, conv)
	bad := []byte(`{"GoalID":"gc","Queue":["x"]`)
	if err := os.WriteFile(p, bad, 0o644); err != nil {
		t.Fatal(err)
	}

	// 1) checked loader reports the corruption.
	if g, err := loadGoalChecked(logDir, conv); err == nil || g != nil {
		t.Fatalf("loadGoalChecked must error on corrupt JSON; got g=%+v err=%v", g, err)
	}

	// 2) maybeCreateGoal fails and does NOT overwrite the corrupt file.
	it := contract.Intent{Intent: contract.IntentNote, CorrectedText: "记一下 新目标"}
	if ng, err := o.maybeCreateGoal(it); err == nil || ng != nil {
		t.Fatalf("maybeCreateGoal must fail closed on corrupt state; ng=%+v err=%v", ng, err)
	}
	if cur, _ := os.ReadFile(p); !bytes.Equal(cur, bad) {
		t.Fatal("maybeCreateGoal must not overwrite the corrupt goal file")
	}

	// 3) advanceGoalQueue returns the corrupt receipt, drains nothing, runs nothing.
	recvs := o.advanceGoalQueue(ctx, logDir, nil)
	if !hasCorruptReceipt(recvs) {
		t.Fatalf("advanceGoalQueue must surface the corrupt receipt; %+v", recvs)
	}
	if noteLines(t, logDir) != 0 {
		t.Fatal("no side effect may run against corrupt state")
	}
	if cur, _ := os.ReadFile(p); !bytes.Equal(cur, bad) {
		t.Fatal("advanceGoalQueue must not overwrite the corrupt goal file")
	}

	// 4) the crash voice bridge intercepts the turn (handled=true) instead of
	// falling through to normal execution that would create a fresh goal.
	rr, handled := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
		contract.Intent{Intent: contract.IntentUnknown, CorrectedText: "继续"}, "继续")
	if !handled || !hasCorruptReceipt(rr) {
		t.Fatalf("resolveCrashedLeaseTurn must intercept corrupt state; handled=%v recvs=%+v", handled, rr)
	}

	// 5) execSequence refuses to start a fresh chain over the corrupt file.
	seqIt := contract.Intent{Intent: contract.IntentSequence,
		Params: map[string]string{"steps": "记一下 新链步\n记一下 第二步"}}
	sr := o.execSequence(ctx, seqIt, logDir)
	if !hasCorruptReceipt(sr) {
		t.Fatalf("execSequence must fail closed on corrupt state; %+v", sr)
	}
	if noteLines(t, logDir) != 0 {
		t.Fatal("execSequence must not run any step against corrupt state")
	}
	if cur, _ := os.ReadFile(p); !bytes.Equal(cur, bad) {
		t.Fatal("execSequence must not overwrite the corrupt goal file")
	}
}
