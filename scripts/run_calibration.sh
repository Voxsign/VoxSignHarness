#!/bin/sh
# run_calibration.sh —— **R 通道：真跑取证**（`skills/validate-align/SKILL.md` v2 承诺的脚本之一）。
#
# 依据（skill 原文）：
#   | **R 真跑校准** | 判据 → 运行时行为：起服务、打接口、断言 | 真装配（不许桩）：build+serve+curl/脚本 | 每条判据的运行证据（响应/日志/数据） |
#   | 双向目标 | **产物验收** vs **能力验收** —— 对产物（如 vhs-asr）与对执行主体（harness）**分开取证** |
#   | 过程轴 | 真装配（**不许桩**）→ 可复现（同一命令重跑一致）→ 可验证（每结论有证据） |
#
# ⚠️ 本脚本**只产 R 证据**。按 skill：**PASS = C + R 双证齐**；只有 R 无 C ⇒ 也不是 PASS。
#
# 用法：
#   sh scripts/run_calibration.sh <判据清单文件>
#
# 判据清单格式（每行一条，`|` 分隔；`#` 开头为注释）：
#   <判据ID> | <对象：产物|能力> | <METHOD> | <路径> | <期望（子串，命中即 PASS）> [| <请求体 JSON>] [| <前提>]
#
# ⚠️ **前提列**（2026-10-03 实测第 15/18 条的同族）：
#   V-03「线 B 不可达 ⇒ 503+degraded」**只在"线 B 不可达"时才有意义**。
#   线 B 活着时它必然 ✗ —— 而那是**"前提不满足"，不是"产品坏"**。
#   ⇒ 前提写 `线B不可达` / `线B可用`；不满足时标 **⚠️ 前提不满足**（不算 ✗ 也不算 ✅）。
#
# ⚠️ **请求体不是可选的装饰**（2026-10-03 实测）：`/v1/voice` 要求 `{"text":"…"}`，
#    只发 `{}` ⇒ **400**（`请求体应为 JSON {text}（ASR 识别文本）`）⇒ 判据永远红。
#    ⇒ 一条 R 证据必须**同时**说明"打什么路径、带什么体、期望什么内容"。
#
# 例（产物验收）：
#   R-1 | 产物 | GET | /v1/health | "ok"
#   R-2 | 能力 | POST | /v1/run | "task_id"
#
# 输出：每条判据 → HTTP 状态 + 响应片段（运行证据）；子串不命中 ⇒ ✗
# 退出码：0 = 每条都命中；1 = 有判据 ✗；2 = 用法/环境错

set -u
ROOT=$(cd "$(dirname "$0")/.." && pwd)

