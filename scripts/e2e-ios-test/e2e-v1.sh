#!/bin/bash
# E2E 契约测试 v1：模拟 iOS 客户端全流程
BASE=http://127.0.0.1:8765
TOK="Authorization: Bearer ios-test-token"
J="Content-Type: application/json"
PASS=0; FAIL=0
ok(){ echo "  ✅ $1"; PASS=$((PASS+1)); }
no(){ echo "  ❌ $1"; FAIL=$((FAIL+1)); }

echo "== 1. 健康/状态 =="
H=$(curl -s -H "$TOK" $BASE/v1/health); echo "$H" | grep -q '"ok":true' && ok "health ok" || no "health: $H"

echo "== 2. 提交任务(QUERY) → SSE → done =="
T=$(curl -s -X POST $BASE/v1/tasks -H "$TOK" -H "$J" -d '{"text":"查一下 docs 目录下有哪些设计文档","mode":"text"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['task_id'])")
echo "  task=$T"
timeout 20 curl -sN -H "$TOK" "$BASE/v1/tasks/$T/events" > /tmp/vhs-ios-test/e1.txt 2>&1
grep -q "^event: stage" /tmp/vhs-ios-test/e1.txt && ok "收到 stage 事件" || no "无 stage"
grep -q "^event: done" /tmp/vhs-ios-test/e1.txt && ok "收到 done 事件" || no "无 done"
# seq 递增检查
python3 - <<'PY'
import re
s=open('/tmp/vhs-ios-test/e1.txt').read()
seqs=[int(m) for m in re.findall(r'"seq":(\d+)',s)]
print("  seq序列:",seqs,"严格递增" if seqs==sorted(seqs) and len(set(seqs))==len(seqs) else "异常")
PY

echo "== 3. 重连幂等 after=lastSeq =="
LAST=$(python3 -c "import re;print(max(int(m) for m in re.findall(r'\"seq\":(\d+)',open('/tmp/vhs-ios-test/e1.txt').read())))")
timeout 8 curl -sN -H "$TOK" "$BASE/v1/tasks/$T/events?after=$LAST" > /tmp/vhs-ios-test/e2.txt 2>&1
if [ -s /tmp/vhs-ios-test/e2.txt ] && ! grep -q "^event:" /tmp/vhs-ios-test/e2.txt; then ok "after=$LAST 无重放(连接保持或正常关闭)"; else echo "  (after 重放内容: $(head -c 200 /tmp/vhs-ios-test/e2.txt))"; no "after 重放非预期"; fi

echo "== 4. 打断语义 cancel → interrupt → canceled =="
T2=$(curl -s -X POST $BASE/v1/tasks -H "$TOK" -H "$J" -d '{"text":"生成《测试-打断语义》文档并保存提交","mode":"text"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['task_id'])")
( timeout 15 curl -sN -H "$TOK" "$BASE/v1/tasks/$T2/events" > /tmp/vhs-ios-test/e3.txt 2>&1 & )
sleep 2
curl -s -X POST "$BASE/v1/tasks/$T2/cancel" -H "$TOK" -H "$J" -d '{"reason":"用户打断"}' > /dev/null
sleep 3
grep -q "^event: interrupt" /tmp/vhs-ios-test/e3.txt && ok "cancel 后收到 interrupt" || no "无 interrupt (内容: $(head -c 300 /tmp/vhs-ios-test/e3.txt))"
grep -q "^event: canceled" /tmp/vhs-ios-test/e3.txt && ok "随后收到 canceled" || no "无 canceled"

echo "== 5. 决策点 need_ask → answer 续跑 =="
T3=$(curl -s -X POST $BASE/v1/tasks -H "$TOK" -H "$J" -d '{"text":"整理 docs 下文件清单","mode":"text"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['task_id'])")
( timeout 18 curl -sN -H "$TOK" "$BASE/v1/tasks/$T3/events" > /tmp/vhs-ios-test/e4.txt 2>&1 & )
sleep 4
grep -q "^event: need_ask" /tmp/vhs-ios-test/e4.txt && ok "need_ask 到达(挂起)" || no "无 need_ask (内容: $(head -c 300 /tmp/vhs-ios-test/e4.txt))"
ANS=$(python3 -c "
import re
s=open('/tmp/vhs-ios-test/e4.txt').read()
m=re.search(r'\"options\":\[\{\"id\":\"([^\"]+)\"',s)
print(m.group(1) if m else '列出文件')
" 2>/dev/null)
[ -z "$ANS" ] && ANS="列出文件"
curl -s -X POST "$BASE/v1/tasks/$T3/answer" -H "$TOK" -H "$J" -d "{\"answer\":\"$ANS\"}" > /dev/null
sleep 4
grep -qE "^event: (done|failed)" /tmp/vhs-ios-test/e4.txt && ok "answer 后续跑至终态" || no "answer 后未到终态 (内容: $(tail -c 300 /tmp/vhs-ios-test/e4.txt))"

echo
echo "===== 结果: PASS=$PASS FAIL=$FAIL ====="
