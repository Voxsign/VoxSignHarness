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
# A5–A7 判据语义 = **规则式分批**（§5.1 1/2/3，确定性）；头部在途的 L2 规划（Lead 实测不稳定：
# 上游 502/模型拒绝/≈60s）**不属于 A1–A12 判据范围** ⇒ 验收默认钉住规则式配置，
# **服务端如实可见**：日志 "L2 未装配（模型中心配置读取失败）" + 响应 l2_enabled=false + l2_note。
# 若调用方显式设置了 VHS_MODEL_CENTER，则尊重之（escape hatch）。
if [ -z "${VHS_MODEL_CENTER:-}" ]; then
  VHS_MODEL_CENTER="$DATA/model-center.unavailable.json"
  echo "[accept] 规划判据按规则式语义验证（A5-A7 语义 = 规则式分批，头部 L2 在途状态见验收报告 §6）"
  echo "[accept] VHS_MODEL_CENTER=$VHS_MODEL_CENTER"
  echo "[accept] （指向不存在的文件 ⇒ 服务端如实日志 L2 未装配 + 响应 l2_enabled=false）"
else
  echo "[accept] 尊重调用方 VHS_MODEL_CENTER=$VHS_MODEL_CENTER（L2 规划行为随头部在途实现）"
fi
VHS_ASR_ADDR="127.0.0.1:$PORT" VHS_ASR_DATA="$DATA" VHS_MODEL_CENTER="$VHS_MODEL_CENTER" "$BIN" > /tmp/vhs-a12-server.log 2>&1 &
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

# ---- 校验对齐门槛（2026-10-04 硬规则：验收输出前必经动作）----
# 任意验收结论输出前，必须先过校验对齐（validate-align：判据双证 PASS ≥90% 且 0 FAIL）。
# 机器可读对齐结果约定：docs/align-results/ALIGN.json（{"aligned": true|false, "score": N, ...}），
# 由校验对齐执行后生成；accept.sh 只做门槛强制（计数），不做判断本身。
ALIGN_FILE="${VHS_ALIGN_RESULT:-$ROOT/docs/align-results/ALIGN.json}"
if [ "${VHS_SKIP_ALIGN:-0}" = "1" ]; then
  echo "[accept] ⚠️ VHS_SKIP_ALIGN=1 ⇒ 跳过校验对齐门槛（仅测试/历史验收，正式交付禁止跳过）"
elif [ ! -f "$ALIGN_FILE" ]; then
  echo "[accept] ❌ 校验对齐门槛：未找到 $ALIGN_FILE ⇒ 禁止出验收（未对齐不许交付）"
  RC=1
else
  ALIGNED=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1])).get('aligned'))" "$ALIGN_FILE" 2>/dev/null || echo "parse_fail")
  SCORE=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1])).get('score'))" "$ALIGN_FILE" 2>/dev/null || echo "?")
  if [ "$ALIGNED" = "True" ] || [ "$ALIGNED" = "true" ]; then
    echo "[accept] ✅ 校验对齐门槛 PASS（aligned=true · score=${SCORE}% · 见 $ALIGN_FILE）"
  else
    echo "[accept] ❌ 校验对齐门槛：aligned=${ALIGNED:-parse_fail} ⇒ 禁止出验收（对齐度未达标）"
    RC=1
  fi
fi

echo "[accept] 验收数据目录（可复核 feedback.jsonl / blacklist.json / reallog.jsonl）："
echo "[accept]   $DATA"
echo "[accept] 服务日志：/tmp/vhs-a12-server.log"
exit $RC
