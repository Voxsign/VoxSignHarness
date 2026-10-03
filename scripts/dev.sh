#!/bin/sh
# dev.sh —— **一条命令起测试页**（Peter：赶快能测）。
#
# 用法：sh scripts/dev.sh
#   → 起服务（默认 127.0.0.1:8123），自动打开浏览器到测试页
# 环境：VHS_ASR_PORT（默认 8123）· VHS_ASR_DATA（默认 <repo>/data/asr）
#       可选 AIOPS_KEY（有则启用模型兜底；没有也能用，纯本地）
set -e
ROOT=$(cd "$(dirname "$0")/.." && pwd)
PORT="${VHS_ASR_PORT:-8123}"
DATA="${VHS_ASR_DATA:-$ROOT/data/asr}"
mkdir -p "$DATA"
echo "[dev] 数据目录: $DATA（真实台账: $DATA/reallog.jsonl）"
echo "[dev] 打开: http://127.0.0.1:$PORT/"
( cd "$ROOT" && VHS_ASR_ADDR="127.0.0.1:$PORT" VHS_ASR_DATA="$DATA" exec go run ./cmd/vhs-asr ) &
SRV=$!
trap 'kill $SRV 2>/dev/null || true' INT TERM
# 等服务就绪（最多 20s）
i=0
while [ $i -lt 40 ]; do
  if curl -fsS -m 1 "http://127.0.0.1:$PORT/v1/health" >/dev/null 2>&1; then break; fi
  i=$((i+1)); sleep 0.5
done
open "http://127.0.0.1:$PORT/" 2>/dev/null || echo "[dev] 请手动打开 http://127.0.0.1:$PORT/"
wait $SRV
