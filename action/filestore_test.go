package action

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// A confirmed action must survive a process restart via JSONL replay.
func TestFileStoreDurableAcrossReopen(t *testing.T) {
	now, _ := clk()
	path := filepath.Join(t.TempDir(), "actions.jsonl")
	fs, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ex := newExec(fs, "w1", now)
	if r := ex.Do(context.Background(), selFor("t1", "k1"),
		countingDo(nil, OutcomeConfirmedOK, "<ext-durable>", nil, 0)); !r.Terminal() {
		t.Fatalf("seed success: %+v", r)
	}

	// Reopen as a new process would.
	fs2, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	cur, err := fs2.Get("t1", "k1")
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if cur.Status != StatusSucceeded || cur.ExternalRef != "<ext-durable>" || cur.Attempts != 1 {
		t.Fatalf("durable record mismatch after replay: %+v", cur)
	}
}

// Crash after the external call but before the status write leaves a dead
// PENDING; a restarted worker must reconcile, never blindly replay.
func TestFileStoreCrashAfterSendDoesNotReplay(t *testing.T) {
	clkNow, adv := clk()
	path := filepath.Join(t.TempDir(), "actions.jsonl")
	fs, _ := OpenFileStore(path)
	sel := selFor("t1", "email-1")
	if _, dec, err := fs.Acquire(sel, "old-worker", 5*time.Second, clkNow()); err != nil || dec != DecisionAcquired {
		t.Fatalf("seed acquire dec=%v err=%v", dec, err)
	}
	// "Crash": no Mark is written. Time passes beyond the lease; new process opens.
	adv(30 * time.Second)
	fs2, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	var replayCalls int32
	exNew := newExec(fs2, "new-worker", clkNow)
	r := exNew.Do(context.Background(), sel,
		countingDo(&replayCalls, OutcomeConfirmedOK, "<dup>", nil, 0))
	if !r.NeedsReconcile || replayCalls != 0 {
		t.Fatalf("dead PENDING after crash must force reconcile and not replay, got %+v calls=%d", r, replayCalls)
	}
	// Strong external evidence the message actually went out -> confirm, no resend.
	rc, err := exNew.Reconcile(context.Background(), "t1", "email-1",
		func(ctx context.Context, rec Record) (ProbeResult, string, error) {
			return ProbeConfirmedDone, "<server-msg-id>", nil
		})
	if err != nil || !rc.Terminal() || replayCalls != 0 {
		t.Fatalf("confirmed-done after crash must resolve without resend, rc=%+v err=%v calls=%d", rc, err, replayCalls)
	}
}

// A durable write failure must not leave a false success in the ledger.
func TestFileStorePersistFailureNoFalseSuccess(t *testing.T) {
	now, _ := clk()
	path := filepath.Join(t.TempDir(), "actions.jsonl")
	fs, _ := OpenFileStore(path)
	sel := selFor("t1", "k1")
	if _, dec, _ := fs.Acquire(sel, "w1", time.Minute, now()); dec != DecisionAcquired {
		t.Fatalf("seed acquire")
	}
	// Make the log read-only so the Mark append/fsync fails.
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer func() { _ = os.Chmod(path, 0o644) }()
	fs2, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("reopen read-only: %v", err)
	}
	_, merr := fs2.Mark("t1", "k1", 1, func(r *Record) { r.Status = StatusSucceeded }, now())
	if merr == nil {
		t.Fatalf("Mark against an unwritable log must fail")
	}
	cur, _ := fs2.Get("t1", "k1")
	if cur.Status == StatusSucceeded {
		t.Fatalf("a failed durable Mark must not be recorded as SUCCEEDED, got %+v", cur)
	}
}

// FileStore must also guarantee a single external call under concurrency.
func TestFileStoreConcurrentExecutesOnce(t *testing.T) {
	now, _ := clk()
	path := filepath.Join(t.TempDir(), "actions.jsonl")
	fs, _ := OpenFileStore(path)
	var calls int32
	const n = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ex := newExec(fs, workerN(i), now)
			<-start
			ex.Do(context.Background(), selFor("t1", "k1"),
				countingDo(&calls, OutcomeConfirmedOK, "<ext>", nil, 20*time.Millisecond))
		}(i)
	}
	close(start)
	wg.Wait()
	if calls != 1 {
		t.Fatalf("FileStore concurrent run must execute the side effect once, got %d", calls)
	}
	cur, _ := fs.Get("t1", "k1")
	if cur.Status != StatusSucceeded {
		t.Fatalf("final FileStore record must be SUCCEEDED, got %s", cur.Status)
	}
}
