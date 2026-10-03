#!/bin/sh
# evidence_collect.sh —— **C 通道：代码证据采集**（`skills/validate-align/SKILL.md` v2 承诺的脚本之一）。
#
# 依据（skill 原文）：
#   ⚠️ **C 通道只认代码**：三类都不算 C 证据（2026-10-03 实测，第 16/23 条）：
#      ① **文档**（`.md/.txt/.json/.yaml`）
#      ② **采集器自身**（自匹配）
#      ③ ⚠️ **注释行**（`// 绝不执行` 之类）—— 命中注释只证"提过"，**不证"实现了"**
#      ⇒ 而"声明/登记/校验"也算不到"写入"（第 16 条）—— **命中 ≠ 发生**。
#   | **C 代码校准** | 判据 → 实现代码真值：存在性/结构/接线 | git grep + Read 源码：接口、结构、调用链 | 每条判据的代码证据（文件:行） |
#   | 评分规则 | 每条判据 PASS = **C 证据 + R 证据双证齐**；只 C 无 R = ◐ 待真跑；只有文档 = ✗ |
#
# 本脚本**只产 C 证据**。按 skill：**单靠本脚本不得给 PASS**（必须与 R 通道双证）。
#
# 用法：
#   sh scripts/evidence_collect.sh <判据清单文件>
#
# 判据清单格式（每行一条，`|` 分隔；`#` 开头为注释）：
#   <判据ID> | <要证明的事实> | <匹配模式> [| <限定路径>] [| <别名模式（可选，同义词/实现名）>]
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

  # ⚠️ **C 通道 = 代码真值 ⇒ 必须排除文档**（2026-10-03 实测发现）：
  # 第一次真跑时，V-02/V-04 的**首条**命中是 `tasks/VHS-VOICE-001-*.md`（规格文档本身）。
  # ⚠️ 但**修不修这条排除，都不影响那两条的判定** —— 它们在 `trajectory/kinds.go:20/21`
  #    有**真实现**（`IntentSourceASR` / `IntentSourceTextFallback`）。
  #    ⇒ 排除规则失效是**真 bug**，但它**没有污染本次结论**（我一度误以为那两条是假阳性，那是错的）。
  # 而 skill 明写：**只有文档 = ✗**。⇒ 排除 *.md/*.txt/*.json/*.yaml 等非代码，
  #    除非调用方**显式**把路径限定为文档（那时由人负责语义）。
  # ⚠️ 排除正则必须同时匹配 `.md:` 与 `.md":` —— git grep 对含非 ASCII 的路径**会加引号**，
  # 只写 `\.md:` 会**静默失效**（2026-10-03 实测：排除规则写了但一条都没排掉）。
  # ⚠️ **必须排除采集器自身**（2026-10-03 实测）：脚本里的注释会包含模式词
  # （如 `RunWithIntent` 出现在解释"为什么 0 命中不许判 ✗"的注释里）
  # ⇒ 曾把 **scripts/evidence_collect.sh:92 自己**当成 C 证据 ⇒ **假阳性**。
  # ⇒ 这与"排除文档"是同一类：**取证工具不能把自己算作证据。**
  hits=$(git -C "$ROOT" grep -nE -- "$pat" -- "$path" 2>/dev/null \
           | grep -vE '\.(md|txt|rst|json|ya?ml|toml)[":]' \
           | grep -vE '^"?scripts/evidence_collect\.sh"?[:"]' \
           | grep -vE '^"?\.calib-evidence' \
           | grep -vE ':[0-9]+:[[:space:]]*(//|#|\*|/\*)' | head -5)
  n=$(printf '%s' "$hits" | grep -c . 2>/dev/null || echo 0)

  if [ "$n" -gt 0 ]; then
    # 取前 3 处，压成一行（文件:行）
    loc=$(printf '%s\n' "$hits" | head -3 | awk -F: '{print $1":"$2}' | tr '\n' ' ')
    echo "| $id | $fact | \`$pat\` | $loc | **C ✅**（R 待补 ⇒ ◐） |" >> "$OUT"
  else
    # ⚠️ **0 命中不得直接判 ✗**（2026-10-03 实测事故）：
    # 我曾按设计稿的符号名 `RunWithIntent` 搜 ⇒ 0 命中 ⇒ 判 V-01「未实现」。
    # 而实现用的名字是 `handleVoice`（server/server.go:183），端点 `/v1/voice` **真跑 200**。
    # ⇒ **把"命名不一致"误报成了"未实现"。**
    # ⇒ 故：先查该判据的**别名/同义词**；别名命中 ⇒ 标 ⚠️「命名未对上」，
    #    **不算 ✗，也不算 ✅** —— 它需要人来判"是不是同一个东西"。
    alias=$(echo "$line" | awk -F'|' '{gsub(/^ +| +$/,"",$5); print $5}')
    ahits=""
    if [ -n "$alias" ]; then
      ahits=$(git -C "$ROOT" grep -nE -- "$alias" -- "$path" 2>/dev/null \
                | grep -vE '\.(md|txt|rst|json|ya?ml|toml)[":]' \
                | grep -vE '^"?scripts/evidence_collect\.sh"?[:"]' \
                | grep -vE '^"?\.calib-evidence' \
                | grep -vE ':[0-9]+:[[:space:]]*(//|#|\*|/\*)' | head -3)
    fi
    if [ -n "$ahits" ]; then
      aloc=$(printf '%s\n' "$ahits" | awk -F: '{print $1":"$2}' | tr '\n' ' ')
      echo "| $id | $fact | \`$pat\` | 主模式 0 命中；**别名 \`$alias\` 命中**：$aloc | **⚠️ 命名未对上**（需人判是否同一语义） |" >> "$OUT"
    else
      echo "| $id | $fact | \`$pat\` | **（主模式 0 命中，别名亦无）** | **✗ 无 C 证据** |" >> "$OUT"
      rc=1
    fi
  fi
