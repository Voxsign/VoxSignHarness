package mail

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ChaseStage is the lifecycle of a request that was dispatched over a mail
// thread and is being chased for a response or completion.
type ChaseStage string

const (
	StageDispatched   ChaseStage = "dispatched"    // request sent, awaiting acknowledgement
	StageAcknowledged ChaseStage = "acknowledged"  // owner accepted the request
	StageInProgress   ChaseStage = "in_progress"   // owner reports work underway
	StageDone         ChaseStage = "done"          // resolved
	StageCancelled    ChaseStage = "cancelled"     // explicitly called off
)

// Chase tracks one outstanding request by its root Message-ID. All follow-ups
// and escalations stay on the same mail thread (In-Reply-To/References).
type Chase struct {
	Tenant         string
	Key            string // root Message-ID, the stable correlation key
	Owner          string // responsible seat to nudge
	Operator       string // seat to escalate to when the owner stays silent
	Stage          ChaseStage
	CreatedAt      time.Time
	LastActivityAt time.Time // last inbound acknowledgement/progress/done
	FirstNudgeAt   time.Time
	LastNudgeAt    time.Time
	EscalatedAt    time.Time
	Nudges         int
	Version        int // optimistic-concurrency fencing token
}

// ChasePolicy defines the SLA timers for chasing. Zero values fall back to
// DefaultChasePolicy through NewTracker.
type ChasePolicy struct {
	AckTimeout    time.Duration // dispatched but not acknowledged -> nudge owner
	DoneTimeout   time.Duration // accepted but not finished -> nudge owner
	EscalateAfter time.Duration // since the first nudge, still unresolved -> escalate
	Cooldown      time.Duration // minimum gap between two nudges (anti-spam)
	MaxNudges     int           // nudges sent before holding for escalation
}

// DefaultChasePolicy is a conservative business-hours-friendly default.
var DefaultChasePolicy = ChasePolicy{
	AckTimeout:    4 * time.Hour,
	DoneTimeout:    24 * time.Hour,
	EscalateAfter: 24 * time.Hour,
	Cooldown:      2 * time.Hour,
	MaxNudges:     2,
}

var (
	ErrChaseExists   = errors.New("mail: chase already exists")
	ErrChaseNotFound = errors.New("mail: chase not found")
	ErrChaseConflict = errors.New("mail: chase was modified concurrently")
	// ErrActionInFlight is returned by a ChaseActioner when another replica
	// already owns the underlying idempotent send for this nudge/escalation.
	ErrActionInFlight = errors.New("mail: chase action already in flight on another worker")
)

type chaseAction int

const (
	chaseNone chaseAction = iota
	chaseNudge
	chaseEscalate
)

// decideTick is the pure SLA decision: given the chase, policy and current
// time, what (if anything) should happen on this scheduler tick. Escalation
// takes priority over nudging once its timer is due.
func decideTick(c Chase, p ChasePolicy, now time.Time) chaseAction {
	if c.Stage == StageDone || c.Stage == StageCancelled {
		return chaseNone
	}
	if !c.FirstNudgeAt.IsZero() && c.EscalatedAt.IsZero() &&
		now.Sub(c.FirstNudgeAt) >= p.EscalateAfter {
		return chaseEscalate
	}
	if c.Nudges >= p.MaxNudges {
		return chaseNone // stop nudging; wait for the escalation timer
	}
	var deadline time.Time
	switch c.Stage {
	case StageDispatched:
		deadline = c.CreatedAt.Add(p.AckTimeout)
	case StageAcknowledged, StageInProgress:
		base := c.LastActivityAt
		if base.IsZero() {
			base = c.CreatedAt
		}
		deadline = base.Add(p.DoneTimeout)
	default:
		return chaseNone
	}
	if now.Before(deadline) {
		return chaseNone
	}
	if !c.LastNudgeAt.IsZero() && now.Sub(c.LastNudgeAt) < p.Cooldown {
		return chaseNone
	}
	return chaseNudge
}

// applyObserve advances the stage from an inbound thread reply. Done and
// cancelled are terminal and never regress.
func applyObserve(c Chase, stage ChaseStage, now time.Time) Chase {
	if c.Stage == StageDone || c.Stage == StageCancelled {
		return c
	}
	c.LastActivityAt = now
	switch stage {
	case StageDone, StageCancelled:
		c.Stage = stage
	case StageInProgress:
		c.Stage = StageInProgress
	case StageAcknowledged:
		if c.Stage == StageDispatched {
			c.Stage = StageAcknowledged
		}
	}
	return c
}

// ChaseStore persists chases with optimistic concurrency.
type ChaseStore interface {
	Create(ctx context.Context, c Chase) (Chase, error)
	Get(ctx context.Context, tenant, key string) (Chase, error)
	ListOpen(ctx context.Context) ([]Chase, error)
	Update(ctx context.Context, c Chase, expectVersion int) (Chase, error)
}

// ChaseActioner sends the actual follow-up mails. Implementations should route
// through GuaranteedSender with deterministic per-nudge Message-IDs so a
// retried tick never double-sends.
type ChaseActioner interface {
	PerformNudge(ctx context.Context, c Chase, nudgeNumber int) (string, error)
	PerformEscalate(ctx context.Context, c Chase) (string, error)
}

// Tracker runs the chase lifecycle and SLA timers.
type Tracker struct {
	store    ChaseStore
	policy   ChasePolicy
	actioner ChaseActioner
	now      func() time.Time
}

