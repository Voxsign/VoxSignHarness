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
for t in asrharness vhs002 vhsui vhsext vhsplan vhsplanmodel vhswm vhscache vhsrecog vhsreal vhsroute vhsref; do
  go vet -tags "$t" ./... >/dev/null
  go test -tags "$t" ./... >/dev/null
done
echo "[gate] go test ./... (默认门禁)"
go test ./...
echo "[gate] OK"
