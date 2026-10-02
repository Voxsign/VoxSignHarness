#!/bin/sh
# M7 LLM 端到端验证（预算恢复后一键跑）
# 用法: VHS_TOKEN=xxx VHS_ADDR=127.0.0.1:8897 sh scripts/verify-llm.sh
ADDR="${VHS_ADDR:-127.0.0.1:8897}"
TOK="${VHS_TOKEN:-m7-token}"
H="Authorization: Bearer $TOK"
CT="Content-Type: application/json"

echo "=== 1. 复杂自然问句 → 应判 QUERY 高置信（非 UNKNOWN）==="
curl -s -X POST "http://$ADDR/v1/tasks" -H "$H" -H "$CT" \
  -d '{"text":"你现在用哪台电脑怎么访问怎么确保安全"}'
echo; sleep 2

echo "=== 2. QUERY 返回真实内容（非空壳）==="
echo "（查询上面返回的 task_id 看 receipt）"

echo "=== 3. NOTE 仍正常（基线）==="
curl -s -X POST "http://$ADDR/v1/tasks" -H "$H" -H "$CT" \
  -d '{"text":"记一下 LLM 验证"}'
echo; sleep 1

echo "=== 4. 健康检查 ==="
curl -s "http://$ADDR/v1/status" -H "$H"
echo
