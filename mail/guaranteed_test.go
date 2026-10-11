package mail

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"voicesign-harness/action"
)

type stubSender struct {
	calls int32
	fn    func(m *OutboundMessage) (string, error)
}

func (s *stubSender) Send(_ context.Context, m *OutboundMessage) (string, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.fn != nil {
		return s.fn(m)
	}
	return "srv-1", nil
}

func testGuard() *OutboundGuard {
	return NewOutboundGuard(GuardPolicy{AllowedDomains: []string{"example.com", "voxsign.ai"}})
}

func testMsg(id, to string) *OutboundMessage {
	return &OutboundMessage{From: "martin@voxsign.ai", To: []string{to}, Subject: "s", TextBody: "b", MessageID: id}
}

func TestGuaranteedSendSuccess(t *testing.T) {
	store := action.NewMemoryStore()
	snd := &stubSender{}
	gs := NewGuaranteedSender(snd, store, "w1", testGuard(), nil)
	res, err := gs.Send(context.Background(), "tenantA", testMsg("m1", "qa@example.com"), SendIntent{})
	if err != nil || !res.Terminal() || res.Status != action.StatusSucceeded || res.ExternalRef != "srv-1" {
		t.Fatalf("expected SUCCEEDED with server id, got %+v err=%v", res, err)
	}
	if snd.calls != 1 {
		t.Fatalf("transport called once, got %d", snd.calls)
	}
}

func TestGuaranteedConcurrentOnce(t *testing.T) {
	store := action.NewMemoryStore()
	snd := &stubSender{fn: func(m *OutboundMessage) (string, error) {
		// wg inside test sleeps via time in action; emulate slow SMTP accept.
		for i := 0; i < 2_000_000; i++ { // brief busy window
		}
		return "srv-1", nil
	}}
	msg := testMsg("m2", "qa@example.com")
	const n = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		worker := NewGuaranteedSender(snd, store, fmt.Sprintf("w-%d", i), testGuard(), nil)
		go func() {
			defer wg.Done()
			<-start
			_, _ = worker.Send(context.Background(), "t", msg, SendIntent{})
		}()
	}
	close(start)
	wg.Wait()
	if snd.calls != 1 {
		t.Fatalf("transport must be called exactly once under concurrency, got %d", snd.calls)
	}
	rec, _ := store.Get("t", "m2")
	if rec.Status != action.StatusSucceeded {
		t.Fatalf("final state SUCCEEDED, got %s", rec.Status)
	}
}

func TestGuaranteedNoReceiptIsUnknownThenProbeConfirms(t *testing.T) {
	store := action.NewMemoryStore()
	snd := &stubSender{fn: func(m *OutboundMessage) (string, error) { return "", nil }} // no receipt
	probe := &fakeProbe{res: action.ProbeConfirmedDone, id: "late-server-id"}
	gs := NewGuaranteedSender(snd, store, "w1", testGuard(), probe)
	res, err := gs.Send(context.Background(), "t", testMsg("m3", "qa@example.com"), SendIntent{})
	if err != nil || res.Status != action.StatusUnknown || !res.NeedsReconcile {
		t.Fatalf("no receipt must be UNKNOWN/needs-reconcile, got %+v err=%v", res, err)
	}
	rc, err := gs.Reconcile(context.Background(), "t", "m3", testMsg("m3", "qa@example.com"))
	if err != nil || !rc.Terminal() || rc.ExternalRef != "late-server-id" || snd.calls != 1 {
		t.Fatalf("probe confirmed-done must resolve without resending, rc=%+v err=%v calls=%d", rc, err, snd.calls)
	}
}

func TestGuaranteedTransientTimeoutUnknown(t *testing.T) {
	store := action.NewMemoryStore()
	snd := &stubSender{fn: func(m *OutboundMessage) (string, error) {
		return "", errors.New("smtp: i/o timeout")
	}}
	probe := &fakeProbe{res: action.ProbeConfirmedDone, id: "srv-after-fact"}
	gs := NewGuaranteedSender(snd, store, "w1", testGuard(), probe)
	res, _ := gs.Send(context.Background(), "t", testMsg("m4", "qa@example.com"), SendIntent{})
	if res.Status != action.StatusUnknown {
		t.Fatalf("ambiguous timeout must be UNKNOWN, got %+v", res)
	}
	rc, _ := gs.Reconcile(context.Background(), "t", "m4", testMsg("m4", "qa@example.com"))
	if !rc.Terminal() || snd.calls != 1 {
		t.Fatalf("reconciliation must confirm the actually-sent mail without resend, rc=%+v calls=%d", rc, snd.calls)
	}
}

