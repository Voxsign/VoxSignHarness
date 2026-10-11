package mail

import (
	"context"
	"errors"
	"fmt"
	"time"

	"voicesign-harness/action"
)

// ErrMissingKey is returned when an outbound message has no Message-ID. The
// Message-ID is the stable idempotency key; callers (a thread tracker, a
// business workflow) must supply one so retries and crash recovery collapse to
// the same logical send instead of creating duplicates.
var ErrMissingKey = errors.New("mail: outbound Message-ID is required as the idempotency key")

// ErrBlocked wraps an OutboundGuard rejection.
var ErrBlocked = errors.New("mail: send blocked by outbound guard")

// SenderErrorClass distinguishes a permanent rejection from an ambiguous
// transport failure.
type SenderErrorClass int

const (
	// SendErrorTransient is a timeout/connection/4xx: the message may or may not
	// have been accepted, so the outcome is UNKNOWN and must be reconciled.
	SendErrorTransient SenderErrorClass = iota
	// SendErrorRejected is a permanent refusal (auth, 5xx, bad recipient): the
	// side effect definitely did not happen, so the action is a retryable FAILED.
	SendErrorRejected
)

type classifiedError struct {
	err   error
	class SenderErrorClass
}

func (e *classifiedError) Error() string { return e.err.Error() }
func (e *classifiedError) Unwrap() error { return e.err }

// AsRejected marks err as a permanent rejection (effect definitely absent).
func AsRejected(err error) error {
	if err == nil {
		return nil
	}
	return &classifiedError{err: err, class: SendErrorRejected}
}

func classifySendError(err error) SenderErrorClass {
	var ce *classifiedError
	if errors.As(err, &ce) {
		return ce.class
	}
	return SendErrorTransient
}

// SentProbe inspects the external world for proof about a specific send.
// Implementations MUST match tenant, correlation id and recipient strictly.
type SentProbe interface {
	ProbeSent(ctx context.Context, tenant, key string, m *OutboundMessage) (action.ProbeResult, string, error)
}

// GuaranteedSender wraps a Sender with the idempotent action ledger, the
// outbound guard and external reconciliation. It guarantees at-most-one real
// send per (tenant, Message-ID) and never reports success without either a
// provider acceptance receipt or a confirmed external probe.
type GuaranteedSender struct {
	sender Sender
	store  action.Store
	guard  *OutboundGuard // optional
	probe  SentProbe      // optional
	worker string
	lease  time.Duration
	now    func() time.Time
}

// NewGuaranteedSender wires a transport sender through the reliability layer.
func NewGuaranteedSender(sender Sender, store action.Store, worker string, guard *OutboundGuard, probe SentProbe) *GuaranteedSender {
	g := &GuaranteedSender{sender: sender, store: store, guard: guard, probe: probe, worker: worker, lease: time.Minute}
	g.now = time.Now
	return g
}

// WithClock overrides the clock (tests).
func (g *GuaranteedSender) WithClock(now func() time.Time) *GuaranteedSender { g.now = now; return g }

// WithLease overrides the in-flight lease duration.
func (g *GuaranteedSender) WithLease(d time.Duration) *GuaranteedSender { g.lease = d; return g }

func (g *GuaranteedSender) executor() *action.Executor {
	return &action.Executor{Store: g.store, WorkerID: g.worker, Lease: g.lease, Now: g.now}
}

// Send validates the message, then performs the idempotent send.
func (g *GuaranteedSender) Send(ctx context.Context, tenant string, m *OutboundMessage, intent SendIntent) (action.Result, error) {
	if m.MessageID == "" {
		return action.Result{}, ErrMissingKey
	}
	if g.guard != nil {
		if err := g.guard.Check(m, intent); err != nil {
			return action.Result{}, fmt.Errorf("%w: %v", ErrBlocked, err)
		}
	}
	sel := action.Selector{Tenant: tenant, Key: m.MessageID, Kind: "email.send"}
	return g.executor().Do(ctx, sel, func(ctx context.Context) (action.Outcome, string, error) {
		serverID, err := g.sender.Send(ctx, m)
		if err != nil {
			if classifySendError(err) == SendErrorRejected {
				return action.OutcomeConfirmedRejected, "", err
			}
			return action.OutcomeInconclusive, serverID, err
		}
		if serverID == "" {
			// No acceptance receipt from the transport: do not assume success.
			return action.OutcomeInconclusive, "", nil
		}
		return action.OutcomeConfirmedOK, serverID, nil
	}), nil
}

// Reconcile uses the external probe to resolve an in-flight/unknown send.
func (g *GuaranteedSender) Reconcile(ctx context.Context, tenant, key string, m *OutboundMessage) (action.Result, error) {
	if g.probe == nil {
		return action.Result{}, errors.New("mail: no sent probe configured")
	}
	return g.executor().Reconcile(ctx, tenant, key, func(ctx context.Context, rec action.Record) (action.ProbeResult, string, error) {
		return g.probe.ProbeSent(ctx, tenant, key, m)
	})
}

// fileSentProbe adapts the reference FileSentBox. The file transport writes
// synchronously, so absence there is authoritative. IMAP/Graph adapters must
// return ProbeUnknown on incomplete or delayed sent-folder scans instead.
type fileSentProbe struct {
	box *FileSentBox
}

// NewFileSentProbe returns a strict probe over a FileSentBox.
func NewFileSentProbe(box *FileSentBox) SentProbe { return &fileSentProbe{box: box} }

func (p *fileSentProbe) ProbeSent(_ context.Context, tenant, key string, m *OutboundMessage) (action.ProbeResult, string, error) {
	recip := ""
	if len(m.To) > 0 {
		recip = m.To[0]
	}
	if p.box.Contains(key, recip) {
		return action.ProbeConfirmedDone, key, nil
	}
	return action.ProbeConfirmedNotDone, "", nil
}
