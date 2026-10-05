#!/bin/bash
#
# m7-verify.sh — M7 真机一键闭环：构建 → 装到 iPhonePeter → 跑 UI/单元测试。
# 用法：在 voicesign-harness/ios/ 下执行  bash m7-verify.sh
# 前置：iPhonePeter 已配对解锁亮屏；harness server 在 0.0.0.0:8897（token m7-token）在场。
#
set -euo pipefail

DEVICE_ID="00008120-001428820AB8201E"
TEAM="P5W752L332"
SCHEME="VoxSign"
DD="/tmp/vhs-m7-dd"

cd "$(dirname "$0")"

echo "== [1/3] 构建（generic iOS）=="
xcodebuild -project ${SCHEME}.xcodeproj -scheme ${SCHEME} \
  -configuration Debug -destination 'generic/platform=iOS' \
  -derivedDataPath "${DD}" \
  CODE_SIGN_STYLE=Automatic DEVELOPMENT_TEAM=${TEAM} build

APP="${DD}/Build/Products/Debug-iphoneos/${SCHEME}.app"
echo "== [2/3] 安装到 ${DEVICE_ID} =="
xcrun devicectl device install app --device ${DEVICE_ID} "${APP}"

echo "== [3/3] 真机跑测试（单测 + UI）=="
xcodebuild test -project ${SCHEME}.xcodeproj -scheme ${SCHEME} \
  -destination "id=${DEVICE_ID}" -derivedDataPath "${DD}" \
  CODE_SIGN_STYLE=Automatic DEVELOPMENT_TEAM=${TEAM}

echo "== 完成：TEST SUCCEEDED 即闭环 =="
