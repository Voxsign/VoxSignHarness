#!/bin/sh
# gate.sh —— 统一门禁（pre-push hook 与 CI 共用同一入口，避免"两套门禁"漂移）。
#
# 为什么存在：有一次我推了 build 断掉的提交，因为"我以为推成功了"。
# 这是纪律防不住的坑 —— 必须由机制挡（见 docs：等价是假设）。
set -e
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
echo "[gate] OK"
