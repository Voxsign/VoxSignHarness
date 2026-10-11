package mail

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"voicesign-harness/action"
)

func testChasePolicy() ChasePolicy {
	return ChasePolicy{
		AckTimeout: time.Hour, DoneTimeout: 2 * time.Hour,
		EscalateAfter: 3 * time.Hour, Cooldown: 30 * time.Minute, MaxNudges: 2,
	}
}

func TestDecideTickSLA(t *testing.T) {
	p := testChasePolicy()
	t0 := time.Unix(1000000, 0)
	base := func(stage ChaseStage, mut func(*Chase)) Chase {
		c := Chase{Tenant: "t", Key: "k", Owner: "o@example.com", Operator: "op@example.com",
			Stage: stage, CreatedAt: t0, LastActivityAt: t0}
		if mut != nil {
			mut(&c)
		}
		return c
	}
	cases := []struct {
		name string
		c    Chase
		now  time.Time
		want chaseAction
	}{
		{"before ack deadline", base(StageDispatched, nil), t0.Add(59 * time.Minute), chaseNone},
		{"at ack deadline", base(StageDispatched, nil), t0.Add(time.Hour), chaseNudge},
		{"cooldown blocks repeat", base(StageDispatched, func(c *Chase) {
			c.Nudges, c.FirstNudgeAt, c.LastNudgeAt = 1, t0.Add(time.Hour), t0.Add(time.Hour)
		}), t0.Add(80 * time.Minute), chaseNone},
		{"second nudge after cooldown", base(StageDispatched, func(c *Chase) {
			c.Nudges, c.FirstNudgeAt, c.LastNudgeAt = 1, t0.Add(time.Hour), t0.Add(time.Hour)
		}), t0.Add(91 * time.Minute), chaseNudge},
		{"max nudges then hold", base(StageDispatched, func(c *Chase) {
			c.Nudges, c.FirstNudgeAt, c.LastNudgeAt = 2, t0.Add(time.Hour), t0.Add(90*time.Minute)
		}), t0.Add(2 * time.Hour), chaseNone},
		{"escalation due", base(StageDispatched, func(c *Chase) {
			c.Nudges, c.FirstNudgeAt, c.LastNudgeAt = 2, t0.Add(time.Hour), t0.Add(90*time.Minute)
		}), t0.Add(4 * time.Hour), chaseEscalate},
		{"already escalated", base(StageDispatched, func(c *Chase) {
			c.Nudges, c.FirstNudgeAt, c.EscalatedAt = 2, t0.Add(time.Hour), t0.Add(4*time.Hour)
		}), t0.Add(5 * time.Hour), chaseNone},
		{"done is silent", base(StageDone, nil), t0.Add(10 * time.Hour), chaseNone},
		{"cancelled is silent", base(StageCancelled, nil), t0.Add(10 * time.Hour), chaseNone},
		{"acknowledged before done deadline", base(StageAcknowledged, nil), t0.Add(119 * time.Minute), chaseNone},
		{"acknowledged past done deadline", base(StageAcknowledged, nil), t0.Add(121 * time.Minute), chaseNudge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decideTick(tc.c, p, tc.now); got != tc.want {
				t.Fatalf("decideTick=%d want %d", got, tc.want)
			}
		})
	}
}

type fakeChaseActioner struct {
	mu          sync.Mutex
	nudges      []int
	escalations int
	fail        bool
}

func (f *fakeChaseActioner) PerformNudge(_ context.Context, _ Chase, n int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return "", errors.New("smtp down")
	}
	f.nudges = append(f.nudges, n)
	return "srv", nil
}

func (f *fakeChaseActioner) PerformEscalate(_ context.Context, _ Chase) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return "", errors.New("smtp down")
	}
	f.escalations++
	return "srv", nil
}

