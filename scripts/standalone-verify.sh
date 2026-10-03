#!/bin/bash
# standalone-verify.sh — VoiceSign Harness「脱离豆包独立运行」一键自检
#
# 用法：bash scripts/standalone-verify.sh [PORT]
# 作用：起服务 → 验证纯规则链路（不依赖模型）→ 验证模型链路（预算/网络）→ 输出诊断
# 依据：2026-10-04 实测（模型中心 daily_budget_exceeded $366.6/$30 → QUERY 降级，规则链路正常）
set -euo pipefail

PORT="${1:-8897}"
ADDR="127.0.0.1:$PORT"
TOK="m7-token"
H="Authorization: Bearer $TOK"
CT="Content-Type: application/json"
PASS=0; FAIL=0

ok()   { echo "  ✅ $1"; PASS=$((PASS+1)); }
bad()  { echo "  ❌ $1"; FAIL=$((FAIL+1)); }

echo "=== 0. 端口占用检测（防止撞上其它服务）==="
if lsof -iTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | grep -v "vhs-m7" | grep -q LISTEN; then
  echo "  ⚠️ 端口 $PORT 已被其它进程占用（见上），换端口重试：bash $0 8899"
  lsof -iTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | head -3
  exit 2
fi
ok "端口 $PORT 可用"

echo "=== 1. 起服务（m7-serve.sh）==="
bash "$(dirname "$0")/m7-serve.sh" "$PORT" >/dev/null 2>&1 || { echo "FATAL: 起服务失败"; exit 1; }
ok "serve 启动"

echo "=== 1. 纯规则链路（不依赖模型，必须通）==="
curl -s --max-time 5 "http://$ADDR/v1/status" -H "$H" | grep -q '"ok":true' && ok "health/status" || bad "status"

TID=$(curl -s -X POST "http://$ADDR/v1/tasks" -H "$H" -H "$CT" \
  -d '{"text":"记一下独立运行自检"}' | python3 -c "import sys,json;print(json.load(sys.stdin).get('task_id',''))")
sleep 4
REC=$(curl -s "http://$ADDR/v1/tasks/$TID" -H "$H")
echo "$REC" | grep -q '"status":"done"' && ok "NOTE 任务 done（纯规则）" || bad "NOTE 未 done: ${REC:0:120}"

TID2=$(curl -s -X POST "http://$ADDR/v1/tasks" -H "$H" -H "$CT" \
  -d '{"text":"帮我查一下 VoxSign 的 readme 文件在哪里"}' | python3 -c "import sys,json;print(json.load(sys.stdin).get('task_id',''))")
# 模型 429 会触发退避重试（≤2 轮），给足窗口（最多 40s）
for i in $(seq 1 8); do
  sleep 5
  REC2=$(curl -s "http://$ADDR/v1/tasks/$TID2" -H "$H")
  if echo "$REC2" | grep -q '"status":"done"\|"status":"failed"\|"status":"canceled"'; then break; fi
done
if echo "$REC2" | grep -q '"status":"done"'; then
  ok "QUERY 任务 done"
  echo "$REC2" | grep -q "模型服务暂不可用" && echo "  ⚠️ 模型降级：QUERY 回答为空（预算/网络问题，非代码缺陷）"
else
  bad "QUERY 未 done: ${REC2:0:120}"
fi

echo "=== 2. 模型链路诊断 ==="
tail -5 /tmp/vhs-m7/log/server.log | grep -E "err:|budget|empty" | head -3 || true
if grep -qE "daily_budget_exceeded|429" /tmp/vhs-m7/log/server.log 2>/dev/null; then
  echo "  ⚠️ 模型中心预算超支（429 daily_budget_exceeded）→ 需人工充值/提高预算"
fi

echo
echo "=== 结果：规则链路 PASS=${PASS} FAIL=${FAIL}；模型链路见上方诊断 ==="
echo "若规则链路全 PASS → iOS 端侧 ASR→文本→提交→SSE 可独立跑通（无需豆包）；"
echo "QUERY 降级仅影响回答质量，不影响识别/分类/执行/回执。"
exit ${FAIL}
