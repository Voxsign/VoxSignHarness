# VoxSign Harness Architecture (v1, post-fix 2026-10-05)

> This document is rewritten against the **real, post-fix code** (replacing the V0
> review draft). Where it disagrees with the code, the code wins. Known
> target-state vs current-state gaps are called out in "§11 Execution model".
> The repository has zero external Go dependencies (pure standard library).

---

## 1. Layer overview

```
+--------------------------------------------------------------+
| Client: iOS VoiceSign (voice/text entry, four-line receipt) |   ios/ (private repo)
+----------------------------+---------------------------------+
                             | HTTP (Line-A entry)
+----------------------------v---------------------------------+
| Line-A entry server (server/): HTTP routing, task table     |
| (byReq dedup), serial gate, confirmation bridge, SSE,      |
| device registration, cloud-tenant isolation                  |
+----------------------------+---------------------------------+
                             | pipeline.Run(ctx, o, text)
+----------------------------v---------------------------------+
| Interaction core pipeline (pipeline/): 13-stage loop (§3)   |
+------+----------------+--------------+--------------+--------+
       |                |              |              |
+------v------+  +------v------+  +---v--------+  +------v------+
| tools       |  | provider    |  | memory/   |  | space       |
| exec/search |  | multi-model  |  | dict      |  | domains/    |
|             |  | routing      |  | context   |  | gates       |
+-------------+  +-------------+  +-----------+  +-------------+
   plus refer (coreference), risk (grading), verify (independent),
   search, selfheal (three rings), plan, route/router (intent routing),
   ground (cognition slices), hotcache, zhiji (in-flight)

+--------------------------------------------------------------+
| Line-B standalone ASR personalization service: cmd/vhs-asr   |
| (default listen 127.0.0.1:8787)                              |
|   exposes /v1/process /v1/dictionary /v1/feedback /v1/lexicon|
|   cloud model-center (modelcenter/): ASR + chat channels     |
+--------------------------------------------------------------+
```

**Line-A / Line-B boundary (architecture contract, archstub D2/D3):**
server (Line-A) calls Line-B **only over HTTP**; the `asr` package must not
import the core (pipeline/plan/space/tools) back.

---

## 2. Directory layout (against the actual tree)

| Path | Role |
|---|---|
| `main.go` / `machine.go` | CLI entry (serve/repl/task); machine.go is the in-flight device config |
| `cmd/vhs-asr` | **Line-B** ASR personalization service binary (default `127.0.0.1:8787`) |
| `cmd/vhs-voice` | Voice adaptation layer (:8950 -> upstream main harness :8941), not Line-B |
| `cmd/vhs-ledger` | Standalone ledger binary |
| `cmd/vhs-relay` | Long-running reverse relay agent back to the local harness |
| `server/` | **Line-A** HTTP entry, task table, serial gate, confirmation bridge, SSE, cloud tenant, robustness (robust.go) |
| `pipeline/` | The 13-stage orchestration core (Run/Options/outcome) |
| `trajectory/` | Append-only JSONL trace (single enum of kinds + Validate) |
| `input/` | Input cleaning / 5-stage correction pipeline, personal dictionary hookup |
| `refer/` | Coreference resolution ("this/that" -> concrete object) |
| `space/` | Space/domain registry and gate (space.Check) |
| `risk/` | Risk grading + irreversible list + Guard fatigue downgrade |
| `verify/` | Independent verifier (reads reality, never self-reports) |
| `tools/` | Tool registry + Executor (file/search/run/note...) |
| `provider/` | Multi-model provider interface and Registry |
| `router/` `route/` | Model routing table / intent routing |
| `memory/` | Personal dictionary / memory persistence |
| `hotcache/` `cache/` | L2 hot-word cache / quad confirmation cache |
| `modelcenter/` | Cloud model-center channels (ASR + chat) |
| `selfheal/` | Three self-healing rings (knowledge base -> diagnose model -> read-only safe replay) |
| `plan/` | Task planning and verification-plan generation |
| `ground/` | Ground cognition slices (project map / clarification context) |
| `contract/` `contracts/` | Intent/action/receipt contracts; `contracts/intent-v1.schema.json` is the intent authority |
| `doccontract/` | Tool-contract bootstrap loading |
| `zhiji/` | In-flight self-model architecture (vector index / STM / edge rules), evolved independently |
| `data/spaces/` | Sample space (domain) manifest definitions |

---

## 3. The 13 pipeline stages (Run control flow)

`pipeline.Run(ctx, o, text)`, inside the serial gate (`o.mu`), in order:

1. `input_raw` trace written (before any processing)
2. clean (input.Cleaner)
3. correct (personal-dictionary correction)
4. intent (intent classification)
5. refer (coreference resolution, writes `refer` trace)
6. space_check (domain/gate, writes `space_check` trace; stop on reject)
7. risk (grading, writes `risk` trace)
8. confirm (dispatch by level: auto/light/strong/human, writes `confirm` trace; human never enters the quad cache)
9. exec (tools execution, writes `receipts` trace)
9-bis. selfheal (on a failed receipt -> diagnose + read-only safe replay, max 2 rounds)
10. verify (independent check, writes `verify` trace; fail -> diagnose layer)
11. attribution (writes back `attribution` trace)
12. quad cache (Set only when not an irreversible approval)
13. final (four-line receipt, writes `final` + `task_metrics`)

