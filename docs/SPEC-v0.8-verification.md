# SPEC v0.8 — Verification (V)

Verification decides whether a claimed outcome actually happened. It never
trusts an executor's self-report, and it treats external side effects
(email sends) as idempotent actions whose completion requires evidence.

## Contract

- The verifier runs commands and reads files/grep itself; an executor claim
  that cannot be independently reproduced is reported unverifiable, and
  path/symlink escapes are rejected.
- External actions go through an idempotent ledger keyed by
  (tenant, key, kind). Concurrent executors for one action run the side
  effect exactly once; an already-succeeded action is skipped idempotently.
- An action that is still in flight is never reported as success and is not
  executed a second time. A missing provider receipt leaves the action
  UNKNOWN; the system probes the outside world before any replay and never
  blindly re-sends.
- Persistence failure after a confirmed send yields UNKNOWN, never a false
  SUCCEEDED. Stale owners are fenced by lease/version so they cannot
  overwrite a newer owner.
- The mail guarantee layer composes ledger + guard + sender + sent-box
  probe. The chase tracker sends each nudge/escalation exactly once across
  replicas by using a deterministic Message-ID as the idempotency key.

## Pinned validators

- `TestRun_DoesNotTrustExecutorClaim`
- `TestRun_Unverifiable`
- `TestRun_PathEscapingRejected`
- `TestConcurrentSameActionExecutesOnce`
- `TestSameWorkerConcurrentExecutesOnce`
- `TestInFlightNotReportedSuccess`
- `TestSucceededIsIdempotentSkip`
- `TestLedgerMarkFailureAfterConfirmedOKIsUnknown`
- `TestFencingStaleOwnerCannotOverwrite`
- `TestGuaranteedConcurrentOnce`
- `TestGuaranteedNoReceiptIsUnknownThenProbeConfirms`
- `TestGuaranteedFileCrashReconcileLoop`
- `TestTrackerNudgeOnceAcrossReplicas`
- `TestTrackerFailedSendRetriesWithoutAdvancing`
