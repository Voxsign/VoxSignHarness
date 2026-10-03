#!/bin/sh
# calib.sh —— **校准**（Peter 2026-10-03 定的核心技能）。
#
# 为什么存在：
#   2026-10-03 一天内，本项目犯了 7 次错（我 6 + 实现方 1），全部撤回。
#   而形状只有两个：
#     ① **我读的东西不是我以为的那个**（读数错）
#     ② **我改的东西改变了我要观测的东西**（观察者效应）
#   ⇒ 两条都是**校准失败**。而代价是：`main` 红了五轮，**没人知道为什么**。
#
# 本脚本做的事：**在动手前后，产出一份「参考系声明」并检查它。**
#   Peter：「校准先于测试，先于改。校准甚至可以在一边干的过程一边校准。」
#
# 用法：
#   sh scripts/calib.sh before   # 动手前：声明参考系 + 自检
#   sh scripts/calib.sh after    # 动手后：**与 before 对比**（= 同一条命令内前后对比的最强形式）
#   sh scripts/calib.sh diff     # 直接对比 .calib-before / .calib-after
#
# 退出码：0 = 一致/正常；非 0 = **参考系在两次之间变了**（这就是"观察者效应"）

set -u

MODE="${1:-before}"
OUT=".calib-${MODE}"

# ---------- 1. 参考系声明 ----------
{
  echo "# 参考系声明（calib.sh ${MODE}）"
  echo "time        : $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "pwd         : $(pwd)"
  echo "toplevel    : $(git rev-parse --show-toplevel 2>/dev/null || echo '(非 git 仓库)')"
  echo "git-dir     : $(git rev-parse --git-dir 2>/dev/null || echo '?')"
  echo "common-dir  : $(git rev-parse --git-common-dir 2>/dev/null || echo '?')"
  echo "remote      : $(git remote get-url origin 2>/dev/null || echo '(无 origin)')"
  echo "HEAD        : $(git rev-parse HEAD 2>/dev/null || echo '?')"
  echo "branch      : $(git symbolic-ref -q --short HEAD 2>/dev/null || echo '(detached)')"
  echo "reflog_len  : $(git reflog show --format=%H 2>/dev/null | wc -l | tr -d ' ')"
  echo "status_md5  : $(git status --porcelain 2>/dev/null | md5sum 2>/dev/null || git status --porcelain 2>/dev/null | md5)"
  echo "status_lines: $(git status --porcelain 2>/dev/null | wc -l | tr -d ' ')"
  # ⚠️ 配置用 --show-origin（**md5 不够** —— 今天证明它不反映真值来源）
  echo "user.email  : $(git config --show-origin user.email 2>/dev/null || echo '(未设置)')"
  echo "user.name   : $(git config --show-origin user.name 2>/dev/null || echo '(未设置)')"
  echo "core.bare   : $(git config --show-origin core.bare 2>/dev/null || echo '(未设置 ⇒ 默认 false)')"
  # **谁可能在改它**
  echo "worktrees   : $(git worktree list 2>/dev/null | wc -l | tr -d ' ') 个"
  echo "writers     : $(ps aux 2>/dev/null | grep -E 'go test|gate\.sh|vhs-' | grep -v grep | wc -l | tr -d ' ') 个疑似进程"
} > "$OUT" 2>&1

# ---------- 2. 输出声明 ----------
echo "=== 参考系声明（已写 ${OUT}）==="
sed 's/^/  /' "$OUT"

# ---------- 3. 自检（硬红线）----------
rc=0
bare=$(git config --get core.bare 2>/dev/null || echo "false")
if [ "$bare" = "true" ]; then
  echo "[calib] ❌ **core.bare=true** —— 仓库被当裸库 ⇒ worktree/status/commit 语义全部异常"
  echo "        （2026-10-03 真实发生过：「47 文件全部 untracked」、worktree 解析异常）"
  rc=1
fi
email=$(git config --get user.email 2>/dev/null || echo "")
case "$email" in
  *vhs@test*|*vhs-test*|*vhs-arch*)
    echo "[calib] ❌ **user.email 是测试身份（$email）** —— 真实身份被测试覆盖了"
    echo "        修：git config --unset user.email（回落全局）"
    rc=1;;
esac

# ---------- 4. diff 模式：与 before 对比 ----------
if [ "$MODE" = "after" ] || [ "$MODE" = "diff" ]; then
  if [ -f .calib-before ] && [ -f .calib-after ]; then
    echo
    echo "=== ⭐ 前后对比（**同一条命令内前后对比 —— 最强形式的证据**）==="
    # 只比"参考系身份"，不比 time（time 必然不同）
    # ⚠️ 不用进程替换 `<(…)` —— 那是 **bash 扩展，POSIX sh 不支持**（本脚本首行是 #!/bin/sh）
    # ⚠️ 必须排除"本来就该不同"的行 —— 否则 diff **恒红**（我第一版就犯了这个错）
    #    · `time`  必然不同
    #    · 标题行含 mode（before/after）必然不同
    grep -vE '^time |^# 参考系声明' .calib-before > /tmp/.calib-b.$$
    grep -vE '^time |^# 参考系声明' .calib-after  > /tmp/.calib-a.$$
    if diff /tmp/.calib-b.$$ /tmp/.calib-a.$$ > /tmp/.calib-diff.$$ 2>&1; then
      echo "  ✅ **参考系未变**（HEAD / status / 配置 / reflog 全部一致）"
    else
      echo "  ❌ **参考系变了** —— 动手这件事改变了被观测对象："
      sed 's/^/    /' /tmp/.calib-diff.$$
      echo
      echo "  ⇒ 结论：**本次动作的结论不可作为"关于原对象"的证据**（观察者效应）"
      echo "  ⇒ 处置：先查明是谁改的（CA-3 归因），再重做"
      rc=2
    fi
    rm -f /tmp/.calib-b.$$ /tmp/.calib-a.$$ /tmp/.calib-diff.$$
  else
    echo "[calib] ⚠️ 缺 .calib-before 或 .calib-after ⇒ 无法对比"
    echo "        用法：sh scripts/calib.sh before → 干活 → sh scripts/calib.sh after"
    rc=3
  fi
fi

# ---------- 5. 参考系依赖提醒 ----------
echo
echo "=== 参考系依赖（**动手前请逐条回答**，这是本技能的核心产物）==="
cat <<'EOF'
  我在哪          : （见上 pwd / toplevel）
  它是哪个仓库     : （见上 remote + HEAD）
  谁可能在改它     : （见上 worktrees / writers —— **别假设只有你一个**）
  我的判据依赖什么 : ❓ **请写下来** —— 这个参考系里"必须恰好有什么"？
                    例：某事依赖"全局 git 身份"、"某个 commit 可达"、"有缓存"、
                        "端口空闲"、"上一个测试留下的状态"
  不成立时怎么办   : ❓ **请写下来** —— SKIP 并显式报告 / 换参考系 / 停止
                    ⚠️ **不得把 SKIP 当通过**（docs/LHT-0002）
EOF
echo
echo "  依据：2026-10-03 main 红了五轮 —— 真因是 SM-4 判据**没有声明**
        它依赖「真值基线 commit 可达」。本地可达⇒绿，CI 全新 clone⇒红，**而它不说原因**。"

exit $rc
# ---------- 6. 未确认项（如实标注，不掩盖）----------
# ⚠️ 本脚本的"反例退出码"未单独区分：身份红线（rc=1）与 diff 分支（rc=2）
#    在实测中前者先触发 ⇒ **diff 分支是否也能独立触发，未单独确认**。
#    ⇒ 按本项目纪律：未确认的路径不得当作已验证。
