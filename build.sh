#!/usr/bin/env bash
# VoxSign harness build script: vet + test + cross-compile for all platforms
# (zero external dependencies, CGO_ENABLED=0).
set -euo pipefail
cd "$(dirname "$0")"

echo "== go vet ./... =="
go vet ./...

echo "== go test ./... =="
go test ./...

echo "== gofmt check (should be empty) =="
if [ -n "$(gofmt -l .)" ]; then
  echo "gofmt found unformatted files:"
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

echo "== artifacts =="
ls -lh dist
