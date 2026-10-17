# VoxSign Harness — Phase 0 Baseline Report (Issue #30)

- **Set**: T-v0.8.1 (40 tasks, 4 classes × 10; `bench/tasks.json`)
- **Base**: `7cc735f` (release v0.8.1, P0 reliability)
- **Runner**: `bench/runner_test.go` — real HTTP/SSE surface (POST `/v1/tasks` → GET `/v1/tasks/{id}/events`), real model center (`aiops.voxsign.ai`, `AIOPS_KEY`), per-task wall-clock + additive `timing` payload from the instrumentation PR.
- **Runs**: 3 consecutive runs, fresh server + temp logDir each. Total wall ~6.8 min/run.
- **Date**: 2026-10-17 (UTC+3)

## 1. N1–N5 (3 runs)

| Metric | Run 1 | Run 2 | Run 3 | Issue target |
|---|---|---|---|---|
| N1 first-event p50 / p90 (ms) | 1 / 920 | 1 / 899 | 1 / 818 | — |
| N1b first terminal/ask p50 / p90 (ms) | 772 / 3135 | 835 / 2995 | 906 / 3220 | ≤1500 / ≤3000 |
| N2 Q&A done p50 / p90 (ms) | 2412 / 3367 | 2291 / 3064 | 2373 / 3603 | ≤3000 / ≤3000 |
| N2 simple (NOTE/EDIT/QUERY) p50 / p90 (ms) | 803 / 2585 | 840 / 2766 | 1072 / 2755 | ≤10000 |
| N2 complex p50 / p90 (ms) | 25 / 3135 | 24 / 2544 | 23 / 2264 | baseline×0.5 (Phase 1) |
| N2 ambiguous p50 / p90 (ms) | 11 / 236 | 9 / 139 | 9 / 144 | — |
| N3 first-pass success | 23/40 (58%) | 23/40 (58%) | 23/40 (58%) | ≥baseline+10pp, min 90% |
| N4 clarify rate (non-ambiguous) | 1/30 (3%) | 1/30 (3%) | 1/30 (3%) | ≤5% |
| N5 empty/false done receipts | 0 | 0 | 0 | 0 |

Notes:
- N1 first event is always the immediate `stage` SSE event (1 ms); the meaningful bar is N1b (first terminal/ask). N1b P50 (0.77–0.91 s) meets the ≤1.5 s goal; P90 (3.0–3.2 s) slightly overshoots 3.0 s.
- N2 Q&A P90 (3.1–3.6 s) slightly overshoots the 3 s goal; P50 (2.3–2.4 s) is inside it. Simple-action N2 P90 ≤2.8 s ≪ 10 s goal.
- N3 is exactly 23/40 across all 3 runs — deterministic, not noise. The 17 non-first-pass cases: 11 done-but-flagged, 6 failed/timeout (details §4).
- N4 and N5 already meet their targets at baseline.

## 2. Where the time goes (timing payload, runs 1–3)

### Top-cost stages (sum ms / tasks with timing)

| Stage | Run 1 | Run 2 | Run 3 | Interpretation |
|---|---|---|---|---|
| exec | 18 861 / 32 | 17 043 / 32 | 20 215 / 32 | includes QUERY LLM answer + real actions |
| classify | 6 715 / 34 | 6 382 / 34 | 5 946 / 34 | intent classifier + LLM intent fallback |
| all others | 0 | 0 | 0 | local stages are sub-ms |

### LLM calls by provider (sum ms / count / avg ms)

| Provider | Run 1 | Run 2 | Run 3 | Note |
|---|---|---|---|---|
| fast (QUERY answer) | 24 266 / 19 / 1 277 | 22 601 / 19 / 1 189 | 25 340 / 20 / 1 267 | model center `deepseek-flash` |
| aiops-fallback (natural reply) | 18 797 / 19 / 989 | 18 560 / 19 / 976 | 19 533 / 19 / 1 028 | local Strata unconfigured → gateway fallback |

### Findings
1. **LLM calls are the entire latency story.** Sum of `fast` + `aiops-fallback` ≈ 43–45 s per run equals the total task wall time; every non-zero stage (exec, classify) is LLM-bound. Stage wall-clock is sub-ms everywhere else.
2. **QUERY answers (fast) average ~1.2–1.3 s per call** and are invoked 19–20 times per 40-task run. This is the single largest lever for N2 Q&A (P50 2.3 s ≈ one fast call + one fallback call).
3. **Natural replies (aiops-fallback) average ~1.0 s × 19 calls.** On this host `STRATA_API_KEY` is unset, so every conversational reply pays the gateway round trip instead of the local Strata Qwen. Configuring local Strata is a deployment-side fix, not a code fix.
4. **classify costs ~180 ms/task on average** (34 tasks) — the LLM intent fallback fires on low-confidence rules and is a second-order lever.

