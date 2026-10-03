#!/bin/sh
# evidence_collect.sh —— **C 通道：代码证据采集**（`skills/validate-align/SKILL.md` v2 承诺的脚本之一）。
#
# 依据（skill 原文）：
#   | **C 代码校准** | 判据 → 实现代码真值：存在性/结构/接线 | git grep + Read 源码：接口、结构、调用链 | 每条判据的代码证据（文件:行） |
#   | 评分规则 | 每条判据 PASS = **C 证据 + R 证据双证齐**；只 C 无 R = ◐ 待真跑；只有文档 = ✗ |
#
# 本脚本**只产 C 证据**。按 skill：**单靠本脚本不得给 PASS**（必须与 R 通道双证）。
#
# 用法：
#   sh scripts/evidence_collect.sh <判据清单文件>
#
# 判据清单格式（每行一条，`|` 分隔；`#` 开头为注释）：
#   <判据ID> | <要证明的事实> | <匹配模式> [| <限定路径>]
#
# 例：
#   PL-1 | 规划器有 L2 分支 | L2Enabled | plan/
#   GATE-1 | 门禁不吞退出码 | go test -tags | scripts/gate.sh
#
# 输出：每条判据 → grep 命中（文件:行:内容）；**0 命中 ⇒ ✗**（该判据无 C 证据）
# 退出码：0 = 每条判据都至少 1 处 C 证据；1 = 有判据 0 命中

set -u

if [ $# -lt 1 ] || [ ! -f "$1" ]; then
  echo "用法: sh scripts/evidence_collect.sh <判据清单文件>"
  echo "格式: <判据ID> | <要证明的事实> | <匹配模式> [| <限定路径>]"
  exit 2
fi

LIST="$1"
ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
HEAD_SHA="$(git rev-parse HEAD 2>/dev/null || echo '?')"
OUT=".calib-evidence-C.md"

# ⚠️ skill 要求"证据可追溯" ⇒ 必须记录**采集时的参考系**（否则证据属于哪个版本无从判断）
{
  echo "# C 通道代码证据（evidence_collect.sh）"
  echo
  echo "> ⚠️ 本文件**只含 C 证据**。按 \`skills/validate-align/SKILL.md\`："
  echo "> **PASS = C + R 双证齐**；只有 C ⇒ **◐ 待真跑**；只有文档 ⇒ **✗**。"
  echo "> **单靠本文件不得给 PASS。**"
  echo
  echo "**参考系**（证据属于哪个版本 —— 缺此则证据不可追溯）："
  echo '```'
  echo "采集时间 : $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "仓库根   : $ROOT"
  echo "HEAD     : $HEAD_SHA"
  echo "分支     : $(git symbolic-ref -q --short HEAD 2>/dev/null || echo '(detached)')"
  echo "远端     : $(git remote get-url origin 2>/dev/null || echo '(无)')"
  echo "⚠️ 本地 HEAD ≠ 远端 tip 时，本证据**不代表远端内容**"
  echo "   本地 HEAD   = $HEAD_SHA"
  echo "   origin/main = $(git rev-parse origin/main 2>/dev/null || echo '?')"
  echo '```'
  echo
  echo "| 判据 | 要证明的事实 | 匹配模式 | C 证据（文件:行） | 判定 |"
  echo "|---|---|---|---|---|"
} > "$OUT"

rc=0
total=0
with_evidence=0

# 逐条处理（跳过注释与空行）
grep -vE '^\s*(#|$)' "$LIST" | while IFS= read -r line; do
  id=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$1); print $1}')
  fact=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$2); print $2}')
  pat=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$3); print $3}')
  path=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$4); print $4}')
  [ -z "$path" ] && path="."

  hits=$(git -C "$ROOT" grep -nE -- "$pat" -- "$path" 2>/dev/null | head -5)
  n=$(printf '%s' "$hits" | grep -c . 2>/dev/null || echo 0)

  if [ "$n" -gt 0 ]; then
    # 取前 3 处，压成一行（文件:行）
    loc=$(printf '%s\n' "$hits" | head -3 | awk -F: '{print $1":"$2}' | tr '\n' ' ')
    echo "| $id | $fact | \`$pat\` | $loc | **C ✅**（R 待补 ⇒ ◐） |" >> "$OUT"
  else
    echo "| $id | $fact | \`$pat\` | **（0 命中）** | **✗ 无 C 证据** |" >> "$OUT"
    rc=1
  fi
done

# ⚠️ 上面的 while 在子 shell 里，rc 传不出来 ⇒ 用文件计数重算（不使用管道读 rc）
# ⚠️ 计数必须排除表头行（`| 判据 | 要证明的事实 |…`）—— 我第一版没排除，总数多算了 1
T=$(grep -E '^\| [^|]+ \|' "$OUT" 2>/dev/null | grep -vc '^| 判据 ' || echo 0)
Z=$(grep -c '0 命中' "$OUT" 2>/dev/null || echo 0)
C=$(grep -c 'C ✅' "$OUT" 2>/dev/null || echo 0)
{
  echo
  echo "## 统计"
  echo '```'
  echo "判据总数     : $T"
  echo "有 C 证据    : $C"
  echo "无 C 证据(✗) : $Z"
  echo "⇒ 按 skill：**有 C 证据的判据仍是 ◐（待真跑）**，须补 R 证据（scripts/run_calibration.sh）才可能 PASS。"
  echo '```'
} >> "$OUT"

cat "$OUT"
echo
echo "[evidence_collect] 已写 $OUT"
echo "[evidence_collect] ⚠️ 只有 C 证据 ⇒ 全部为 **◐ 待真跑**，**不得据此给 PASS**（skills/validate-align/SKILL.md）"
[ "$Z" -gt 0 ] && { echo "[evidence_collect] ❌ 有 $Z 条判据 0 命中（✗ 无 C 证据）"; exit 1; }
exit 0
