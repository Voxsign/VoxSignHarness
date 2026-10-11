package action

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func clk() (func() time.Time, func(time.Duration)) {
	t := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	now := func() time.Time { return t }
	adv := func(d time.Duration) { t = t.Add(d) }
	return now, adv
}

func newExec(s Store, worker string, now func() time.Time) *Executor {
	return &Executor{Store: s, WorkerID: worker, Lease: time.Minute, Now: now}
}

func countingDo(counter *int32, outcome Outcome, ref string, err error, delay time.Duration) DoFunc {
	return func(ctx context.Context) (Outcome, string, error) {
		if counter != nil {
			atomic.AddInt32(counter, 1)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		return outcome, ref, err
	}
}

func selFor(tenant, key string) Selector {
	return Selector{Tenant: tenant, Key: key, Kind: "send_email"}
}

// 1. The worker that creates the action also executes it the first time.
func TestFirstCreatorExecutesOnce(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	ex := newExec(s, "w1", now)
	var calls int32
	res := ex.Do(context.Background(), selFor("t1", "k1"),
		countingDo(&calls, OutcomeConfirmedOK, "<ext-1>", nil, 0))
	if !res.Terminal() || res.Status != StatusSucceeded {
		t.Fatalf("expected SUCCEEDED, got %+v", res)
	}
	if calls != 1 || res.Attempts != 1 || res.ExternalRef != "<ext-1>" {
		t.Fatalf("creator must run exactly once, calls=%d attempts=%d ref=%q", calls, res.Attempts, res.ExternalRef)
	}
}

// 2. Many concurrent workers on one (tenant,key) produce exactly one external call.
func TestConcurrentSameActionExecutesOnce(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	var calls int32
	const n = 24
	var wg sync.WaitGroup
	results := make([]Result, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ex := newExec(s, workerN(i), now)
			<-start
			results[i] = ex.Do(context.Background(), selFor("t1", "k1"),
				countingDo(&calls, OutcomeConfirmedOK, "<ext-x>", nil, 25*time.Millisecond))
		}(i)
	}
	close(start)
	wg.Wait()
	if calls != 1 {
		t.Fatalf("external action must run exactly once, ran %d times", calls)
	}
	var succeeded int
	for _, r := range results {
		if r.Status == StatusSucceeded {
			succeeded++
		}
		if r.Status == StatusFailed || r.Status == StatusUnknown {
			t.Fatalf("no worker may report failed/unknown on a clean concurrent run: %+v", r)
		}
	}
	if succeeded == 0 {
		t.Fatalf("at least one worker must report SUCCEEDED")
	}
	cur, err := s.Get("t1", "k1")
	if err != nil || cur.Status != StatusSucceeded || cur.ExternalRef != "<ext-x>" {
		t.Fatalf("final record must be SUCCEEDED with ext ref, got %+v err=%v", cur, err)
	}
}

