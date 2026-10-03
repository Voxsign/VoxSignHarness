#!/bin/bash
BASE=http://127.0.0.1:8765
TOK="Authorization: Bearer ios-test-token"
J="Content-Type: application/json"
PASS=0; FAIL=0
ok(){ echo "  ✅ $1"; PASS=$((PASS+1)); }
no(){ echo "  ❌ $1"; FAIL=$((FAIL+1)); }

echo "== A. 打断语义（用长任务：跑测试）=="
T=$(curl -s -X POST $BASE/v1/tasks -H "$TOK" -H "$J" -d '{"text":"跑一遍 go test ./... 全量测试","mode":"text"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['task_id'])")
( timeout 15 curl -sN -H "$TOK" "$BASE/v1/tasks/$T/events" > /tmp/vhs-ios-test/ea.txt 2>&1 & )
sleep 3
echo "  cancel 于 $(date +%T)"
curl -s -X POST "$BASE/v1/tasks/$T/cancel" -H "$TOK" -H "$J" -d '{"reason":"用户打断"}' | head -c 200; echo
sleep 4
grep -q "^event: interrupt" /tmp/vhs-ios-test/ea.txt && ok "interrupt 即时到达" || no "无 interrupt: $(head -c 200 /tmp/vhs-ios-test/ea.txt)"
grep -q "^event: canceled" /tmp/vhs-ios-test/ea.txt && ok "canceled 终态到达" || no "无 canceled: $(tail -c 200 /tmp/vhs-ios-test/ea.txt)"

echo "== B. after 重连幂等（挂起时重连）=="
T2=$(curl -s -X POST $BASE/v1/tasks -H "$TOK" -H "$J" -d '{"text":"整理 docs 目录下的文件清单","mode":"text"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['task_id'])")
( timeout 12 curl -sN -H "$TOK" "$BASE/v1/tasks/$T2/events" > /tmp/vhs-ios-test/eb.txt 2>&1 & )
sleep 5   # 等 need_ask 或 stage 出现
SEQS=$(python3 -c "import re;print(','.join(re.findall(r'\"seq\":(\d+)',open('/tmp/vhs-ios-test/eb.txt').read())) )")
echo "  首连 seqs=$SEQS"
LAST=$(python3 -c "import re;s=open('/tmp/vhs-ios-test/eb.txt').read();m=re.findall(r'\"seq\":(\d+)',s);print(m[-1] if m else 0)")
timeout 8 curl -sN -H "$TOK" "$BASE/v1/tasks/$T2/events?after=$LAST" > /tmp/vhs-ios-test/eb2.txt 2>&1
echo "  after=$LAST 重连内容:"
cat /tmp/vhs-ios-test/eb2.txt | head -12
RSEQS=$(python3 -c "import re;print(','.join(re.findall(r'\"seq\":(\d+)',open('/tmp/vhs-ios-test/eb2.txt').read())) if open('/tmp/vhs-ios-test/eb2.txt').read().strip() else 'EMPTY')")
if [ "$RSEQS" = "EMPTY" ]; then
  # 连接保持/无新事件：合法（无重放）
  ok "after=$LAST 无重放（无新事件）"
elif python3 -c "
import re,sys
old=list(map(int,'$SEQS'.split(','))) if '$SEQS' else []
new=list(map(int,re.findall(r'\"seq\":(\d+)',open('/tmp/vhs-ios-test/eb2.txt').read())))
print('PASS' if all(n>($LAST if $LAST else 0) and n not in old for n in new) else 'FAIL')" | grep -q PASS; then
  ok "after=$LAST 只回放新事件 seq=$RSEQS"
else
  no "after 重放重复: old=$SEQS new=$RSEQS"
fi

echo
echo "===== 结果: PASS=$PASS FAIL=$FAIL ====="
