# SPEC v0.8 — Execution Environment (E)

The execution environment is the layer that actually runs work: the HTTP
task service, the in-process task state machine, and the resilience
primitives that keep side effects from running away. It is deliberately
customer-neutral; customer behaviour lives in adapters, not here.

## Contract

- The task service exposes `POST /v1/tasks`, `GET /v1/tasks/{id}`,
  `POST /v1/tasks/{id}/answer` and `POST /v1/tasks/{id}/rollback`.
- A task moves through a single explicit state machine
  (need_ask / need_confirm / running / done / failed / withdrawn) and is
  persisted so a restart restores pending work instead of replaying it.
- Identical request bodies are deduplicated by request id; concurrent
  requests for different tasks must never cross their state.
- Outbound HTTP is wrapped by layered timeouts, a circuit breaker,
  monotonic retry/backoff, a bulkhead that fails fast when saturated, and
  `safeGo` panic isolation so one panicking task cannot take down the
  process.
- Authentication is mandatory; health and status endpoints report liveness
  without leaking task content.

## Pinned validators

The following automated validators pin this contract. Renaming or removing
one without updating this document fails the documentation-contract gate:

- `TestServerHealthAndAuth`
- `TestTasksLifecycleConfirm`
- `TestRequestIDDedup`
- `TestTaskPersistenceRestore`
- `TestASRSimPhoneConcurrentTasksDoNotCross`
- `TestBulkheadFullFailsFast`
- `TestCircuitBreakerOpensAndRecovers`
- `TestRetryBackoffMonotonic`
- `TestSafeGoRecoversTaskPanic`