if [ $# -lt 1 ] || [ ! -f "$1" ]; then
  echo "用法: sh scripts/run_calibration.sh <判据清单文件>"
  echo "格式: <判据ID> | <产物|能力> | <METHOD> | <路径> | <期望子串>"
  exit 2
fi
LIST="$1"

PORT="${VHS_CALIB_PORT:-8144}"
DATA="${VHS_CALIB_DATA:-$(mktemp -d /tmp/vhs-calib.XXXXXX)}"
BINDIR="$(mktemp -d /tmp/vhs-calibbin.XXXXXX)"
BIN="$BINDIR/vhs-asr"
LOG=/tmp/vhs-calib-server.log
OUT=".calib-evidence-R.md"

cleanup() { [ -n "${SRV:-}" ] && kill "$SRV" 2>/dev/null || true; rm -rf "$BINDIR"; }
trap cleanup EXIT INT TERM

echo "[calib-R] ① **真装配**（不许桩）：build 真二进制"
( cd "$ROOT" && go build -o "$BIN" . ) || { echo "[calib-R] ❌ build 失败 ⇒ 无 R 证据"; exit 2; }

# ⚠️ 与 accept.sh 同款：默认钉住规则式配置，并**如实说明**（不许静默变弱）
if [ -z "${VHS_MODEL_CENTER:-}" ]; then
  VHS_MODEL_CENTER="$DATA/model-center.unavailable.json"
  echo "[calib-R] ⚠️ 未设 VHS_MODEL_CENTER ⇒ 指向不存在的文件（规则式语义）"
  echo "[calib-R]    ⇒ **本次 R 证据只覆盖规则式路径**；L2/模型路径**未被本次真跑覆盖**（如实标注）"
fi

# ⚠️ **起前必须断言端口空闲**（2026-10-03 实测事故；本项目实例集里记过同一个坑）：
# 我并发跑 gate 时 `go test -tags vhsui ./...` 也在起服务；而 `--addr` 被忽略，
# 于是**我的 R 通道打到了别人的服务**（拿到测试页的 200 HTML），**差点据此判"产品红"**。
# ⇒ 规则：**端口被占 ⇒ 拒绝跑（exit 2）**，而不是"打上去看看"。
#   ⚠️ 这也印证「校准」：**先声明参考系，再取证** —— 拿不到干净参考系就不取证。
# ⚠️ **线 B 必须一起起**（2026-10-03 实测，第 15 条教训）：
# 我第一版只起线 A ⇒ 7 条判据里 5 条返回**同一个 503**（ASR 不可达）——
# 它们**在"前置依赖缺失"处就 short-circuit 了** ⇒ **一串"看起来是红、其实没跑到"的结论。**
# ⇒ 产品真实运行的参考系 = **线 A 与线 B 同时活着**；少了线 B 就不是那个参考系。
ASR_PORT="${VHS_CALIB_ASR_PORT:-8123}"
# ⚠️ **两个二进制不能同名**（2026-10-03 实测）：我第一版把线 A 与线 B 都建到
# `$BINDIR/vhs-asr` ⇒ **线 B 的 build 覆盖了线 A** ⇒ `"$BIN" serve` 跑的其实是线 B
# （它不认 `serve`，于是监听 8144 而非 8765）⇒ R 通道永远打不到 /v1/voice。
ASR_BIN="$BINDIR/vhs-asr-lineB"
echo "[calib-R] ①b **真装配线 B**（ASR 服务）build + 起服务 :$ASR_PORT"
( cd "$ROOT" && go build -o "$ASR_BIN" ./cmd/vhs-asr ) || { echo "[calib-R] ❌ 线 B build 失败 ⇒ 无 R 证据"; exit 2; }
if lsof -nP -iTCP:"$ASR_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "[calib-R] ❌ **线 B 端口 $ASR_PORT 已被占用** ⇒ 拒绝跑（同 ②）"
  lsof -nP -iTCP:"$ASR_PORT" -sTCP:LISTEN 2>/dev/null | tail -n +2 | awk '{print "      PID "$2" "$1}' | head -3
  exit 2
fi
VHS_ASR_ADDR="127.0.0.1:$ASR_PORT" VHS_ASR_DATA="$DATA/asr" "$ASR_BIN" > "$LOG.asr" 2>&1 &
SRV_ASR=$!
i=0; while [ $i -lt 40 ]; do
  curl -sf -m 2 "http://127.0.0.1:$ASR_PORT/v1/health" >/dev/null 2>&1 && break
  i=$((i+1)); sleep 0.25
done
if [ $i -ge 40 ]; then
  echo "[calib-R] ❌ 线 B 未就绪 ⇒ 拒取证（否则判据会在 ASR 不可达处 short-circuit）"
  tail -6 "$LOG.asr" 2>/dev/null | sed 's/^/         /'
  exit 2
fi
echo "[calib-R]    线 B 就绪（${i}×0.25s）"
# 线 B 也要清理
cleanup() { [ -n "${SRV:-}" ] && kill "$SRV" 2>/dev/null || true; [ -n "${SRV_ASR:-}" ] && kill "$SRV_ASR" 2>/dev/null || true; rm -rf "$BINDIR"; }

echo "[calib-R] ② 起真服务 addr=127.0.0.1:$PORT"
if lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "[calib-R] ❌ **端口 $PORT 已被占用** ⇒ 拒绝跑（否则会打到别人的服务，得出假结论）"
  lsof -nP -iTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | tail -n +2 | awk '{print "      PID "$2" "$1}' | head -5
  echo "[calib-R]    ⇒ 处置：停掉占用者，或设 VHS_CALIB_PORT 换端口。"
  exit 2
fi
VHS_ASR_ADDR="127.0.0.1:$PORT" VHS_ASR_DATA="$DATA" VHS_MODEL_CENTER="$VHS_MODEL_CENTER" \
  "$BIN" serve > "$LOG" 2>&1 &
SRV=$!

# 等服务就绪（真跑不许"睡死了就当起来了"）
i=0
while [ $i -lt 40 ]; do
  # ⚠️ 线 A 的端口**来自配置**（`--addr` 被忽略）⇒ 必须**从日志读**，不能假定 $PORT
  RP=$(grep -oE 'listening on [0-9.]+:[0-9]+' "$LOG" 2>/dev/null | tail -1 | grep -oE '[0-9]+$')
  if [ -n "$RP" ]; then
    if [ "$RP" != "$PORT" ]; then
      echo "[calib-R] ⚠️ 配置端口 ${PORT}，实际监听 **${RP}**（--addr 被忽略）⇒ 用实际端口"
      PORT="$RP"
    fi
    break
  fi
  i=$((i+1)); sleep 0.25
done
if [ $i -ge 40 ]; then
  echo "[calib-R] ❌ 服务未就绪（40×0.25s）⇒ 无 R 证据。服务日志尾部："
  tail -8 "$LOG" | sed 's/^/         /'
  exit 2
fi
echo "[calib-R]    服务就绪（${i}×0.25s）"
# 起后自证：该端口只应有 1 个监听者（就是我刚起的 ${SRV}）
nlisten=$(lsof -nP -iTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | tail -n +2 | wc -l | tr -d ' ')
echo "[calib-R]    端口 $PORT 监听者数 = ${nlisten}（应为 1；我起的 PID=${SRV}）"
if [ "$nlisten" -gt 1 ]; then
  echo "[calib-R] ❌ 端口 $PORT 有多个监听者 ⇒ **响应未必来自我起的进程** ⇒ 拒绝取证"
  exit 2
fi

{
  echo "# R 通道运行证据（run_calibration.sh）"
  echo
  echo "> ⚠️ 本文件**只含 R 证据**。按 \`skills/validate-align/SKILL.md\`："
  echo "> **PASS = C + R 双证齐**；只有 R 无 C ⇒ **不是 PASS**。"
  echo
  echo "**参考系与真装配声明**："
  echo '```'
  echo "采集时间   : $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "HEAD       : $(git -C "$ROOT" rev-parse HEAD 2>/dev/null || echo '?')"
  echo "origin/main: $(git -C "$ROOT" rev-parse origin/main 2>/dev/null || echo '?')"
  echo "二进制     : 真 build（go build ./cmd/vhs-asr）⇒ **真装配，非桩**"
  echo "服务地址   : 127.0.0.1:$PORT"
  echo "L2 配置    : ${VHS_MODEL_CENTER}"
  echo "⚠️ 默认钉规则式 ⇒ **本次 R 证据只覆盖规则式路径**（L2/模型路径未真跑）"
  echo '```'
  echo
  echo "| 判据 | 对象 | 请求 | HTTP | 运行证据（响应片段） | 判定 |"
  echo "|---|---|---|---|---|---|"
} > "$OUT"

grep -vE '^\s*(#|$)' "$LIST" > "$DATA/.crit" 2>/dev/null || true
while IFS= read -r line; do
  id=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$1); print $1}')
  obj=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$2); print $2}')
  m=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$3); print toupper($3)}')
  p=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$4); print $4}')
  want=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$5); print $5}')
  body=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$6); print $6}')
  [ -z "$body" ] && body='{}'   # 缺省空对象；端点若要求字段，判据里必须显式写
  pre=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$7); print $7}')
  # 前提判定：**按实际状态判**，不靠人（否则又会把"前提"写成想当然）
  PRE_OK=1
  case "$pre" in
    *线B不可达*|*线B.*不可达*)
      curl -sf -m 2 "http://127.0.0.1:$ASR_PORT/v1/health" >/dev/null 2>&1 && PRE_OK=0 ;;
    *线B可用*|*线B.*可用*)
      curl -sf -m 2 "http://127.0.0.1:$ASR_PORT/v1/health" >/dev/null 2>&1 || PRE_OK=0 ;;
  esac
  # ⚠️ **禁止空期望**（2026-10-03 实测事故）：
  # 我曾用 `cmd/vhs-asr` 打 `/v1/voice` 得到 **200**，就当成 PASS ——
  # 而那个 200 是**测试页的 200**（`<!doctype html>`），**不是 `handleVoice` 的 200**。
  # ⇒ **"HTTP 200" ≠ "走到了那个 handler"** ⇒ R 证据必须断言**响应内容**（契约字段）。
  if [ -z "$want" ]; then
    echo "| $id | $obj | \`$m $p\` | - | **（清单未写期望子串）** | **✗ 判据无效**（R 证据必须断言响应内容，不能只验状态码） |" >> "$OUT"
    continue
  fi

  resp=$(curl -s -m 30 -X "$m" "http://127.0.0.1:$PORT$p" -H 'Content-Type: application/json' -d "$body" -w '\n%{http_code}' 2>/dev/null)
  code=$(printf '%s' "$resp" | tail -1)
  # ⚠️ **断言必须用完整响应，截断只用于显示**（2026-10-03 实测第 18 条）：
  # 我第一版先 `cut -c1-120` 再 grep ⇒ `intent_source` 在 120 字符之后 ⇒ **永远匹配不到 ⇒ 假红**。
  # ⇒ 判据不得在"被自己截断过的"数据上断言。
  full=$(printf '%s' "$resp" | sed '$d' | tr -d '\n')
  rbody=$(printf '%s' "$full" | cut -c1-120)

  # ⚠️ **前提不满足 ⇒ 三态，不判 ✗**（否则会把"装置前提"误报成"产品红"）
  if [ -n "$pre" ] && [ "$PRE_OK" != "1" ]; then
    echo "| $id | $obj | \`$m $p\` | - | **（前提「${pre}」不满足）** | **⚠️ 前提不满足**（不算 ✗ 也不算 ✅） |" >> "$OUT"
    continue
  fi
  if printf '%s' "$full" | grep -qF -- "$want"; then
    echo "| $id | $obj | \`$m $p\` | $code | \`$rbody\` | **R ✅**（C 待补 ⇒ ◐） |" >> "$OUT"
  else
    echo "| $id | $obj | \`$m $p\` | $code | \`$rbody\` | **✗ 期望 \`$want\` 未出现** |" >> "$OUT"
  fi