func workerN(i int) string {
	return "w-" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + "-" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	b := []byte{}
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// 3. A confirmed action is idempotent: a second Do never calls the external side.
func TestSucceededIsIdempotentSkip(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	ex := newExec(s, "w1", now)
	var calls int32
	do := countingDo(&calls, OutcomeConfirmedOK, "<ext-1>", nil, 0)
	if r := ex.Do(context.Background(), selFor("t1", "k1"), do); !r.Terminal() {
		t.Fatalf("first must succeed: %+v", r)
	}
	r2 := ex.Do(context.Background(), selFor("t1", "k1"), do)
	if !r2.Terminal() || !r2.AlreadyDone || calls != 1 {
		t.Fatalf("second Do must be an idempotent already-done skip, got %+v calls=%d", r2, calls)
	}
}

// 4. An action in flight on another worker is PENDING/in-progress, NEVER success,
// and the second worker must not perform the side effect.
func TestInFlightNotReportedSuccess(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	sel := selFor("t1", "k1")
	if _, dec, err := s.Acquire(sel, "workerA", time.Minute, now()); err != nil || dec != DecisionAcquired {
		t.Fatalf("workerA acquire: dec=%v err=%v", dec, err)
	}
	var calls int32
	exB := newExec(s, "workerB", now)
	res := exB.Do(context.Background(), sel, countingDo(&calls, OutcomeConfirmedOK, "<refB>", nil, 0))
	if !res.InProgress || res.Status != StatusPending || res.Terminal() {
		t.Fatalf("must report in-progress PENDING, got %+v", res)
	}
	if calls != 0 {
		t.Fatalf("workerB must not perform the side effect while A holds the lease, calls=%d", calls)
	}
	cur, _ := s.Get("t1", "k1")
	if cur.Status != StatusPending {
		t.Fatalf("record must remain PENDING, got %s", cur.Status)
	}
}

// 5. A durable external rejection is FAILED (retryable), never done.
func TestConfirmedRejectedMarksFailed(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	ex := newExec(s, "w1", now)
	var calls int32
	res := ex.Do(context.Background(), selFor("t1", "k1"),
		countingDo(&calls, OutcomeConfirmedRejected, "", nil, 0))
	if res.Status != StatusFailed || res.Terminal() {
		t.Fatalf("rejection must be FAILED and not terminal, got %+v", res)
	}
	cur, _ := s.Get("t1", "k1")
	if cur.Status != StatusFailed {
		t.Fatalf("record must be FAILED, got %s", cur.Status)
	}
	// FAILED is retryable: an atomic re-claim succeeds.
	rec, dec, err := s.Acquire(selFor("t1", "k1"), "w2", time.Minute, now())
	if err != nil || dec != DecisionAcquired {
		t.Fatalf("FAILED must be re-claimable, dec=%v err=%v", dec, err)
	}
	if rec.Status != StatusPending || rec.Owner != "w2" {
		t.Fatalf("re-claimed record must be PENDING owned by w2, got %+v", rec)
	}
}

// 6. THE core false-success bug: external call returns OK but the ledger write
// fails. The result MUST be UNKNOWN (never SUCCEEDED), flagged for reconcile.
func TestLedgerMarkFailureAfterConfirmedOKIsUnknown(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	s.FailMark = true
	ex := newExec(s, "w1", now)
	var sends int32
	res := ex.Do(context.Background(), selFor("t1", "k1"),
		countingDo(&sends, OutcomeConfirmedOK, "<ext-sent>", nil, 0))
	if sends != 1 {
		t.Fatalf("external send happened once, got %d", sends)
	}
	if res.Status != StatusUnknown || res.Terminal() || !res.NeedsReconcile {
		t.Fatalf("ledger mark failure after OK must be UNKNOWN/needs-reconcile, got %+v", res)
	}
	if res.ExternalRef != "<ext-sent>" {
		t.Fatalf("uncertain result must retain the external correlation ref, got %q", res.ExternalRef)
	}
}

// 7. Handler error and 8. inconclusive outcome both resolve to UNKNOWN, and a
// later Acquire must force reconciliation rather than blindly replay.
func TestHandlerErrorAndInconclusiveAreUnknown(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	sel := selFor("t1", "k1")

	ex := newExec(s, "w1", now)
	r1 := ex.Do(context.Background(), sel, countingDo(nil, OutcomeInconclusive, "", nil, 0))
	if r1.Status != StatusUnknown || !r1.NeedsReconcile {
		t.Fatalf("inconclusive must be UNKNOWN/needs-reconcile, got %+v", r1)
	}
	// A fresh worker must not replay an UNKNOWN record without evidence.
	ex2 := newExec(s, "w2", now)
	var calls int32
	r2 := ex2.Do(context.Background(), sel, countingDo(&calls, OutcomeConfirmedOK, "<x>", nil, 0))
	if !r2.NeedsReconcile || calls != 0 {
		t.Fatalf("UNKNOWN must not be replayed without reconciliation, got %+v calls=%d", r2, calls)
	}

	// Handler error path.
	s2 := NewMemoryStore()
	ex3 := newExec(s2, "w1", now)
	r3 := ex3.Do(context.Background(), selFor("t2", "k2"),
		countingDo(nil, OutcomeConfirmedOK, "", errors.New("smtp timeout"), 0))
	if r3.Status != StatusUnknown || r3.Err == nil {
		t.Fatalf("handler error must be UNKNOWN with cause, got %+v", r3)
	}
}

// 9. Fencing + crash safety: when A's lease expires, B must not directly take
// over a dead PENDING (the side effect may already have happened). B reconciles;
// only after a strong not-done finding does it reopen and acquire. A stale write
// from A with its old version is then rejected; B's write stands.
func TestFencingStaleOwnerCannotOverwrite(t *testing.T) {
	now, adv := clk()
	s := NewMemoryStore()
	sel := selFor("t1", "k1")
	recA, decA, _ := s.Acquire(sel, "A", 10*time.Second, now())
	if decA != DecisionAcquired {
		t.Fatalf("A acquire %v", decA)
	}
	adv(11 * time.Second) // A's lease expires (A may have crashed after sending)

	if _, decB0, _ := s.Acquire(sel, "B", 10*time.Second, now()); decB0 != DecisionNeedsReconcile {
		t.Fatalf("a dead PENDING must require reconciliation, got %v", decB0)
	}
	exB := newExec(s, "B", now)
	rr, err := exB.Reconcile(context.Background(), "t1", "k1",
		func(ctx context.Context, rec Record) (ProbeResult, string, error) {
			return ProbeConfirmedNotDone, "", nil
		})
	if err != nil || rr.Status != StatusPending {
		t.Fatalf("reconcile not-done must reopen to PENDING, got %+v err=%v", rr, err)
	}
	recB, decB, _ := s.Acquire(sel, "B", 10*time.Second, now())
	if decB != DecisionAcquired || recB.Owner != "B" || recB.Version <= recA.Version {
		t.Fatalf("B must acquire after reopen with a higher version, got A=v%d B=%+v dec=%v", recA.Version, recB, decB)
	}
	_, errA := s.Mark("t1", "k1", recA.Version, func(r *Record) {
		r.Status = StatusSucceeded
		r.ExternalRef = "<from-stale-A>"
	}, now())
	if !errors.Is(errA, ErrFence) {
		t.Fatalf("stale owner Mark must return ErrFence, got %v", errA)
	}
	_, errB := s.Mark("t1", "k1", recB.Version, func(r *Record) {
		r.Status = StatusSucceeded
		r.ExternalRef = "<from-B>"
	}, now())
	if errB != nil {
		t.Fatalf("current owner Mark should succeed, got %v", errB)
	}
	cur, _ := s.Get("t1", "k1")
	if cur.Status != StatusSucceeded || cur.ExternalRef != "<from-B>" {
		t.Fatalf("new owner's result must stand, got %+v", cur)
	}
}

// 10. Retrying a FAILED action is an atomic claim: concurrent retriers, one wins.
func TestFailedRetryAtomicClaim(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	sel := selFor("t1", "k1")
	ex := newExec(s, "w0", now)
	if r := ex.Do(context.Background(), sel, countingDo(nil, OutcomeConfirmedRejected, "", nil, 0)); r.Status != StatusFailed {
		t.Fatalf("seed FAILED, got %+v", r)
	}
	var acquired, inflight int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, dec, _ := s.Acquire(sel, workerN(i), time.Minute, now())
			switch dec {
			case DecisionAcquired:
				atomic.AddInt64(&acquired, 1)
			case DecisionInFlight:
				atomic.AddInt64(&inflight, 1)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if acquired != 1 {
		t.Fatalf("exactly one retrier must acquire, got acquired=%d inflight=%d", acquired, inflight)
	}
}

// 11. Tenant isolation: same key in different tenants is fully independent.
func TestTenantIsolation(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	var aCalls, bCalls int32
	exA := newExec(s, "wA", now)
	exB := newExec(s, "wB", now)
	rA := exA.Do(context.Background(), selFor("tenantA", "same-key"),
		countingDo(&aCalls, OutcomeConfirmedOK, "<refA>", nil, 0))
	rB := exB.Do(context.Background(), selFor("tenantB", "same-key"),
		countingDo(&bCalls, OutcomeConfirmedOK, "<refB>", nil, 0))
	if !rA.Terminal() || !rB.Terminal() || aCalls != 1 || bCalls != 1 {
		t.Fatalf("each tenant must execute independently: A=%+v B=%+v", rA, rB)
	}
	gotA, _ := s.Get("tenantA", "same-key")
	gotB, _ := s.Get("tenantB", "same-key")
	if gotA.ExternalRef != "<refA>" || gotB.ExternalRef != "<refB>" {
		t.Fatalf("tenant records must not mix: A=%q B=%q", gotA.ExternalRef, gotB.ExternalRef)
	}
	if _, err := s.Get("tenantA", "nonexistent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing key must be ErrNotFound, got %v", err)
	}
}

// 12-14. Reconciler: confirmed-done resolves; unknown stays unresolved (no
// replay); confirmed-not-done reopens for a clean retry.
func TestReconcileOutcomes(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	sel := selFor("t1", "k1")
	ex := newExec(s, "w1", now)
	if r := ex.Do(context.Background(), sel, countingDo(nil, OutcomeInconclusive, "", nil, 0)); r.Status != StatusUnknown {
		t.Fatalf("seed UNKNOWN, got %+v", r)
	}

	// Probe Unknown -> stays UNKNOWN, no replay.
	r1, err := ex.Reconcile(context.Background(), "t1", "k1",
		func(ctx context.Context, rec Record) (ProbeResult, string, error) { return ProbeUnknown, "", nil })
	if err != nil || r1.Status != StatusUnknown || !r1.NeedsReconcile {
		t.Fatalf("unknown probe must keep UNKNOWN, got %+v err=%v", r1, err)
	}

	// Probe ConfirmedDone (strictly matched by the adapter) -> SUCCEEDED.
	r2, err := ex.Reconcile(context.Background(), "t1", "k1",
		func(ctx context.Context, rec Record) (ProbeResult, string, error) { return ProbeConfirmedDone, "<server-id-9>", nil })
	if err != nil || !r2.Terminal() || r2.ExternalRef != "<server-id-9>" {
		t.Fatalf("confirmed-done must resolve to SUCCEEDED, got %+v err=%v", r2, err)
	}

	// A second UNKNOWN action that the probe proves never happened reopens.
	sel2 := selFor("t1", "k2")
	if r := ex.Do(context.Background(), sel2, countingDo(nil, OutcomeInconclusive, "", nil, 0)); r.Status != StatusUnknown {
		t.Fatalf("seed UNKNOWN k2, got %+v", r)
	}
	r3, err := ex.Reconcile(context.Background(), "t1", "k2",
		func(ctx context.Context, rec Record) (ProbeResult, string, error) { return ProbeConfirmedNotDone, "", nil })
	if err != nil || r3.Status != StatusPending || r3.NeedsReconcile {
		t.Fatalf("confirmed-not-done must reopen to PENDING, got %+v err=%v", r3, err)
	}
	rec, dec, err := s.Acquire(sel2, "w2", time.Minute, now())
	if err != nil || dec != DecisionAcquired || rec.Status != StatusPending {
		t.Fatalf("reopened action must be acquirable for a clean retry, dec=%v rec=%+v err=%v", dec, rec, err)
	}
}

// 15. The kernel never infers success from an unrelated "sent" item; strict
// recipient/tenant/correlation matching is the probe's job. An unrelated sent
// item yields ProbeUnknown and the action stays unresolved.
func TestUnrelatedSentItemDoesNotConfirm(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	sel := selFor("tenantA", "k9")
	ex := newExec(s, "w1", now)
	if r := ex.Do(context.Background(), sel, countingDo(nil, OutcomeInconclusive, "", nil, 0)); r.Status != StatusUnknown {
		t.Fatalf("seed UNKNOWN, got %+v", r)
	}
	// Simulate a strict probe: sent-items contain a message, but for a different
	// recipient/correlation -> it must return Unknown, not ConfirmedDone.
	strictProbe := func(ctx context.Context, rec Record) (ProbeResult, string, error) {
		sentItems := []struct{ tenant, msgID, to string }{
			{"tenantA", "other-msg", "other@example.com"},
			{"tenantB", rec.Key, "qa@example.com"},
		}
		for _, it := range sentItems {
			if it.tenant == rec.Tenant && it.msgID == rec.Key { // recipient also checked in real probe
				return ProbeConfirmedDone, it.msgID, nil
			}
		}
		return ProbeUnknown, "", nil
	}
	r, err := ex.Reconcile(context.Background(), "tenantA", "k9", strictProbe)
	if err != nil || r.Status != StatusUnknown {
		t.Fatalf("unrelated sent item must not confirm the action, got %+v err=%v", r, err)
	}
}

// 16. Even goroutines sharing one worker id (same-process duplicate dispatch)
// must not re-run a PENDING action while its lease is live.
func TestSameWorkerConcurrentExecutesOnce(t *testing.T) {
	now, _ := clk()
	s := NewMemoryStore()
	var calls int32
	const n = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ex := newExec(s, "same-worker", now)
			<-start
			ex.Do(context.Background(), selFor("t1", "ksame"),
				countingDo(&calls, OutcomeConfirmedOK, "<x>", nil, 15*time.Millisecond))
		}()
	}
	close(start)
	wg.Wait()
	if calls != 1 {
		t.Fatalf("same-worker concurrent Do must execute the side effect once, got %d", calls)
	}
}
