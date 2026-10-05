#!/usr/bin/env bash
# zhiji-check.sh —— 开发机一键检查知己 Phase 0 包（zhiji/）。
# 在无 CI 的开发机上快速跑：build + vet + test + gofmt。
# 用法：bash scripts/zhiji-check.sh
set -euo pipefail
cd "$(dirname "$0")/.."

echo "== go build ./zhiji/ =="
go build ./zhiji/

echo "== go vet ./zhiji/ =="
go vet ./zhiji/

echo "== go test ./zhiji/ -v =="
go test ./zhiji/ -v

echo "== gofmt 检查（应为空）=="
if [ -n "$(gofmt -l zhiji/)" ]; then
  echo "gofmt 发现未格式化文件："
  gofmt -l zhiji/
  exit 1
fi

echo "== zhiji 检查通过 =="