---

## 4. Intent classification (single authority)

**`contracts/intent-v1.schema.json` is the single authoritative schema for the
intent JSON.** The deterministic classifier in pipeline `input` produces a
`contract.Intent` that must satisfy this schema. Any intent enum/field follows
this schema; no separate copy lives in this document or in code comments.

---

## 5. Observability (trajectory, append-only JSONL)

The trace is a replayable evidence chain "raw input -> understanding -> action ->
result", written one entry at a time at mode 0600.

**Single enum of kinds** (`trajectory.Kinds`):

- Base events: `input_raw / input_clean / input_correct / intent / intent_source /
  start / model / actions / receipts / final / error / task_metrics`
- The 13-stage intermediate decision events:
  `refer / space_check / risk / confirm / verify / attribution`

**Write contract**: an unregistered kind is no longer silently swallowed by `_ =`;
`pipeline.(*Options).write` logs a `[trajectory] write dropped` warning on a
`Trace.Write` error (still non-blocking for read-only tasks).

**request_id propagation**: entry (`/v1/tasks` body `request_id` / HTTP header
`X-Request-Id`) -> `Options.RequestID` -> pipeline trace Entry `request_id` ->
selfheal diagnose trace `RequestID`. Three log classes carry the rid: server
access logs, ASR platform/calibration logs, and selfheal diagnose logs.

---

## 6. Security (red lines)

- Serial gate: only one Run in flight per process (protects uncommitted user
  changes; never overwrites others' files).
- Irreversible actions (commit/deploy/human) always require human confirmation
  and are never "learned away" by the quad cache.
- space.Check rejects -> never executes; verify reads reality and never
  self-reports; attribution must come from a real error.
- Loopback auth: when the ASR service token is empty, only loopback addresses
  are allowed.

---

## 7. Performance baseline

Cold-start-to-first-response timing is measured externally. The task-layer hard
limit is `taskMaxDeadline = 10min` (an external call can never hang the process).

---

## 8. Verification methodology

1. **Unit criteria (red-first)**: each package ships `*_criteria_test.go` that
   mechanically extracts symbols with go/parser and reconciles them.
2. **Executable regression gate**: `CGO_ENABLED=0 go test ./...`
   (the repo is pure cgo-free; CGO_ENABLED=0 keeps the test binaries portable).
3. **Architecture contract (archstub D1-D7)**: Line-A/Line-B boundary, no ASR
   main closure, input validation, and other structural invariants.

---

## 9. Robustness (robust.go)

Four robustness primitives for external calls: tiered timeouts (fast/mid/long),
circuit breaker, exponential-backoff retry (idempotent only), and bounded
bulkheads. `safeGo`: every business background goroutine is recover-guarded — a
single-task panic is isolated (log + stack, write an error trace, mark the task
canceled) and the process keeps running.

---

## 10. Config and ports

- ASR Line-B default endpoint `http://127.0.0.1:8787` (override via
  `VHS_ASR_ENDPOINT`). Line-B is `cmd/vhs-asr` (listens on 8787, exposes
  `/v1/process`).
- vhs-voice (:8950) is a voice adaptation layer, not Line-B; the main harness
  defaults to :8941.

---

## 11. Execution model (target vs current state)

**Target (settled principles)**: one Runner per virtual user, a central
scheduler (dispatches by priority / serial gate), trace id (request_id) flowing
from entry through logs and traces, multi-tenant isolation.

**Current state (as-is)**:
- Temporary goroutines + serial gate: one `safeGo` goroutine per task, a global
  `o.mu` serial gate + server task table. Multi-tenant Runners / central
  scheduler are reserved-but-unimplemented.
- trace id propagation: `Options.RequestID` is landed; the main dedup path is
  end-to-end consistent.
- Multi-tenancy: in cloud mode, hot-word libraries/data are isolated by sub
  (`server.cloud`); Runner-level multi-tenancy is reserved.

> This section only describes the target and the gap; it does not change code.

---

## 12. Relationship to the client repositories

This repository (voicesign-harness) is VoiceSign's **orchestration /
observability / verification harness**: pipeline orchestration, trace
observability, the space/risk/verify safety gates, the Line-B ASR personalization
service, and the e2e contract. The client side lives in the companion
[VoxSign-IOS](https://github.com/Voxsign/VoxSign-IOS) and the private
VoxSign-Server backend. The harness aligns on the intent contract via
`contracts/intent-v1.schema.json`, receives commands over HTTP (Line-A) and
returns results as a four-line receipt. The two sides do not import each other;
they are decoupled by contract + HTTP.