// NewTracker builds a tracker, substituting defaults for zero policy fields.
func NewTracker(store ChaseStore, actioner ChaseActioner, policy ChasePolicy) *Tracker {
	if policy.AckTimeout == 0 {
		policy.AckTimeout = DefaultChasePolicy.AckTimeout
	}
	if policy.DoneTimeout == 0 {
		policy.DoneTimeout = DefaultChasePolicy.DoneTimeout
	}
	if policy.EscalateAfter == 0 {
		policy.EscalateAfter = DefaultChasePolicy.EscalateAfter
	}
	if policy.Cooldown == 0 {
		policy.Cooldown = DefaultChasePolicy.Cooldown
	}
	if policy.MaxNudges == 0 {
		policy.MaxNudges = DefaultChasePolicy.MaxNudges
	}
	t := &Tracker{store: store, policy: policy, actioner: actioner}
	t.now = time.Now
	return t
}

// WithClock overrides the clock (tests).
func (t *Tracker) WithClock(now func() time.Time) *Tracker { t.now = now; return t }

// OpenChase registers a newly dispatched request.
func (t *Tracker) OpenChase(ctx context.Context, tenant, key, owner, operator string) (Chase, error) {
	now := t.now()
	c := Chase{
		Tenant: tenant, Key: key, Owner: owner, Operator: operator,
		Stage: StageDispatched, CreatedAt: now,
	}
	return t.store.Create(ctx, c)
}

// Observe records an inbound reply that moves the chase forward.
func (t *Tracker) Observe(ctx context.Context, tenant, key string, stage ChaseStage) (Chase, error) {
	c, err := t.store.Get(ctx, tenant, key)
	if err != nil {
		return Chase{}, err
	}
	updated := applyObserve(c, stage, t.now())
	updated.Version = c.Version
	return t.store.Update(ctx, updated, c.Version)
}

// Cancel terminates a chase without completion.
func (t *Tracker) Cancel(ctx context.Context, tenant, key string) (Chase, error) {
	c, err := t.store.Get(ctx, tenant, key)
	if err != nil {
		return Chase{}, err
	}
	c.Stage = StageCancelled
	return t.store.Update(ctx, c, c.Version)
}

// Tick evaluates every open chase once and performs due nudges/escalations. A
// failed send does not advance the chase (it is retried on a later tick); a
// concurrent update loses the CAS race and is skipped, not repeated.
func (t *Tracker) Tick(ctx context.Context) (nudges, escalations int, err error) {
	open, listErr := t.store.ListOpen(ctx)
	if listErr != nil {
		return 0, 0, listErr
	}
	var errs []error
	for _, c := range open {
		switch decideTick(c, t.policy, t.now()) {
		case chaseNudge:
			next := c.Nudges + 1
			if _, sendErr := t.actioner.PerformNudge(ctx, c, next); sendErr != nil {
				if errors.Is(sendErr, ErrActionInFlight) {
					continue // another replica owns this nudge
				}
				errs = append(errs, sendErr)
				continue // do not advance; retry later
			}
			c.Nudges = next
			c.LastNudgeAt = t.now()
			if c.FirstNudgeAt.IsZero() {
				c.FirstNudgeAt = c.LastNudgeAt
			}
			if _, upErr := t.store.Update(ctx, c, c.Version); upErr != nil {
				if errors.Is(upErr, ErrChaseConflict) {
					continue // another replica handled it
				}
				errs = append(errs, upErr)
			} else {
				nudges++
			}
		case chaseEscalate:
			if _, sendErr := t.actioner.PerformEscalate(ctx, c); sendErr != nil {
				if errors.Is(sendErr, ErrActionInFlight) {
					continue // another replica owns the escalation
				}
				errs = append(errs, sendErr)
				continue
			}
			c.EscalatedAt = t.now()
			if _, upErr := t.store.Update(ctx, c, c.Version); upErr != nil {
				if errors.Is(upErr, ErrChaseConflict) {
					continue
				}
				errs = append(errs, upErr)
			} else {
				escalations++
			}
		}
	}
	return nudges, escalations, errors.Join(errs...)
}

// MemoryChaseStore is an in-process ChaseStore with CAS fencing.
type MemoryChaseStore struct {
	mu     sync.Mutex
	chases map[string]Chase
}

// NewMemoryChaseStore creates an empty in-memory chase store.
func NewMemoryChaseStore() *MemoryChaseStore {
	return &MemoryChaseStore{chases: map[string]Chase{}}
}

func chaseMapKey(tenant, key string) string { return tenant + "\x00" + key }

func (s *MemoryChaseStore) Create(_ context.Context, c Chase) (Chase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := chaseMapKey(c.Tenant, c.Key)
	if _, ok := s.chases[k]; ok {
		return Chase{}, ErrChaseExists
	}
	c.Version = 1
	s.chases[k] = c
	return c, nil
}

func (s *MemoryChaseStore) Get(_ context.Context, tenant, key string) (Chase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.chases[chaseMapKey(tenant, key)]
	if !ok {
		return Chase{}, ErrChaseNotFound
	}
	return c, nil
}

func (s *MemoryChaseStore) ListOpen(_ context.Context) ([]Chase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Chase
	for _, c := range s.chases {
		if c.Stage != StageDone && c.Stage != StageCancelled {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *MemoryChaseStore) Update(_ context.Context, c Chase, expectVersion int) (Chase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := chaseMapKey(c.Tenant, c.Key)
	cur, ok := s.chases[k]
	if !ok {
		return Chase{}, ErrChaseNotFound
	}
	if cur.Version != expectVersion {
		return Chase{}, ErrChaseConflict
	}
	c.Version = expectVersion + 1
	s.chases[k] = c
	return c, nil
}
