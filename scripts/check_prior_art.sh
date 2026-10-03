#!/bin/sh
# check_prior_art.sh —— 写「未核 / 没有 / 不存在」之前，**先查既有结论**。
#
# ⚠️ 为什么需要它（2026-10-03 · 同一天犯三次）
#   ① 我"重新发现"了 Peter 校准报告里已写过的**至少四件事**（性能口径 · 能力层 0% · skill 零接线 · NF-5 修法）
#   ② 我把 `skill/` 零接线当"新发现的缺口" ⇒ 而 `docs/测试方案-线C-技能层.md` **已登记为待办（责任人：我）**
#   ③ 我提了「给写类意图默认落 project」的方案 ⇒ 而 `docs/research-inputs/13-Codex-域评审.md`（**17 行**）
#      **明确禁止**「退回更宽权限」⇒ **裁决早已存在，我的方案违背它**
#
#   ⇒ 共同点：**我把"我没想到"当成了"不存在"，然后据此提方案。**
#   ⇒ 而"下次小心"没用（今天反复量到：写进文档对行为没有约束力）
#   ⇒ **做成一条命令：写结论之前先跑它。**
#
# 用法：  sh scripts/check_prior_art.sh 默认域 权限
#         sh scripts/check_prior_art.sh --strict 关键词   # 有命中即退出码 1（用于卡住）
#
# 退出码：0 = 无既有结论（可以写"未核/不存在"）；1 = **有命中，必须先读**；2 = 装置不可用
set -u

STRICT=0
[ "${1:-}" = "--strict" ] && { STRICT=1; shift; }

if [ $# -eq 0 ]; then
  echo "用法：sh scripts/check_prior_art.sh [--strict] <关键词> [关键词2 ...]"
  echo "  在写「未核 / 没有 / 不存在 / 首次」之前跑它，避免重复发现既有结论。"
  exit 2
fi

ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || { echo "[prior-art] ❌ 不在 git 仓里"; exit 2; }
cd "$ROOT" || exit 2

TOTAL=0
for kw in "$@"; do
  echo "[prior-art] 关键词：${kw}"
  # 搜文档与任务卡（**排除代码** —— 代码要读，但"既有结论"主要在这些地方）
  HITS=$(git grep -ln -- "$kw" -- 'docs/*.md' 'tasks/*.md' 'skills/*.md' '*.md' 2>/dev/null | sort -u)
  # ⚠️ `grep -c` 无匹配时**退出码为 1** ⇒ `|| echo 0` 会**再追加一个 0** ⇒ n="0\n0" ⇒ 判断失真。
  #    ⇒ 用 `wc -l`（无此问题），并对空串单独处理。
  n=0
  if [ -n "$HITS" ]; then
    n=$(printf '%s\n' "$HITS" | grep -c . 2>/dev/null)
    n=${n:-0}
  fi
  TOTAL=$((TOTAL + n))
  if [ "$n" = "0" ]; then
    echo "  ✅ 无命中 ⇒ 可以写「未核 / 不存在」（但**仍建议搜代码**：git grep -n \"$kw\" -- '*.go'）"
  else
    echo "  ⚠️ **$n 处命中** ⇒ **必须先读再写**："
    printf '%s\n' "$HITS" | head -10 | sed 's/^/       /'
    [ "$n" -gt 10 ] && echo "       …（还有 $((n-10)) 处）"
    # 给出每处命中的**上下文行**（只看前 3 处，避免刷屏）
    printf '%s\n' "$HITS" | head -3 | while IFS= read -r f; do
      [ -n "$f" ] || continue
      echo "     --- $f"
      git grep -n -- "$kw" -- "$f" 2>/dev/null | head -3 | sed 's/^/         /'
    done
  fi
  echo
done

echo "[prior-art] ---- 合计命中 ${TOTAL} 处"
if [ "$TOTAL" -gt 0 ]; then
  echo "[prior-art] ⚠️ **有既有结论 ⇒ 先读它们，再决定是否说"未核/不存在"**"
  [ "$STRICT" = "1" ] && exit 1
fi
exit 0