func TestTrackerNudgeAndEscalateLifecycle(t *testing.T) {
	ctx := context.Background()
	t0 := time.Unix(2000000, 0)
	clock := t0
	store := NewMemoryChaseStore()
	act := &fakeChaseActioner{}
	tr := NewTracker(store, act, testChasePolicy()).WithClock(func() time.Time { return clock })
	if _, err := tr.OpenChase(ctx, "t", "thread-1", "owner@example.com", "op@example.com"); err != nil {
		t.Fatalf("open: %v", err)
	}
	var nN, nE int
	for i := 0; i < 6; i++ {
		clock = clock.Add(time.Hour)
		n, e, err := tr.Tick(ctx)
		if err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
		nN, nE = nN+n, nE+e
	}
	if nN != 2 || nE != 1 {
		t.Fatalf("want 2 nudges and 1 escalation, got %d/%d", nN, nE)
	}
	if !reflect.DeepEqual(act.nudges, []int{1, 2}) {
		t.Fatalf("nudge sequence: %v", act.nudges)
	}
	got, _ := store.Get(ctx, "t", "thread-1")
	if got.Nudges != 2 || got.EscalatedAt.IsZero() {
		t.Fatalf("final chase state wrong: %+v", got)
	}
}

func TestTrackerAcknowledgementResetsDeadline(t *testing.T) {
	ctx := context.Background()
	t0 := time.Unix(2100000, 0)
	clock := t0
	store := NewMemoryChaseStore()
	tr := NewTracker(store, &fakeChaseActioner{}, testChasePolicy()).WithClock(func() time.Time { return clock })
	_, _ = tr.OpenChase(ctx, "t", "k", "owner@example.com", "op@example.com")
	clock = t0.Add(30 * time.Minute)
	if _, err := tr.Observe(ctx, "t", "k", StageAcknowledged); err != nil {
		t.Fatalf("observe: %v", err)
	}
	clock = t0.Add(time.Hour) // past ack timeout, but acknowledged -> done timeout not due
	if n, _, _ := tr.Tick(ctx); n != 0 {
		t.Fatalf("acknowledged chase must not nudge before done timeout, nudges=%d", n)
	}
	clock = t0.Add(2*time.Hour + 31*time.Minute)
	if n, _, _ := tr.Tick(ctx); n != 1 {
		t.Fatalf("expected one nudge after done timeout, got %d", n)
	}
}

