#!/bin/sh
# timing_sensitive_scan.sh —— 找**依赖环境速度**的判据（今天 3 次 CI 红全部出自这一类）。
#
# ⚠️ 为什么需要它（2026-10-03 实测）：
#   今天 CI 红了 **3 次**，而**三次都是判据自身的问题，没有一次是产品缺陷**：
#     · `TestPerformanceP99` 红 2/5 次 —— 阈值 **1ms** 落在**尾部噪声带**（本地 p99 15–67µs，CI 1354–2057µs）
#     · `TestR04EvictionPrefersExpired` 红 1 次 —— **TTL 50ms** 与"塞 30 条"的**耗时同量级**
#   ⇒ 共同点：**判据把"环境速度"当成了常量。**
#   ⇒ 而这类缺陷**只在慢机器上暴露** ⇒ **本地永远绿** ⇒ **能一路通过本地 gate 直到 CI**。
#
# 用法：  sh scripts/timing_sensitive_scan.sh
# 退出码：0 = 未发现；1 = **发现可疑判据**（列出）；2 = 装置不可用
set -u
# ⚠️ 用 git rev-parse 而非 [ -d .git ]：**worktree 的 .git 是文件**（今天第 N 次踩到同类假设）
ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || { echo "[timing] ❌ 不在 git 仓里"; exit 2; }
cd "$ROOT" || exit 2

echo "[timing] 扫描「真实时钟 + 短时长」的判据（高风险：慢 CI 上行为不同）"
n=0

echo
echo "── ① 短 sleep（<200ms）—— 与"待测动作耗时"可能同量级"
git grep -nE "time\.Sleep\([0-9]+\s*\*\s*time\.(Milli|Micro)second" -- '*_test.go' 2>/dev/null \
  | sed 's/^/  /'
c=$(git grep -cE "time\.Sleep\([0-9]+\s*\*\s*time\.(Milli|Micro)second" -- '*_test.go' 2>/dev/null | wc -l | tr -d ' ')
n=$((n+c))

echo
echo "── ② 短 TTL（<1s）传给 Open/New —— 过期时刻取决于塞入耗时"
git grep -nE "(Open|New)\([^)]*[0-9]+\s*\*\s*time\.(Milli|Micro)second" -- '*_test.go' 2>/dev/null \
  | sed 's/^/  /'
c=$(git grep -cE "(Open|New)\([^)]*[0-9]+\s*\*\s*time\.(Milli|Micro)second" -- '*_test.go' 2>/dev/null | wc -l | tr -d ' ')
n=$((n+c))

echo
echo "── ③ 耗时断言用的绝对阈值（可能落在噪声带）"
git grep -nE "(p99|p50|Latency|Elapsed|Duration)\s*[<>]=?\s*[0-9]+\s*\*\s*time\.(Milli|Micro)second" -- '*_test.go' 2>/dev/null | sed 's/^/  /'
c=$(git grep -cE "(p99|p50|Latency|Elapsed|Duration)\s*[<>]=?\s*[0-9]+\s*\*\s*time\.(Milli|Micro)second" -- '*_test.go' 2>/dev/null | wc -l | tr -d ' ')
n=$((n+c))

echo
echo "[timing] ---- 命中文件数（去重前）：${n}"
if [ "$n" -gt 0 ]; then
  echo "[timing] ⚠️ **有可疑判据** ⇒ 逐个判断："
  echo "         · 它的**成败**是否会因机器慢而改变？"
  echo "         · 若会 ⇒ **放大余量到 2–5 个数量级**（如 TTL 50ms→2s · sleep 70ms→2.5s）"
  echo "         · 或**去掉真实时钟**（注入可控时钟 / 用显式触发代替 sleep）"
  exit 1
fi
echo "[timing] ✅ 未发现可疑判据"
