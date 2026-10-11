# SPEC v0.8 — Lifecycle Orchestration (L)

Lifecycle orchestration turns an intent into a bounded, recoverable run:
planning, the pipeline order, goal tracking across turns, L2 model-assisted
planning with graceful degradation, and self-healing that never replays an
irreversible action.

## Contract

- The pipeline runs its stages in a fixed order and its confirmation
  strategy and verification hook are observable in the receipt.
- A goal completes only on evidence, not on the model's claim; stale goals
  are nudged and recently resumable goals are continued rather than restarted.
- L2 planning is configurable and may differ from the rule-based planner,
  but when the model fails it degrades to the rule path and marks the
  degradation; it never introduces an external capability that is not
  registered.
- Plans use only real, reachable capabilities; gaps are refused with an
  owner, partial plans execute only their reachable prefix, and replans are
  bounded and explain their changes.
- Self-heal classifies read-only versus mutating fixes: read-only fixes may
  be replayed under filtered parameters, irreversible fixes are never
  replayed, and recovery is bounded to two rounds before asking.

## Pinned validators

- `TestPipelineOrdering`
- `TestPipelineConfirmStrategy`
- `TestGoalLifecycle`
- `TestGoalCompletedOnlyOnEvidence`
- `TestGoalStaleNudge`
- `TestRecentResumableGoal`
- `TestReviveTriggers`
- `TestL2FailureDegradesAndMarks`
- `TestL2ModelIsConfigurable`
- `TestPlanCapabilityGapRefusedWithOwner`
- `TestRV4BoundedReplans`
- `TestSafeRetryNeverReplaysIrreversible`
- `TestSafeRetryTwoRoundLimit`
