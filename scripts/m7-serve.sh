#!/bin/bash
# M7 server 一键启动（含模型中心接线 + 异常自愈 diag provider）
# 用法：bash scripts/m7-serve.sh [PORT]
# 依赖：~/.modelcenter/creds.txt（模型中心 key）；/tmp/vhs-m7-config.json 会自动重建
set -euo pipefail

PORT="${1:-8897}"
LOG_DIR="${VHS_LOG_DIR:-/tmp/vhs-m7/log}"
BIN="${VHS_BIN:-/tmp/vhs-m7-binary}"
CONF="${VHS_CONF:-/tmp/vhs-m7-config.json}"

# 0) key（从模型中心凭证文件取首个 sk-mc- token）
KEY=$(grep -oE 'sk-mc-[a-zA-Z0-9]+' "$HOME/.modelcenter/creds.txt" | head -1)
if [ -z "$KEY" ]; then echo "FATAL: 未找到模型中心 key（~/.modelcenter/creds.txt）"; exit 1; fi

# 1) 配置文件（providers：center/fast/strong=gpt-6-luna + diag=jev-diagnose）
if [ ! -f "$CONF" ]; then
  mkdir -p "$(dirname "$CONF")"
  cat > "$CONF" <<'JSON'
{
  "providers": [
    {"name":"center","kind":"openai","endpoint":"https://model.peterzou.com/v1","model":"gpt-6-luna","response_format":true,"params":{"use_max_completion_tokens":true}},
    {"name":"fast","kind":"openai","endpoint":"https://model.peterzou.com/v1","model":"gpt-6-luna","response_format":true,"params":{"use_max_completion_tokens":true}},
    {"name":"strong","kind":"openai","endpoint":"https://model.peterzou.com/v1","model":"gpt-6-luna","response_format":true,"params":{"use_max_completion_tokens":true}},
    {"name":"deepseek","kind":"openai","endpoint":"https://api.deepseek.com","model":"deepseek-flash","response_format":true},
    {"name":"openai","kind":"openai","endpoint":"https://api.openai.com/v1","model":"gpt-5.4-mini","response_format":true},
    {"name":"gemini","kind":"openai","endpoint":"https://generativelanguage.googleapis.com/v1beta/openai","model":"gemini-3.8-flash","response_format":true},
    {"name":"mock","kind":"mock","model":"mock"},
    {"name":"diag","kind":"openai","endpoint":"https://model.peterzou.com/v1","model":"jev-diagnose","response_format":true}
  ]
}
JSON
fi

# 2) 日志目录
mkdir -p "$LOG_DIR/tasks" "$LOG_DIR/spaces"

# 3) 构建（若二进制缺失或源码更新）
if [ ! -x "$BIN" ] || [ -n "$(find . -name '*.go' -newer "$BIN" | head -1)" ]; then
  echo ">> 重新构建 $BIN ..."
  go build -o "$BIN" .
fi

# 4) 启动（幂等：先停旧）
pkill -f "$(basename "$BIN") serve" 2>/dev/null || true
sleep 1
VHS_ADDR=0.0.0.0:$PORT VHS_TOKEN=m7-token VHS_LOG_DIR="$LOG_DIR" \
  VHS_API_KEY="$KEY" VHS_CONFIG="$CONF" \
  nohup "$BIN" serve > "$LOG_DIR/server.log" 2>&1 &
echo ">> server pid=$! port=$PORT log=$LOG_DIR/server.log"

# 5) 健康检查
sleep 2
curl -s --max-time 5 "http://127.0.0.1:$PORT/v1/status" -H "Authorization: Bearer m7-token" | head -c 200
echo
