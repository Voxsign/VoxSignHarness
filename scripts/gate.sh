#!/bin/sh
# gate.sh —— 统一门禁（pre-push hook 与 CI 共用同一入口，避免"两套门禁"漂移）。
#
# 为什么存在：有一次我推了 build 断掉的提交，因为"我以为推成功了"。
# 这是纪律防不住的坑 —— 必须由机制挡（见 docs：等价是假设）。
set -e
# ============ 校准（CA-1：跑门禁不得改变被验证对象）============
# 2026-10-03：本项目一天内 7 次错、全部撤回，形状只有两个 —— 读数错 / 观察者效应。
# 而 `core.bare` 被置 true 与"钩子里的 gate 为何失败"**至今未归因**。
# ⇒ 门禁必须能自证"它没有改变被它验证的东西"（Peter：「校准先于测试，先于改」）。
#
# ✅ **两侧都已反证过**（2026-10-03）：
#   · 正例：不改任何东西 ⇒ 退出码 0 · "[gate] 校准 ✅ 均一致"
#   · 反例：在门禁中途注入 `git config core.bare true` ⇒ 退出码非 0 ·
#           "[gate] ❌ 门禁改变了被验证对象" + **差异行（指明是哪一项变了）**
#   ⇒ 这是本项目第一条满足"机制闭环三条"（①现象 ②路径 ③反证）的判据。
#   ⇒ 而差异行 = **CA-3（归因可分）** 的第一块砖：它不只说"变了"，它说"哪一项变了"。
CALIB_BEFORE=$(mktemp)
CALIB_AFTER=$(mktemp)
calib() {  # $1=输出文件
  # ⚠️ **每条命令都必须 `set -e` 安全**（2026-10-03 CI 事故）：
  # 本脚本头上有 `set -e`，而下面几条命令在 CI（全新 clone / 配置未设置）会返回非 0
  # ⇒ **整脚本在打印任何 [gate] 之前就 exit 1** —— 本地（有配置）永远不会复现。
  # ⇒ 每条都 `|| echo '(未设置/不可用)'`，**失败只影响该行内容，不影响脚本退出码**。
  {
    git rev-parse HEAD 2>/dev/null || echo '(无 HEAD)'
    git status --porcelain 2>/dev/null | md5sum 2>/dev/null || git status --porcelain 2>/dev/null | md5 || echo '(无 status)'
    git reflog show --format=%H 2>/dev/null | wc -l || echo '0'
    git config --show-origin user.email 2>/dev/null || echo '(未设置)'
    git config --show-origin core.bare 2>/dev/null || echo '(未设置 ⇒ 默认 false)'
  } > "$1" 2>&1 || true
}
calib "$CALIB_BEFORE"
echo "[gate] 校准快照已取（before）"

echo "[gate] go build ./..."
go build ./...
echo "[gate] go vet ./..."
go vet ./...
echo "[gate] tagged 判据：编译 + vet + **真跑**（默认门禁看不到 tag 文件 —— 曾经的盲区）"
# 教训：只编译不跑 ⇒ 一条 tagged 判据变红也能被推出去。**必须真跑。**
# ⚠️ **不许吞输出**（2026-10-03 事故）：原先两句都带 `>/dev/null`，
# 于是门禁失败时**什么都不告诉你** —— CI 与 pre-push 都只打印到这一行就 exit 1，
# 失败原因从此不可见（这条"静默失败"在本项目出现第 4 次：
# 测试页 → LLM 归因 → 判据空过 → **门禁自己**）。
# ⇒ 改为：**成功才安静，失败必须把是哪个 tag、哪一步、完整输出都打出来**。
for t in asrharness vhs002 vhsui vhsext vhsplan vhsplanmodel vhswm vhscache vhsrecog vhsreal vhsroute vhsref; do
  if ! out=$(go vet -tags "$t" ./... 2>&1); then
    echo "[gate] ❌ go vet -tags $t 失败："
    echo "$out"
    exit 1
  fi
  if ! out=$(go test -tags "$t" ./... 2>&1); then
    echo "[gate] ❌ go test -tags $t 失败："
    echo "$out"
    exit 1
  fi
done
echo "[gate] go test ./... (默认门禁)"
go test ./...

# ============ 校准比对（CA-1/CA-2：门禁须能自证它没改被验证对象）============
calib "$CALIB_AFTER"
if ! diff "$CALIB_BEFORE" "$CALIB_AFTER" > /tmp/.gate-calib-diff.$$ 2>&1; then
  echo "[gate] ❌ **门禁改变了被验证对象**（观察者效应）—— 本次结论不可作为\"关于原对象\"的证据："
  sed 's/^/       /' /tmp/.gate-calib-diff.$$
  echo "       ⇒ 若要归因，看上面的差异行：哪一项变了，就去查谁改的。"
  rm -f "$CALIB_BEFORE" "$CALIB_AFTER" /tmp/.gate-calib-diff.$$
  exit 1
fi
rm -f "$CALIB_BEFORE" "$CALIB_AFTER" /tmp/.gate-calib-diff.$$
echo "[gate] 校准 ✅ 门禁未改变被验证对象（HEAD / status / reflog / 身份 / bare 均一致）"

# ============ 规范与代码对齐（2026-10-03 加）============
# ⚠️ 为什么加：`docs/SPEC-v2` §4.1 的 space_check 伪代码**把四步判定简化成一句**，
#   而实际代码是逐项判定 ⇒ 我据简化伪代码推理 ⇒ 把 `boundary_violation` 读成"越界"
#   （实为"工具未授权"）⇒ **追错 10 轮**。
# ⇒ 教训不只是"要读代码"，而是：**建好的检查必须真的跑** ——
#   "写进文档没有约束力" 的升级版是 "**写进工具但不去跑，同样没有约束力**"。
if [ -x scripts/spec_code_align.sh ] || [ -f scripts/spec_code_align.sh ]; then
  if ! out=$(sh scripts/spec_code_align.sh 2>&1); then
    echo "[gate] ❌ **规范与代码不对齐** ⇒ 拿规范当尺子会漏判："
    printf '%s\n' "$out" | sed 's/^/       /'
    echo "       ⇒ 处置：把缺的判定补进 docs/SPEC-v2（或修代码使其一致）"
    exit 1
  fi
  echo "[gate] 规范对齐 ✅ $(printf '%s\n' "$out" | tail -1)"
else
  echo "[gate] ⚠️ 未找到 scripts/spec_code_align.sh ⇒ **跳过**（不代表通过）"
fi

echo "[gate] OK"
