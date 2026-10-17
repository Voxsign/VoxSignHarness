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

## 3. Where the time goes (timing payload, same 3 runs)
- exec + classify dominate; Q&A tasks each spend ~1.2 s × 1 fast call at
  `aiops.voxsign.ai` (deepseek-flash). Full per-stage table available in the
  raw log (`/tmp/baseline_v081_honest.log`).

## 4. Files
- `bench/tasks.json` — frozen task set T (40 tasks, 4 classes, M3/M7/M8 tags)
- `bench/runner_test.go` — reproducible honest runner (skips without `AIOPS_KEY`)
- `pipeline/timing.go`, `pipeline/pipeline.go`, `provider/provider.go`,
  `trajectory/kinds.go`, `server/server.go` — additive stage/LLM timing (PR)
- `bench/FORK-INVENTORY.md` — fork-line B inventory (Phase 0′, 129 commits)
