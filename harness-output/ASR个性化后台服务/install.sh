#!/bin/sh
# ASR 个性化后台服务 · 安装脚本
# 用法:  sh install.sh [PREFIX]     默认安装到 <产物目录>/bin
# 安装后:  bin/asr-service -addr 127.0.0.1:8080 -data-dir ./data
set -e
PREFIX="${1:-$PWD/bin}"
mkdir -p "$PREFIX"
go build -o "$PREFIX/asr-service" .
echo "[install] 已安装: $PREFIX/asr-service"
echo "[install] 运行示例: $PREFIX/asr-service -addr 127.0.0.1:8080 -data-dir ./data"
echo "[install] 验收: sh ../../scripts/accept_asr.sh 8911"
