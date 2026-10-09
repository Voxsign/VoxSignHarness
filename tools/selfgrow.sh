#!/bin/bash
# R13 (2026-10-09): self-growth loop (V / self-growing harness) — scans the
# trace audit trail for execution failures / confirmation gates / cost outliers
# and appends a deduplicated issue record to GROWING-ISSUES.md. This makes the
# harness discover its own weaknesses instead of waiting for the next manual
# test round; distillation picks issues from this ledger in later rounds.
#
#   bash tools/selfgrow.sh [trace_dir]
# Default trace_dir: ~/.voicesign/harness/context_slots (local) — pass
# /opt/vhs-unifusion/logs/context_slots via the server for container data.
set -u
DIR="${1:-$HOME/.voicesign/harness/context_slots}"
LEDGER="$(cd "$(dirname "$0")/.." && pwd)/GROWING-ISSUES.md"
[ -d "$DIR" ] || { echo "no trace dir: $DIR"; exit 1; }
[ -f "$LEDGER" ] || { echo "# Growing Issues（harness 自我生长台账）" > "$LEDGER"; echo "" >> "$LEDGER"; echo "由 tools/selfgrow.sh 自动追加；每轮蒸馏从这里取题。" >> "$LEDGER"; echo "" >> "$LEDGER"; }

FOUND=0; ADDED=0
for tf in "$DIR"/trace-*.jsonl; do
  [ -f "$tf" ] || continue
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    ts=$(echo "$line" | python3 -c "import sys,json;print(json.loads(sys.stdin.read()).get('ts',''))" 2>/dev/null)
    action=$(echo "$line" | python3 -c "import sys,json;print(json.loads(sys.stdin.read()).get('action',''))" 2>/dev/null)
    # receipts with ok:false. Confirmation gates (err starts with "准备安装")
    # and deny-once replies (err starts with "上次已取消") are DESIGNED behavior
    # (R6/R13) — filter them out; only real execution failures grow the ledger.
    fails=$(echo "$line" | python3 -c "
import sys,json
d=json.loads(sys.stdin.read())
bad=[]
for r in d.get('receipts',[]):
    if r.get('ok'): continue
    e=r.get('err','')
    if e.startswith('准备安装') or e.startswith('上次已取消'): continue
    bad.append(r['tool']+':'+e[:120])
print('|'.join(bad))" 2>/dev/null)
    [ -z "$fails" ] && continue
    FOUND=$((FOUND+1))
    stamp="${ts}__${action}__${fails:0:80}"
    if ! grep -qF "$stamp" "$LEDGER"; then
      echo "- [$(date '+%Y-%m-%d %H:%M')] trace:${ts} action=${action} issue=${fails}" >> "$LEDGER"
      ADDED=$((ADDED+1))
    fi
  done < "$tf"
done
echo "selfgrow: scanned=$DIR found=$FOUND new=$ADDED ledger=$LEDGER"
[ "$ADDED" -gt 0 ] && echo "  -> new issues appended; next distillation round should address them."
