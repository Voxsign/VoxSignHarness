# SPEC v0.8 — Observability (O)

Observability makes a run inspectable after the fact: an event bus for
in-process fan-out, an append-only trajectory log, and a server-sent event
stream for live stage and decision-point updates.

## Contract

- The event bus never blocks the runner, fans out to every subscriber, and
  delivers without drops; replay is lossless and duplicate-free.
- The trajectory log appends parseable lines, creates its directory on
  open, rejects empty or unknown event kinds, and serialises concurrent
  writers.
- SSE emits a defined stage and event sequence. Reconnect is idempotent and
  supports replay-after; observer events are not throttled, while
  decision-point (need_ask) and interrupt events are delivered immediately.
- A request id is propagated end to end and written to the trajectory, so a
  single utterance can be traced from ingress to outcome.

## Pinned validators

- `TestBus_NeverBlocksRunner`
- `TestBus_PublishSubscribeFanout`
- `TestBus_ReliableNoDrop`
- `TestBus_ReplayNoDupLoss`
- `TestWriteAppendsParseableLines`
- `TestKindSingleSourceOfTruth`
- `TestSSEEventSequence`
- `TestSSEStageSequence`
- `TestSSEReconnectIdempotent`
- `TestSSEInterruptImmediacy`
- `TestRequestIDPassthroughAndTrajectory`