func TestTrackerDoneAndCancelAreTerminal(t *testing.T) {
	ctx := context.Background()
	t0 := time.Unix(2200000, 0)
	clock := t0
	store := NewMemoryChaseStore()
	tr := NewTracker(store, &fakeChaseActioner{}, testChasePolicy()).WithClock(func() time.Time { return clock })
	_, _ = tr.OpenChase(ctx, "t", "done-k", "owner@example.com", "op@example.com")
	clock = t0.Add(5 * time.Minute)
	if _, err := tr.Observe(ctx, "t", "done-k", StageDone); err != nil {
		t.Fatalf("observe done: %v", err)
	}
	clock = t0.Add(10 * time.Hour)
	if n, e, _ := tr.Tick(ctx); n != 0 || e != 0 {
		t.Fatalf("done chase must stay silent, got %d/%d", n, e)
	}

	clock = t0
	tr2 := NewTracker(store, &fakeChaseActioner{}, testChasePolicy()).WithClock(func() time.Time { return clock })
	_, _ = tr2.OpenChase(ctx, "t", "cancel-k", "owner@example.com", "op@example.com")
	if _, err := tr2.Cancel(ctx, "t", "cancel-k"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	clock = t0.Add(10 * time.Hour)
	if n, e, _ := tr2.Tick(ctx); n != 0 || e != 0 {
		t.Fatalf("cancelled chase must stay silent, got %d/%d", n, e)
	}
}

func TestTrackerFailedSendRetriesWithoutAdvancing(t *testing.T) {
	ctx := context.Background()
	t0 := time.Unix(2300000, 0)
	clock := t0
	store := NewMemoryChaseStore()
	act := &fakeChaseActioner{fail: true}
	tr := NewTracker(store, act, testChasePolicy()).WithClock(func() time.Time { return clock })
	_, _ = tr.OpenChase(ctx, "t", "k", "owner@example.com", "op@example.com")
	clock = t0.Add(time.Hour)
	if n, _, err := tr.Tick(ctx); n != 0 || err == nil {
		t.Fatalf("failed nudge must report error and not count, n=%d err=%v", n, err)
	}
	got, _ := store.Get(ctx, "t", "k")
	if got.Nudges != 0 || !got.FirstNudgeAt.IsZero() {
		t.Fatalf("failed send must not advance the chase, got %+v", got)
	}
	act.fail = false
	if n, _, err := tr.Tick(ctx); n != 1 || err != nil {
		t.Fatalf("retry after recovery must nudge once, n=%d err=%v", n, err)
	}
}

// gsNudgeActioner routes chase follow-ups through a GuaranteedSender so the
// idempotent ledger (not the tracker) enforces exactly-once delivery.
type gsNudgeActioner struct {
	gs     *GuaranteedSender
	tenant string
}

func (a *gsNudgeActioner) followup(ctx context.Context, to, id, subject string) (string, error) {
	msg := &OutboundMessage{
		From: "martin@voxsign.ai", To: []string{to}, Subject: subject, TextBody: "follow-up",
		MessageID: id, InReplyTo: "thread-9",
	}
	res, err := a.gs.Send(ctx, a.tenant, msg, SendIntent{})
	if err != nil {
		return "", err
	}
	if res.InProgress || res.NeedsReconcile {
		return "", ErrActionInFlight
	}
	return res.ExternalRef, nil
}

func (a *gsNudgeActioner) PerformNudge(ctx context.Context, c Chase, n int) (string, error) {
	return a.followup(ctx, c.Owner, fmt.Sprintf("%s.nudge%d", c.Key, n), fmt.Sprintf("nudge %d", n))
}

func (a *gsNudgeActioner) PerformEscalate(ctx context.Context, c Chase) (string, error) {
	return a.followup(ctx, c.Operator, c.Key+".escalate", "escalation")
}

// 20 scheduler replicas tick concurrently at the first nudge deadline; the
// guaranteed-send ledger must collapse them to exactly one nudge mail.
func TestTrackerNudgeOnceAcrossReplicas(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	mb := NewFileMailbox(root)
	actionStore, err := action.OpenFileStore(filepath.Join(root, "actions.jsonl"))
	if err != nil {
		t.Fatalf("action store: %v", err)
	}
	chaseStore := NewMemoryChaseStore()
	t0 := time.Unix(3000000, 0)
	clock := t0
	mk := func(worker string) *Tracker {
		gs := NewGuaranteedSender(mb.FileSentBox, actionStore, worker, testGuard(), NewFileSentProbe(mb.FileSentBox))
		return NewTracker(chaseStore, &gsNudgeActioner{gs: gs, tenant: "t"}, testChasePolicy()).
			WithClock(func() time.Time { return clock })
	}
	if _, err := mk("replica-0").OpenChase(ctx, "t", "thread-9", "owner@example.com", "op@example.com"); err != nil {
		t.Fatalf("open: %v", err)
	}
	clock = t0.Add(time.Hour) // first nudge due

	const n = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		tr := mk(fmt.Sprintf("replica-%d", i))
		go func() {
			defer wg.Done()
			<-start
			_, _, _ = tr.Tick(ctx)
		}()
	}
	close(start)
	wg.Wait()

	entries, _ := os.ReadDir(filepath.Join(root, "sent"))
	if len(entries) != 1 {
		t.Fatalf("exactly one nudge mail must exist across replicas, got %d", len(entries))
	}
	if !mb.Contains("thread-9.nudge1", "owner@example.com") {
		t.Fatalf("the single nudge must target the owner")
	}
	got, _ := chaseStore.Get(ctx, "t", "thread-9")
	if got.Nudges != 1 {
		t.Fatalf("chase must record exactly one nudge, got %d", got.Nudges)
	}
}
