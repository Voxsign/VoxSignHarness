// Package action is the kernel's idempotent ledger for external side effects
// (sending an email, opening a ticket, calling a customer API).
//
// It makes one guarantee precise: an external action identified by a stable
// (tenant, key) is executed at most once per confirmed outcome, and a task is
// reported as done ONLY when the external effect is confirmed AND that
// confirmation is durably recorded. In particular:
//
//   - "Skipped because an identical action is in flight on another worker" is
//     NEVER reported as success. It returns InProgress and leaves the action
//     PENDING.
//   - The worker that creates an action is also its first executor; creating a
//     PENDING record and then refusing to run it is a bug, not a feature.
//
// *- When the external call succeeded but the ledger write failed, or the
//     process crashed between the external call and the status write, the
//     action is UNKNOWN. It is never optimistically replayed; a Reconciler with
//     a strong external probe resolves it.
//   - Ownership uses a monotonic fencing version: a worker whose lease expired
//     and whose ownership was taken over cannot overwrite the new owner.
//
// Tenant isolation is mandatory: every lookup is scoped by tenant, so one
// customer can never confirm, replay or observe another customer's actions.
//
// Scope note: this package records and guards outbound actions the kernel asks
// for; it is a client-side guard, NOT a mail system and NOT an MTA. It sends no
// mail itself — a customer adapter's handler does the actual transport.
package action

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Status is the durable state of one external action.
type Status string

const (
	// StatusPending means the action is created or running and its external
	// outcome is not confirmed yet.
	StatusPending Status = "PENDING"
	// StatusSucceeded means the external system durably accepted the effect AND
	// this fact is recorded. This is the only state that means "done".
	StatusSucceeded Status = "SUCCEEDED"
	// StatusFailed means the external system durably rejected the effect. It is
	// retryable via an atomic re-claim.
	StatusFailed Status = "FAILED"
	// StatusUnknown means the effect may or may not have happened; do not replay
	// without a strong external reconciliation.
	StatusUnknown Status = "UNKNOWN"
)

// Sentinel errors.
var (
	// ErrFence means the writer held a stale version; ownership moved on.
	ErrFence = errors.New("action: fencing token stale, ownership changed")
	// ErrNotFound means no action exists for the (tenant, key).
	ErrNotFound = errors.New("action: not found")
	// ErrNotReopenable means the action cannot be reopened in its current state.
	ErrNotReopenable = errors.New("action: not reopenable in current state")
)

// Outcome reports what the handler established about the external call. A
// handler MUST be conservative: confirm success only on durable acceptance.
type Outcome int

const (
	// OutcomeInconclusive means acceptance could not be determined (timeout,
	// ambiguous response, crash window). The action becomes UNKNOWN.
	OutcomeInconclusive Outcome = iota
	// OutcomeConfirmedOK means the external system durably accepted the effect;
	// externalRef must be its stable correlation id.
	OutcomeConfirmedOK
	// OutcomeConfirmedRejected means the external system durably refused.
	OutcomeConfirmedRejected
)

// Selector identifies one logical external action. Key must be a stable
// idempotency key derived from the external correlation (e.g. the inbound
// message id + action kind + normalized recipient). Kind is metadata only and
// is not part of the uniqueness key.
type Selector struct {
	Tenant string
	Key    string
	Kind   string
}

