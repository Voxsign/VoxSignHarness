#!/bin/sh
# accept.sh —— A1–A12 完整验收（交接 §5.2，Peter 原话：做一个稳定的版本，改完测完了再弄）。
#
# 驱动方式：Chrome --headless=new --remote-debugging-port + Node ≥22 原生 WebSocket（CDP），
# **零 npm 依赖**。无 Chrome / 无 node ⇒ 显式 skip 并大声说明（不许静默变弱）。
#
# 用法：sh scripts/accept.sh
# 环境：VHS_ACCEPT_PORT（默认 8133）· VHS_ACCEPT_DATA（默认 mktemp，验收后保留可复核）
#      VHS_ACCEPT_CDP_PORT（Chrome 调试口，默认 9333）
set -e
ROOT=$(cd "$(dirname "$0")/.." && pwd)
CHROME="${VHS_ACCEPT_CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
NODE_BIN="$(command -v node || true)"

if [ -z "$NODE_BIN" ]; then
  echo "[accept] ⚠️ 无 node ⇒ SKIP A1–A12（不许静默变弱）。请安装 Node ≥22 后重跑。"
  exit 2
fi
if [ ! -x "$CHROME" ]; then
  echo "[accept] ⚠️ 无 Chrome（$CHROME）⇒ SKIP A1–A12（不许静默变弱）。"
  exit 2
fi

PORT="${VHS_ACCEPT_PORT:-8133}"
DATA="${VHS_ACCEPT_DATA:-$(mktemp -d /tmp/vhs-a12.XXXXXX)}"
BINDIR="$(mktemp -d /tmp/vhs-a12bin.XXXXXX)"
BIN="$BINDIR/vhs-asr"

echo "[accept] ① 一条命令起服务（A1）：build 真二进制 + 起服务 port=$PORT data=$DATA"
( cd "$ROOT" && go build -o "$BIN" ./cmd/vhs-asr )
VHS_ASR_ADDR="127.0.0.1:$PORT" VHS_ASR_DATA="$DATA" "$BIN" > /tmp/vhs-a12-server.log 2>&1 &
SRV=$!
trap 'kill $SRV 2>/dev/null || true; rm -rf "$BINDIR"' EXIT INT TERM

i=0
while [ $i -lt 120 ]; do
  if curl -fsS -m 1 "http://127.0.0.1:$PORT/v1/health" >/dev/null 2>&1; then break; fi
  i=$((i+1)); sleep 0.5
done
if ! curl -fsS -m 1 "http://127.0.0.1:$PORT/v1/health" >/dev/null 2>&1; then
  echo "[accept] 服务 60s 未起 ⇒ 失败（看 /tmp/vhs-a12-server.log）"
  tail -8 /tmp/vhs-a12-server.log
  exit 1
fi
echo "[accept] 服务已就绪，跑 A1–A12（CDP）…"
VHS_ACCEPT_SRV_PID=$SRV VHS_ACCEPT_PORT=$PORT VHS_ACCEPT_DATA=$DATA \
VHS_ACCEPT_CDP_PORT="${VHS_ACCEPT_CDP_PORT:-9333}" VHS_ACCEPT_CHROME="$CHROME" \
  node "$ROOT/scripts/accept-a1a12.mjs"
RC=$?
kill $SRV 2>/dev/null || true
echo "[accept] 验收数据目录：$DATA（可复核 feedback.jsonl / blacklist.json / reallog.jsonl）"
echo "[accept] 服务日志：/tmp/vhs-a12-server.log"
exit $RC