## 3. N1–N5 gap to targets (what Phase 1 must close)

| Metric | Baseline (median of 3) | Target | Gap |
|---|---|---|---|
| N1b P50 | 835 ms | ≤1500 ms | ✅ met (with headroom) |
| N1b P90 | 3135 ms | ≤3000 ms | 135 ms overshoot |
| N2 Q&A P50 | 2373 ms | ≤3000 ms | ✅ met |
| N2 Q&A P90 | 3367 ms | ≤3000 ms | 367 ms overshoot |
| N3 first-pass | 58% | ≥90% (and ≥baseline+10pp) | −32 pp |
| N4 clarify | 3% | ≤5% | ✅ met |
| N5 empty/false | 0 | 0 | ✅ met |

## 4. M7/M8 symptom judgement (still reproduces: yes/no, per run)

| Task | Symptom (issue) | Run1 | Run2 | Run3 | Judgement |
|---|---|---|---|---|---|
| oc-01 download+build golang/go | long task >90 s (M8 latency) | timeout | timeout | timeout | **yes (reproduces)** — exceeds 90 s, no progress streaming |
| am-01 "帮我处理那个" | clarify should be ask event (M7 Ask form) | done | done | done | **yes (variant)** — receipt says "needs clarification… not sure what you meant" but terminal event is `done`, not `ask`; client may not prompt for answer |
| am-05 "继续" | no-context continue | failed | failed | failed | **yes (variant)** — fails instead of asking; env-limited (no goal/context) |
| am-09 "把项目提交了" | commit without repo context | failed | failed | failed | **yes (variant)** — env-limited (temp dir has no git repo) |
| cm-01/cm-02 commit | commit on non-git temp dir | failed | failed | failed | env-limited, not product evidence |
| oc-05 email analysis | no mail account configured | failed | failed | failed | env-limited |
| dp-02 deploy prod | no deploy target in bench env | failed | failed | failed | env-limited |

Rule applied: a symptom is counted only when the failure is a product-behavior gap, not a missing bench environment. Env-limited cases are recorded but not counted as product regressions.

## 5. Environment limits (disclosed)

- `STRATA_API_KEY` not set on this host → natural replies pay gateway fallback (~1.0 s/call). Baseline N2/N1b would improve if local Strata were configured; Phase 1 lever (b) is not evaluated yet.
- oc-01 (clone+build golang/go) exceeded the 90 s per-task cap; real duration not measured. Streaming/progress would make long tasks feel responsive even if duration is unchanged (Phase 2 territory).
- commit/deploy/email tasks run against an empty temp dir with no git repo, no mail account, no deploy target; their failures are environmental, but they confirm the harness marks them failed honestly (no false done) — N5 = 0.

## 6. Data-supported Phase 1 candidates (Controller decides scope)

Ranked by measured cost, not by document:
1. **Fast-tier latency** (lever a): `fast` QUERY answers avg 1.27 s × ~20 calls/run. Cheapest code-side fix is routing warm queries to a faster model tier / shorter prompts; measured effect = N2 Q&A P50/P90 both dominated by this call.
2. **Natural-reply fallback cost** (lever b partial): 19 calls × ~1.0 s paid to the gateway because local Strata is absent. Code-side candidates: skip LLM reply when a deterministic answer suffices (e.g. "记一下/收到" receipts), or cache repeated conversational replies.
3. **LLM call merging / skipping** (lever b): Q&A P50 ≈ fast + fallback ≈ 2.3 s; a single-call path (one LLM call that both answers and renders) would target N2 Q&A P90 ≤3 s.
4. **classify fallback** (lever a/b): ~180 ms × 34 tasks; only worth touching after 1–3.
5. Non-goal items observed but out of Phase 1: oc-01 long-task streaming (Phase 2), Strata local config (ops).

## 7. Files

- `bench/tasks.json` — frozen task set T (40 tasks, 4 classes, M3/M7/M8 tags)
- `bench/runner_test.go` — reproducible runner (skips without `AIOPS_KEY`; usage header at top)
- `pipeline/timing.go`, `pipeline/pipeline.go`, `provider/provider.go`, `trajectory/kinds.go`, `server/server.go` — additive stage/LLM timing instrumentation (PR `perf/stage-timing`)