done < "$DATA/.crit"

R=$(grep -c 'R ✅' "$OUT" 2>/dev/null || echo 0)
# ⚠️ 失败计数必须覆盖**所有** ✗ 类别 —— 我第一版只数 `✗ 期望`，
# 于是新加的 `✗ 无效判据` **不被计入** ⇒ 退出码错误地为 0（2026-10-03 实测）
# ⚠️ **精确计数**（2026-10-03 第 19 条）：`grep -c '✗ '` 会把 ⚠️ 行里的
# 「不算 ✗ 也不算 ✅」这句**说明文字**也数进去 ⇒ 统计多报 ✗。
# ⇒ 只数**判定格**（`| **✗ …` / `| **⚠️ …`），不数叙述里的符号。
Z=$(grep -c '| \*\*✗ ' "$OUT" 2>/dev/null || echo 0)
W=$(grep -c '| \*\*⚠️ 前提不满足' "$OUT" 2>/dev/null || echo 0)
T=$(grep -E '^\| [A-Za-z0-9_-]+ \|' "$OUT" 2>/dev/null | grep -vc '^| 判据 ' || echo 0)
{
  echo
  echo "## 统计"
  echo '```'
  echo "判据总数   : $T"
  echo "有 R 证据  : $R"
  echo "✗          : $Z"
  echo "⚠️ 前提不满足 : $W   ← **不算 ✗ 也不算 ✅**（装置前提，不是产品结论）"
  echo "⇒ 按 skill：**PASS = C + R 双证齐**。有 R 证据仍须补 C 证据（scripts/evidence_collect.sh）。"
  echo '```'
  echo
  echo "**服务日志尾部**（运行证据的一部分）："
  echo '```'
  tail -12 "$LOG" 2>/dev/null || echo '(无日志)'
  echo '```'
} >> "$OUT"

cat "$OUT"
echo
echo "[calib-R] 已写 $OUT"
[ "$Z" -gt 0 ] && { echo "[calib-R] ❌ $Z 条判据 ✗"; exit 1; }
exit 0
