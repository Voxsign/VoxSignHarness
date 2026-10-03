#!/bin/sh
# preflight.sh —— **写操作之前**先核参考系。
#
# ⚠️ 为什么需要它（2026-10-03 差点出的事故）：
#   我在**主仓**（`<…>/voicesign-harness`）而不是工作 worktree（`~/vhs-fix-wt`）里做台账编辑。
#   主仓 `On branch main` 且**与 origin/main 分叉**（1 vs 267 commits），
#   工作区里还有**其他作者的 51 项未提交改动**。
#   ⇒ `git commit` 失败（`error: Please commit or stash them.`）
#   ⇒ **那次失败是好事**：若它成功，我就会**把别人的 51 项未提交工作一起提交**。
#
# ⇒ 教训（第 26 条）：**`cd` 到仓库根 ≠ "在对的 worktree 里"。**
#   本项目有 ≥4 个副本（`~/VoxSign` · `~/voicesign-harness` · 主仓 · `~/vhs-live` · `~/vhs-fix-wt`），
#   且**主仓长期有别人的活**。
# ⇒ 而"下次小心"不是修法（今天反复量到：写进文档对行为没有约束力）
#   ⇒ **把它变成一条命令，写之前先跑。**
#
# 用法：  sh scripts/preflight.sh              # 核当前目录
#         sh scripts/preflight.sh --expect-clean   # 要求"无未提交改动"（提交前用）
#
# 退出码：0 = 可以写；1 = **不要写**（有风险，逐条说明）；2 = 装置不可用
set -u

EXPECT_CLEAN=0
[ "${1:-}" = "--expect-clean" ] && EXPECT_CLEAN=1

command -v git >/dev/null 2>&1 || { echo "[preflight] ❌ 无 git"; exit 2; }
git rev-parse --git-dir >/dev/null 2>&1 || { echo "[preflight] ❌ 不在 git 仓里"; exit 2; }

TOP=$(git rev-parse --show-toplevel 2>/dev/null)
BR=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)
SHA=$(git rev-parse --short HEAD 2>/dev/null)
DIRTY=$(git status --porcelain 2>/dev/null | wc -l | tr -d ' ')
UNPUSHED=$(git rev-list --count '@{u}..HEAD' 2>/dev/null || echo "?")

echo "[preflight] 参考系"
echo "  仓根        : ${TOP}"
echo "  分支/HEAD   : ${BR}  ${SHA}"
echo "  未提交改动  : ${DIRTY} 项"
echo "  未推送提交  : ${UNPUSHED}"

rc=0

# ① detached HEAD 是**正常的**（本项目用 detached worktree 推 main）⇒ 只提示
[ "$BR" = "HEAD" ] && echo "  ℹ️  detached HEAD（本项目常用；推之前必须用**显式 SHA**，不用 HEAD）"

# ② 主仓识别：若路径就是长期主仓，且改动多 ⇒ 很可能是"别人的活"
case "$TOP" in
  */DoubaoWork/*/voicesign-harness)
    echo "  ⚠️ **这是主仓**（长期有别人的未提交工作）"
    if [ "$DIRTY" -gt 0 ]; then
      echo "  ❌ 主仓有 ${DIRTY} 项未提交改动 ⇒ **不要在这里提交**（会带上别人的工作）"
      echo "     ⇒ 改到你的 worktree：cd ~/vhs-fix-wt"
      rc=1
    fi ;;
esac

# ③ --expect-clean：提交前要求工作区干净（或只有你**明确**要提交的文件）
if [ "$EXPECT_CLEAN" = "1" ] && [ "$DIRTY" -gt 0 ]; then
  echo "  ⚠️ 工作区有 ${DIRTY} 项改动（若这些**全部**是你这轮改的，可继续；否则**停**）"
  git status --porcelain 2>/dev/null | head -8 | sed 's/^/       /'
  [ "$DIRTY" -gt 20 ] && { echo "  ❌ ${DIRTY} 项过多 ⇒ 极可能混入别人的工作 ⇒ **停**"; rc=1; }
fi

# ④ 上游分叉（1 vs 267 那次就是分叉）
if [ "$UNPUSHED" != "?" ] && [ "$UNPUSHED" -gt 20 ] 2>/dev/null; then
  echo "  ⚠️ 本地领先上游 ${UNPUSHED} 个提交 ⇒ 可能长期未同步/分叉 ⇒ **核清再写**"
fi

if [ "$rc" = "0" ]; then echo "[preflight] ✅ 可以写"; else echo "[preflight] ❌ **不要写**（见上）"; fi
exit $rc