done

# ⚠️ 上面的 while 在子 shell 里，rc 传不出来 ⇒ 用文件计数重算（不使用管道读 rc）
# ⚠️ 计数必须排除表头行（`| 判据 | 要证明的事实 |…`）—— 我第一版没排除，总数多算了 1
T=$(grep -E '^\| [^|]+ \|' "$OUT" 2>/dev/null | grep -vc '^| 判据 ' || echo 0)
Z=$(grep -c '✗ 无 C 证据' "$OUT" 2>/dev/null || echo 0)
W=$(grep -c '⚠️ 命名未对上' "$OUT" 2>/dev/null || echo 0)
C=$(grep -c 'C ✅' "$OUT" 2>/dev/null || echo 0)
{
  echo
  echo "## 统计"
  echo '```'
  echo "判据总数     : $T"
  echo "有 C 证据    : $C"
  echo "无 C 证据(✗) : $Z"
  echo "⚠️ 命名未对上  : $W   ← **不算 ✗ 也不算 ✅**，需人判是否同一语义"
  echo "⇒ 按 skill：**有 C 证据的判据仍是 ◐（待真跑）**，须补 R 证据（scripts/run_calibration.sh）才可能 PASS。"
  echo '```'
} >> "$OUT"

cat "$OUT"
echo
echo "[evidence_collect] 已写 $OUT"
echo "[evidence_collect] ⚠️ 只有 C 证据 ⇒ 全部为 **◐ 待真跑**，**不得据此给 PASS**（skills/validate-align/SKILL.md）"
[ "$W" -gt 0 ] && echo "[evidence_collect] ⚠️ 有 $W 条判据「主模式 0 命中但有别名命中」—— **需人判是否同一语义**（不许当 ✗，也不许当 ✅）"
[ "$Z" -gt 0 ] && { echo "[evidence_collect] ❌ 有 $Z 条判据 0 命中（✗ 无 C 证据）"; exit 1; }
exit 0