// Record is the durable ledger entry for one action. It stores correlation
// metadata, never message bodies or secrets.
type Record struct {
	Tenant      string    `json:"tenant"`
	Key         string    `json:"key"`
	Kind        string    `json:"kind,omitempty"`
	Status      Status    `json:"status"`
	Owner       string    `json:"owner,omitempty"`
	LeaseUntil  time.Time `json:"lease_until,omitempty"`
	Version     int64     `json:"version"`
	Attempts    int       `json:"attempts"`
	ExternalRef string    `json:"external_ref,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (r Record) clone() Record { return r }

// AcquireDecision tells the caller why Acquire returned without a lease.
type AcquireDecision int

const (
	// DecisionAcquired: the caller now owns an exclusive lease and MUST run the
	// action exactly once, then Mark with the returned Version.
	DecisionAcquired AcquireDecision = iota
	// DecisionAlreadySucceeded: a prior confirmed action exists; do nothing.
	DecisionAlreadySucceeded
	// DecisionInFlight: another live owner holds a valid lease; do nothing now.
	DecisionInFlight
	// DecisionNeedsReconcile: an expired UNKNOWN/PENDING record may already have
	// an external effect. Reconcile before any replay.
	DecisionNeedsReconcile
)

// Store is the persistence boundary for the action ledger. Implementations
// must serialise Acquire/Mark per (tenant, key); an in-memory implementation is
// provided and durable implementations (JSONL/SQL) satisfy the same contract.
type Store interface {
	// Acquire atomically creates-and-claims a new action, or takes over an
	// expired lease, or reports why it did not acquire. It closes the
	// create-then-claim race window in a single critical section.
	Acquire(sel Selector, owner string, lease time.Duration, now time.Time) (Record, AcquireDecision, error)
	// Mark durably writes a result, but only if expectVersion still matches the
	// current version; otherwise it returns ErrFence and changes nothing.
	Mark(tenant, key string, expectVersion int64, mutate func(*Record), now time.Time) (Record, error)
	// Get returns the current record for a tenant-scoped key.
	Get(tenant, key string) (Record, error)
	// Reopen moves an UNKNOWN (or FAILED) record back to PENDING after a strong
	// external probe has established the effect did not happen. It is the only
	// sanctioned path that permits re-execution after an inconclusive result.
	Reopen(tenant, key string, expectVersion int64, now time.Time) (Record, error)
}

// DoFunc performs the real side effect. It must be conservative: return
// OutcomeConfirmedOK only after durable external acceptance (with a stable
// externalRef); OutcomeConfirmedRejected on durable refusal; and
// OutcomeInconclusive (or a non-nil error) whenever acceptance is uncertain.
type DoFunc func(ctx context.Context) (outcome Outcome, externalRef string, err error)

// Result is what Executor.Do returns to the caller. Exactly one terminal state
// is meaningful together with its flags; callers must switch on Status and
// must not treat InProgress/NeedsReconcile as completion.
type Result struct {
	Status         Status
	AlreadyDone    bool
	InProgress     bool
	NeedsReconcile bool
	ExternalRef    string
	Attempts       int
	Err            error
}

// Terminal reports whether the action reached a confirmed terminal state.
func (r Result) Terminal() bool { return r.Status == StatusSucceeded }

// Executor runs external actions exactly-once-under-confirmation against a
// Store. It is safe for concurrent use by many workers.
type Executor struct {
	Store    Store
	WorkerID string
	Lease    time.Duration
	Now      func() time.Time
}

func (e *Executor) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Executor) leaseFor() time.Duration {
	if e.Lease > 0 {
		return e.Lease
	}
	return 30 * time.Second
}

// Do executes the action for (tenant, key) under the ledger protocol.
func (e *Executor) Do(ctx context.Context, sel Selector, do DoFunc) Result {
	rec, decision, err := e.Store.Acquire(sel, e.WorkerID, e.leaseFor(), e.now())
	if err != nil {
		return Result{Status: StatusUnknown, Err: err}
	}
	switch decision {
	case DecisionAlreadySucceeded:
		return Result{Status: StatusSucceeded, AlreadyDone: true, ExternalRef: rec.ExternalRef, Attempts: rec.Attempts}
	case DecisionInFlight:
		return Result{Status: StatusPending, InProgress: true, Attempts: rec.Attempts}
	case DecisionNeedsReconcile:
		return Result{Status: StatusUnknown, NeedsReconcile: true, Attempts: rec.Attempts}
	}

	version := rec.Version
	outcome, ref, doErr := do(ctx)

	switch outcome {
	case OutcomeConfirmedOK:
		if doErr != nil {
			return e.markInconclusive(sel, version, ref, doErr)
		}
		mr, merr := e.Store.Mark(sel.Tenant, sel.Key, version, func(r *Record) {
			r.Status = StatusSucceeded
			r.ExternalRef = ref
			r.LastError = ""
			r.Attempts++
			r.releaseLease()
		}, e.now())
		if merr != nil {
			// The external effect may have happened, but we could not durably
			// record it. Never report success in this case.
			return Result{Status: StatusUnknown, ExternalRef: ref, NeedsReconcile: true,
				Err: fmt.Errorf("external effect confirmed but ledger mark failed: %w", merr)}
		}
		return Result{Status: StatusSucceeded, ExternalRef: mr.ExternalRef, Attempts: mr.Attempts}

	case OutcomeConfirmedRejected:
		mr, merr := e.Store.Mark(sel.Tenant, sel.Key, version, func(r *Record) {
			r.Status = StatusFailed
			r.LastError = "external system rejected the action"
			r.Attempts++
			r.releaseLease()
		}, e.now())
		if merr != nil {
			return Result{Status: StatusUnknown, NeedsReconcile: true, Err: merr}
		}
		return Result{Status: StatusFailed, Attempts: mr.Attempts,
			Err: errors.New("external action confirmed rejected")}

	default: // OutcomeInconclusive, including handler error / timeout / crash window
		return e.markInconclusive(sel, version, ref, doErr)
	}
}

func (e *Executor) markInconclusive(sel Selector, version int64, ref string, cause error) Result {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	mr, merr := e.Store.Mark(sel.Tenant, sel.Key, version, func(r *Record) {
		r.Status = StatusUnknown
		if ref != "" {
			r.ExternalRef = ref
		}
		r.LastError = msg
		// Release the lease so a reconciler can pick it up; Acquire on an
		// UNKNOWN record still refuses to replay without external evidence.
		r.Owner = ""
		r.LeaseUntil = time.Time{}
	}, e.now())
	if merr != nil {
		// Even the UNKNOWN marker could not be persisted: report nothing more
		// optimistic than UNKNOWN.
		return Result{Status: StatusUnknown, ExternalRef: ref, NeedsReconcile: true,
			Err: fmt.Errorf("inconclusive outcome and ledger mark failed: %w", merr)}
	}
	err := cause
	if err == nil {
		err = errors.New("external outcome inconclusive")
	}
	return Result{Status: StatusUnknown, ExternalRef: mr.ExternalRef, NeedsReconcile: true,
		Attempts: mr.Attempts, Err: err}
}

// ProbeResult is the reconciler's external finding.
type ProbeResult int

const (
	// ProbeUnknown: no strong evidence either way (sent-items delay, paging,
	// permission gaps). The action stays UNKNOWN and is not replayed.
	ProbeUnknown ProbeResult = iota
	// ProbeConfirmedDone: strong evidence the effect exists externally, matched
	// by stable correlation id AND tenant AND normalized recipient.
	ProbeConfirmedDone
	// ProbeConfirmedNotDone: strong, verifiable evidence the effect was not
	// accepted (explicit rejection / non-submission under a consistency
	// guarantee). Re-execution is then permitted.
	ProbeConfirmedNotDone
)

// Probe inspects the external system for one record. Implementations own the
// strict matching rules; an unrelated "sent" item (different recipient, tenant
// or correlation id) must NOT count as ProbeConfirmedDone.
type Probe func(ctx context.Context, rec Record) (result ProbeResult, externalRef string, err error)

// Reconcile resolves an UNKNOWN / stale-PENDING action using an external probe.
// It never re-executes directly: ConfirmedNotDone reopens the record so the
// scheduler can Acquire and run it again cleanly.
func (e *Executor) Reconcile(ctx context.Context, tenant, key string, probe Probe) (Result, error) {
	cur, err := e.Store.Get(tenant, key)
	if err != nil {
		return Result{Status: StatusUnknown, Err: err}, err
	}
	if cur.Status == StatusSucceeded {
		return Result{Status: StatusSucceeded, AlreadyDone: true, ExternalRef: cur.ExternalRef, Attempts: cur.Attempts}, nil
	}
	res, ref, perr := probe(ctx, cur)
	if perr != nil {
		// Probe itself failed: stay unresolved, do not replay.
		return Result{Status: StatusUnknown, NeedsReconcile: true, Err: perr}, nil
	}
	switch res {
	case ProbeConfirmedDone:
		mr, merr := e.Store.Mark(tenant, key, cur.Version, func(r *Record) {
			r.Status = StatusSucceeded
			if ref != "" {
				r.ExternalRef = ref
			}
			r.LastError = ""
			r.releaseLease()
		}, e.now())
		if merr != nil {
			return Result{Status: StatusUnknown, NeedsReconcile: true, Err: merr}, merr
		}
		return Result{Status: StatusSucceeded, ExternalRef: mr.ExternalRef, Attempts: mr.Attempts}, nil
	case ProbeConfirmedNotDone:
		if _, rerr := e.Store.Reopen(tenant, key, cur.Version, e.now()); rerr != nil {
			return Result{Status: cur.Status, NeedsReconcile: true, Err: rerr}, rerr
		}
		return Result{Status: StatusPending, NeedsReconcile: false, Err: nil}, nil
	default:
		return Result{Status: StatusUnknown, NeedsReconcile: true}, nil
	}
}

func (r *Record) releaseLease() {
	r.Owner = ""
	r.LeaseUntil = time.Time{}
}

// MemoryStore is a concurrency-safe, zero-dependency Store for tests and single
// -process embeddings. Durable deployments wrap or replace it with a JSONL/SQL
// implementation of the same interface.
type MemoryStore struct {
	mu       sync.Mutex
	rows     map[recKey]*Record
	FailMark bool // test injection: Mark fails without mutating state
}

type recKey struct {
	tenant string
	key    string
}

// NewMemoryStore returns an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{rows: map[recKey]*Record{}}
}

// Acquire implements Store.
func (s *MemoryStore) Acquire(sel Selector, owner string, lease time.Duration, now time.Time) (Record, AcquireDecision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := recKey{sel.Tenant, sel.Key}
	out, dec := acquireLogic(s.rows[k], sel, owner, lease, now)
	if dec == DecisionAcquired {
		cp := out
		s.rows[k] = &cp
	}
	return out, dec, nil
}

func (r *Record) takeOver(owner string, lease time.Duration, now time.Time) {
	r.Owner = owner
	r.LeaseUntil = now.Add(lease)
	r.Version++
	r.Status = StatusPending
	r.UpdatedAt = now
}

// Mark implements Store with optimistic concurrency via Version.
func (s *MemoryStore) Mark(tenant, key string, expectVersion int64, mutate func(*Record), now time.Time) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[recKey{tenant, key}]
	if !ok {
		return Record{}, ErrNotFound
	}
	out, err := markLogic(*r, expectVersion, mutate, now)
	if err != nil {
		return out, err
	}
	if s.FailMark {
		// Injection point for tests: a persistence failure must not mutate the
		// in-memory state, mirroring a failed fsync/SQL write.
		return r.clone(), errors.New("action: injected persistence failure")
	}
	cp := out
	s.rows[recKey{tenant, key}] = &cp
	return out, nil
}

// Get implements Store.
func (s *MemoryStore) Get(tenant, key string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[recKey{tenant, key}]
	if !ok {
		return Record{}, ErrNotFound
	}
	return r.clone(), nil
}

// Reopen implements Store.
func (s *MemoryStore) Reopen(tenant, key string, expectVersion int64, now time.Time) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[recKey{tenant, key}]
	if !ok {
		return Record{}, ErrNotFound
	}
	out, err := reopenLogic(*r, expectVersion, now)
	if err != nil {
		return out, err
	}
	cp := out
	s.rows[recKey{tenant, key}] = &cp
	return out, nil
}
