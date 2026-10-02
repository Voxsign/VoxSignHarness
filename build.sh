#!/usr/bin/env bash
# VoxSign harness 构建脚本：vet + test + 全平台交叉编译（零外部依赖，CGO_ENABLED=0）。
set -euo pipefail
cd "$(dirname "$0")"

echo "== go vet ./... =="
go vet ./...

echo "== go test ./... =="
go test ./...

echo "== gofmt 检查（应为空）=="
if [ -n "$(gofmt -l .)" ]; then
  echo "gofmt 发现未格式化文件："
  gofmt -l .
  exit 1
fi

mkdir -p dist
for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
  os="${target%/*}"
  arch="${target#*/}"
  out="dist/vhs-${os}-${arch}"
  if [ "$os" = "windows" ]; then out="${out}.exe"; fi
  echo "== build ${os}/${arch} -> ${out} =="
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w" -o "$out" .
done

echo "== 产物 =="
ls -lh dist