func TestGuaranteedRejectedIsFailedAndRetryable(t *testing.T) {
	store := action.NewMemoryStore()
	snd := &stubSender{}
	snd.fn = func(m *OutboundMessage) (string, error) {
		if atomic.LoadInt32(&snd.calls) == 1 {
			return "", AsRejected(errors.New("550 5.1.1 mailbox unavailable"))
		}
		return "srv-ok", nil
	}
	gs := NewGuaranteedSender(snd, store, "w1", testGuard(), nil)
	msg := testMsg("m5", "qa@example.com")
	r1, _ := gs.Send(context.Background(), "t", msg, SendIntent{})
	if r1.Status != action.StatusFailed || r1.Terminal() {
		t.Fatalf("permanent rejection must be retryable FAILED, got %+v", r1)
	}
	r2, err := gs.Send(context.Background(), "t", msg, SendIntent{})
	if err != nil || !r2.Terminal() || r2.Status != action.StatusSucceeded {
		t.Fatalf("retry after fixing the cause must succeed, got %+v err=%v", r2, err)
	}
	if snd.calls != 2 {
		t.Fatalf("transport attempts: want 2, got %d", snd.calls)
	}
}

func TestGuaranteedGuardBlocksBeforeLedger(t *testing.T) {
	store := action.NewMemoryStore()
	snd := &stubSender{}
	gs := NewGuaranteedSender(snd, store, "w1", testGuard(), nil)
	msg := testMsg("m6", "qa@example.com")
	if _, err := gs.Send(context.Background(), "t", msg, SendIntent{RawInstruction: "先别发"}); !errors.Is(err, ErrBlocked) {
		t.Fatalf("negation must block, got %v", err)
	}
	if snd.calls != 0 {
		t.Fatalf("blocked send must not touch the transport, calls=%d", snd.calls)
	}
	if _, err := store.Get("t", "m6"); !errors.Is(err, action.ErrNotFound) {
		t.Fatalf("a blocked send must not create an action record, got err=%v", err)
	}
}

func TestGuaranteedMissingKey(t *testing.T) {
	store := action.NewMemoryStore()
	snd := &stubSender{}
	gs := NewGuaranteedSender(snd, store, "w1", testGuard(), nil)
	m := testMsg("", "qa@example.com")
	if _, err := gs.Send(context.Background(), "t", m, SendIntent{}); !errors.Is(err, ErrMissingKey) {
		t.Fatalf("missing Message-ID must be rejected, got %v", err)
	}
	if snd.calls != 0 {
		t.Fatalf("keyless send must not touch transport, calls=%d", snd.calls)
	}
}

// Full crash/reconcile loop with the file transport: an unacknowledged first
// attempt (no receipt, nothing on disk) is proven not-done by the strict probe,
// reopened, and then sent exactly once.
func TestGuaranteedFileCrashReconcileLoop(t *testing.T) {
	root := t.TempDir()
	mb := NewFileMailbox(root)
	store, err := action.OpenFileStore(filepath.Join(root, "actions.jsonl"))
	if err != nil {
		t.Fatalf("open action store: %v", err)
	}
	snd := &stubSender{}
	snd.fn = func(m *OutboundMessage) (string, error) {
		if atomic.LoadInt32(&snd.calls) == 1 {
			return "", nil // crashed before acceptance receipt; nothing written
		}
		return mb.FileSentBox.Send(context.Background(), m)
	}
	gs := NewGuaranteedSender(snd, store, "w1", testGuard(), NewFileSentProbe(mb.FileSentBox))
	msg := testMsg("m7@test", "qa@example.com")
	r1, _ := gs.Send(context.Background(), "t", msg, SendIntent{})
	if r1.Status != action.StatusUnknown {
		t.Fatalf("first attempt must be UNKNOWN, got %+v", r1)
	}
	if mb.Contains("m7@test", "qa@example.com") {
		t.Fatalf("nothing should have been written on the unacknowledged attempt")
	}
	rc, _ := gs.Reconcile(context.Background(), "t", "m7@test", msg)
	if rc.Status != action.StatusPending || rc.NeedsReconcile {
		t.Fatalf("strict not-done probe must reopen to PENDING, got %+v", rc)
	}
	r2, err := gs.Send(context.Background(), "t", msg, SendIntent{})
	if err != nil || !r2.Terminal() {
		t.Fatalf("clean retry must succeed, got %+v err=%v", r2, err)
	}
	if !mb.Contains("m7@test", "qa@example.com") {
		t.Fatalf("the mail must now exist for its real recipient")
	}
	if mb.Contains("m7@test", "other@example.com") {
		t.Fatalf("unrelated recipient must not match")
	}
}

type fakeProbe struct {
	res action.ProbeResult
	id  string
}

func (p *fakeProbe) ProbeSent(_ context.Context, _, _ string, _ *OutboundMessage) (action.ProbeResult, string, error) {
	return p.res, p.id, nil
}
