# VoxSign Harness — Phase 0 Baseline Report (Issue #30, corrected base)

**⚠️ Correction (2026-10-17)**: earlier reports measured the wrong base.
`dd697db` (with its "26 fake done" finding) is the private-repo `ui-v3-doubao`
fork line (2026-10-05), NOT public main. Public main = `7cc735f` (v0.8.1,
2026-10-11) — it HAS a real `failed` event (server.go:1264) and its honest
baseline is reported below. Numbers are the **honest** N1–N5 on v0.8.1,
3 runs, identical.

- **Set**: T-v0.8.1 (40 tasks, 4 classes × 10; `bench/tasks.json`)
- **Base**: `7cc735f` = public `main` (v0.8.1), verified via `gh api` + local fetch
- **Runner**: `bench/runner_test.go` — real HTTP/SSE surface, real model center,
  additive `timing` payload; honest judgement (failed receipts inside done events
  are NOT first-pass and count as dishonest done; clarify-in-done counts for N4).
- **Runs**: 3 consecutive runs, 2026-10-17, identical across runs.

## 1. N1–N5 (3 runs, v0.8.1, honest judgement)

| Metric | Run 1 | Run 2 | Run 3 | Issue target |
|---|---|---|---|---|
| N1 first-event p50 / p90 (ms) | 2 / 835 | 1 / 696 | 1 / 869 | — |
| N1b first terminal/ask p50 / p90 (ms) | 753 / 3144 | 747 / 3031 | 718 / 3365 | ≤1500 / ≤3000 |
| N2 Q&A done p50 / p90 (ms) | 2829 / 3327 | 2604 / 3031 | 2383 / 3365 | ≤3000 |
| N2 simple p50 / p90 (ms) | 778 / 2496 | 795 / 2630 | 790 / 2457 | ≤10000 |
| N2 complex p50 / p90 (ms) | 23 / 2881 | 24 / 2910 | 25 / 2710 | Phase 1 |
| N2 ambiguous p50 / p90 (ms) | 8 / 148 | 8 / 123 | 10 / 149 | — |
| N3 first-pass | **23/40 (58%)** | 23/40 (58%) | 23/40 (58%) | ≥90% |
| N4 clarify (non-ambiguous) | **1/30 (3%)** | 1/30 (3%) | 1/30 (3%) | ≤5% |
| N5 dishonest done | **0** | 0 | 0 | 0 |

Done events: 29/40 (11 non-done are real `failed` / `ask` / `canceled` events —
honesty surface intact on v0.8.1).

## 2. Gap to targets (v0.8.1)

| Metric | Baseline | Target | Gap |
|---|---|---|---|
| N1b P50 / P90 | 739 / 3180 ms | ≤1500 / ≤3000 | P90 ~+180 ms over |
| N2 Q&A P50 / P90 | 2605 / 3341 ms | ≤3000 | P90 ~+340 ms over — **main latency lever** |
| N2 simple P90 | 2528 ms | ≤10000 | ✅ met |
| N3 first-pass | 58% (23/40) | ≥90% (+10pp) | **−32 pp — critical** |
| N4 clarify | 3% (1/30) | ≤5% | ✅ met |
| N5 dishonest done | 0 | 0 | ✅ met |

**Corrected verdict: honesty is OK on v0.8.1 (N4 3%, N5 0). The real gaps are
latency (Q&A P90 ~3.34 s vs ≤3 s, driven by ~19 fast-tier LLM calls at ~1.2 s
each) and first-pass success (58% vs ≥90%).** This re-ranks Phase 1: lever (a)
fast-tier routing / fewer LLM calls is the top cost item; N3 improvement via
fewer clarification-worthy classify errors; levers d/e (error_class,
finish_reason=length→failed) remain mandatory-but-defensive hardening.

## 3. Where the time goes (timing payload — 1 run instrumented, N metrics 3-run identical)

> NOTE (2026-10-17 fix): the initial commits defined the timing collector but the
> server `done` events never carried `out.Timing`, so the runner saw `timing:
> null`. Fixed (additive `"timing"` field in both `done` emitters, server.go).
> One instrumented run below; N1–N5 unchanged by the fix (identical to the
> 3-run honest baseline).

| LLM provider | sum ms | count | avg ms | role |
|---|---|---|---|---|
| fast | 22 959 | 18 | 1 275 | classify LLM fallback + intent patch (llmIntentFallback) |
| aiops-fallback | 19 155 | 19 | 1 008 | QUERY natural reply (llmNaturalReply: strata→fallback chain) |

| Stage | sum ms | n | note |
|---|---|---|---|
| exec | 19 169 | 22 | action exec + QUERY answers |
| classify | 3 894 | 28 | rule classify + LLM patch |

**Latency spine on v0.8.1** = ~19× llmNaturalReply (~1.0 s each, after a
strata attempt that fails fast without STRATA_API_KEY) + ~18× llmIntentFallback
(~1.3 s each). Phase 1 lever (a) targets this: skip the strata empty attempt
when no local key, and route QUERY answers through the same fast tier the
fallback already uses — fewer wasted calls and one consistent fast path.

## 4. Phase 1 lever (a) — first cut (v0.9.0-alpha, 1 run vs 1 run)

Change: `llmNaturalReply` now skips the strata attempt entirely when
`STRATA_API_KEY` is unset (was: every natural reply first called the LAN
strata host, which can only fail without a key).

| metric | before | after |
|---|---|---|
| N1b first-terminal/ask p50 | 831 | 689 (-17%) |
| N1b p90 | 3 335 | 3 129 (-6%) |
| N2 qa p50 | 2 655 | 2 505 (-5.6%) |
| aiops-fallback avg | 1 008 | 950 (-5.7%) |
| N3 first-pass | 58% | 58% (unchanged) |
| N4 clarify | 3% | 3% (unchanged) |
| N5 dishonest done | 0 | 0 (unchanged) |

Call count unchanged (18 fast + 19 fallback); this cut removes the wasted
strata leg, it does not yet reduce the number of calls. Next lever: fewer /
faster calls (streaming + fast tier for QUERY) — that is the S1 (Doubao
reference) track, Phase 1b.

## 5. Phase 1 second cut — S2 error_class + letter e (v0.9.0-alpha)

- **S2 / Issue #30 letter d**: SSE `failed` event now carries `error_class`
  ∈ {model_error, execution_error, network_error} (server.deriveErrorClass,
  additive, 5 unit cases). Blocked goals keep `blocked=true` and classify as
  execution_error.
- **letter e**: aiopsChatFallback refuses `finish_reason=length` replies —
  a truncated answer is never surfaced as complete; hard fallback takes over.
- 1-run re-baseline after both: N3 58% / N4 3% / N5 0 unchanged (no
  regression); per-run latency varies with cross-region gateway noise
  (single-run N2 qa p50 2.7s; 3-run stable values in §2 remain the
  authoritative baseline).

Phase 1 state: lever (a) landed + d/e landed. Remaining for Phase 1:
S5 env profiles; then Phase 1b = S1 streaming probe (model-center stream
support) + fewer-call QUERY routing.

## 4. Files
- `bench/tasks.json` — frozen task set T (40 tasks, 4 classes, M3/M7/M8 tags)
- `bench/runner_test.go` — reproducible honest runner (skips without `AIOPS_KEY`)
- `pipeline/timing.go`, `pipeline/pipeline.go`, `provider/provider.go`,
  `trajectory/kinds.go`, `server/server.go` — additive stage/LLM timing (PR)
- `bench/FORK-INVENTORY.md` — fork-line B inventory (Phase 0′, 129 commits)
