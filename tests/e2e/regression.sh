#!/bin/bash
# R13 (2026-10-09): automated regression suite (V layer) — replays the distilled
# end-to-end scenarios against the private local Qwen and asserts the
# harness behavior, not just output presence. Run before every release:
#   bash tests/e2e/regression.sh
# Requires: bin/vhs built (go build -o bin/vhs .), STRATA_API_KEY, AIOPS_KEY set.
# Note: CI (GitHub Actions) cannot reach the LAN Qwen — this is the developer
# gate; CI keeps unit tests. Scenario failures exit non-zero.

set -u
cd "$(dirname "$0")/../.." || exit 1
VHS=./bin/vhs
[ -x "$VHS" ] || { echo "build first: go build -o bin/vhs ."; exit 1; }
# Credentials and the local endpoint are deployment-private: supply them via the
# environment, they are never hardcoded in the repository.
: "${STRATA_API_KEY:?set STRATA_API_KEY to the local Qwen key}"
: "${AIOPS_KEY:?set AIOPS_KEY to the aiops gateway key}"
export STRATA_API_KEY AIOPS_KEY
: "${VHS_EMAIL_STRATA:?set VHS_EMAIL_STRATA to the private local Qwen base URL}"
export VHS_EMAIL_STRATA

PASS=0; FAIL=0
run() { # run <label> <expected-grep> <cmd...>
  local label="$1" expect="$2"; shift 2
  local out; out=$("$@" 2>&1)
  if echo "$out" | grep -qE "$expect"; then PASS=$((PASS+1)); echo "PASS $label";
  else FAIL=$((FAIL+1)); echo "FAIL $label"; echo "  expect: $expect"; echo "  --- full output ---"; echo "$out" | head -12; fi
}

# --- R6/R11: install confirmation gate + R13 deny-once ---
run "install-gate" "待确认" $VHS run "装一下 codex"
run "cancel-honest" "已取消" $VHS run "算了，不装了"
run "deny-once-block" "上次已取消安装" $VHS run "装一下 codex"
run "revive-reconfirm" "待确认" $VHS run "还是要装 codex"
run "cancel-cleanup" "已取消" $VHS run "算了，不装了"

# --- R7: email processing chain ---
run "email-summary" "处理结果|邮件|收件箱|未读" $VHS run "处理一下邮件"

# --- R11: english boundary (never the R5 english-rejection template) ---
run "en-greet" "你好|好的|记住|Hello|hi|在的|帮你" $VHS run "hello"

# --- R12: multi-task order + interruption ---
run "seq-chain" "处理结果|记一条|已记住|邮件" $VHS run "先看邮件，再记一条：周五下午2点发货"

# --- R13: trace audit trail ---
TRACE=~/.voicesign/harness/context_slots/trace-default.jsonl
if [ -s "$TRACE" ] && [ "$(wc -l < "$TRACE")" -ge 1 ]; then
  # last line must carry request_id + ts + receipts (O layer fields)
  if tail -1 "$TRACE" | grep -q '"request_id"' && tail -1 "$TRACE" | grep -q '"ts"'; then
    PASS=$((PASS+1)); echo "PASS trace-audit-trail"
  else FAIL=$((FAIL+1)); echo "FAIL trace-audit-trail (fields missing in last line)"; fi
else FAIL=$((FAIL+1)); echo "FAIL trace-audit-trail (trace file absent/empty)"; fi

echo "===== regression: PASS=$PASS FAIL=$FAIL ====="
# self-growth: harvest execution issues from the audit trail into GROWING-ISSUES.md
# (never blocks the gate — it feeds the NEXT distillation round)
bash "$(dirname "$0")/../../tools/selfgrow.sh" >/dev/null 2>&1 || true
[ "$FAIL" -eq 0 ]
