#!/usr/bin/env bash
# Mutation tests for the action package.
#
# Injects each fatal reliability defect one at a time, confirms the targeted
# regression test goes RED, then restores the source and confirms the whole
# package is green again. A mutation that stays GREEN means the test suite does
# not actually guard that invariant and must be strengthened.
#
# Usage:  bash scripts/action-mutate.sh
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO" || exit 1

ACTION="action/action.go"
LOGIC="action/storelogic.go"
cp "$ACTION" /tmp/vhs-m-action.go.bak
cp "$LOGIC" /tmp/vhs-m-storelogic.go.bak

restore() {
  cp /tmp/vhs-m-action.go.bak "$ACTION"
  cp /tmp/vhs-m-storelogic.go.bak "$LOGIC"
}
trap restore EXIT

run_mut() {
  local name="$1" file="$2" pat="$3" rep="$4" testname="$5" flag="${6:-}"
  restore
  if [ "$flag" = "g" ]; then
    perl -0pi -e "s/\Q${pat}\E/${rep}/g" "$file"
  else
    perl -0pi -e "s/\Q${pat}\E/${rep}/" "$file"
  fi
  if go test -count=1 -run "$testname" ./action/ >"/tmp/vhs-mut-${name}.log" 2>&1; then
    echo "MUT ${name}: !!! NOT CAPTURED !!!"
    fail=1
  else
    echo "MUT ${name}: CAPTURED (regression test failed as expected)"
  fi
  restore
}

fail=0

# M1: a failed ledger write after a confirmed OK must never become SUCCEEDED.
run_mut "M1-false-success" "$ACTION" \
  'Status: StatusUnknown, ExternalRef: ref, NeedsReconcile: true,' \
  'Status: StatusSucceeded, ExternalRef: ref, NeedsReconcile: false,' \
  'TestLedgerMarkFailureAfterConfirmedOKIsUnknown'

# M2: an in-flight action held by another live worker must not be taken over.
run_mut "M2-inflight-takeover" "$LOGIC" \
  'return r, DecisionInFlight' \
  'return r, DecisionAcquired' \
  'TestInFlightNotReportedSuccess' "g"

# M3: an UNKNOWN action must not be blindly replayed without reconciliation.
run_mut "M3-unknown-replay" "$LOGIC" \
  'return r, DecisionNeedsReconcile' \
  'r.takeOver(owner, lease, now); return r, DecisionAcquired' \
  'TestHandlerErrorAndInconclusiveAreUnknown'

restore
echo "---"
if go test -race -count=1 ./action/ >/tmp/vhs-mut-restore.log 2>&1; then
  echo "RESTORE CHECK: all green"
else
  echo "RESTORE CHECK: FAILED"
  cat /tmp/vhs-mut-restore.log
  fail=1
fi

exit "$fail"
