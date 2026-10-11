// Store-independent action state machine. These pure functions encode the
// acquire / mark / reopen rules once so that every Store implementation
// (in-memory, JSONL, SQL) behaves identically and can share one conformance
// test suite. They perform no locking and no I/O.

package action

import "time"

// acquireLogic computes the result of an Acquire attempt.
//
//	cur == nil           -> create PENDING, owned by caller (Acquired)
//	SUCCEEDED            -> AlreadySucceeded (never re-run)
//	FAILED               -> atomic retry take-over (Acquired); a confirmed
//	                        failure means the effect did not happen, so retry is safe
//	UNKNOWN, live lease  -> InFlight
//	UNKNOWN, expired     -> NeedsReconcile (effect unknown; never blind replay)
//	PENDING, live lease  -> InFlight
//	PENDING, no/self own -> Acquired (clean retry after Reopen, or self-renew)
//	PENDING, other+expired-> NeedsReconcile (possible crash-after-side-effect)
func acquireLogic(cur *Record, sel Selector, owner string, lease time.Duration, now time.Time) (Record, AcquireDecision) {
	if cur == nil {
		r := Record{
			Tenant: sel.Tenant, Key: sel.Key, Kind: sel.Kind, Status: StatusPending,
			Owner: owner, LeaseUntil: now.Add(lease), Version: 1,
			CreatedAt: now, UpdatedAt: now,
		}
		return r, DecisionAcquired
	}
	r := *cur
	liveOther := r.Owner != "" && r.Owner != owner && r.LeaseUntil.After(now)
	// liveLease is true whenever any execution flow (including this same owner)
	// holds an unexpired lease; a second Acquire in that window is a duplicate.
	liveLease := r.Owner != "" && r.LeaseUntil.After(now)

	switch r.Status {
	case StatusSucceeded:
		return r, DecisionAlreadySucceeded
	case StatusFailed:
		// Confirmed not-done: safe to atomically claim a retry.
		r.takeOver(owner, lease, now)
		return r, DecisionAcquired
	case StatusUnknown:
		if liveOther {
			return r, DecisionInFlight
		}
		return r, DecisionNeedsReconcile
	default: // PENDING
		if liveLease {
			// A live lease (any owner, including this same owner) means the
			// action is already executing; a duplicate Acquire must not re-run it.
			return r, DecisionInFlight
		}
		if r.Owner == "" || r.Owner == owner {
			r.takeOver(owner, lease, now)
			return r, DecisionAcquired
		}
		// Owned by a different worker whose lease has expired. That worker may
		// have crashed after performing the side effect but before recording it;
		// require external reconciliation instead of replaying.
		return r, DecisionNeedsReconcile
	}
}

// markLogic applies a mutation under an optimistic version (fencing) check.
func markLogic(cur Record, expectVersion int64, mutate func(*Record), now time.Time) (Record, error) {
	if cur.Version != expectVersion {
		return cur, ErrFence
	}
	r := cur
	mutate(&r)
	r.Version++
	r.UpdatedAt = now
	return r, nil
}

// reopenLogic is the only path that returns an inconclusive record to the
// runnable PENDING state; it requires a fresh fencing version.
func reopenLogic(cur Record, expectVersion int64, now time.Time) (Record, error) {
	if cur.Version != expectVersion {
		return cur, ErrFence
	}
	switch cur.Status {
	case StatusUnknown, StatusFailed:
		// always reopenable once external not-done evidence exists
	case StatusPending:
		// An orphan PENDING (owner lease expired, possible crash) may be reopened
		// after reconciliation; a PENDING with a live owner lease is protected.
		if cur.Owner != "" && cur.LeaseUntil.After(now) {
			return cur, ErrNotReopenable
		}
	default:
		return cur, ErrNotReopenable
	}
	r := cur
	r.Status = StatusPending
	r.Owner = ""
	r.LeaseUntil = time.Time{}
	r.Version++
	r.UpdatedAt = now
	return r, nil
}
