#!/bin/bash
# tests/run_real_case.sh -- 用截图里的真实对话序列测试 harness。
#
# 来源：VoxSign iOS 截图（2026-10-07 17:34），真实失败案例：
#   系统在 周永明/周永勇/周勇明 之间横跳。
# 现在测：新架构能不能正确处理这个序列。

set -e
export PATH=/Users/sofia/.local/go/bin:$PATH
cd /Users/sofia/voxsign-work/VoxSignHarness

export AIOPS_KEY=$AIOPS_KEY
export VHS_LOG_DIR=/tmp/vhs-real-screenshot
rm -rf $VHS_LOG_DIR
mkdir -p $VHS_LOG_DIR/zhiji

PASS=0
FAIL=0

run_step() {
  local desc="$1" input="$2" expect="$3"
  echo "--- $desc"
  echo "    用户说: $input"
  out=$(go run . run "$input" 2>&1 | grep "^Result:" | head -1)
  echo "    系统答: $out"
  if echo "$out" | grep -q "$expect"; then
    echo "    ✅ PASS"
    PASS=$((PASS+1))
  else
    echo "    ❌ FAIL (期望包含: $expect)"
    FAIL=$((FAIL+1))
  fi
  echo ""
}

echo "=========================================="
echo "真实截图对话复现测试"
echo "场景：用户反复纠正名字，ASR 各种错"
echo "=========================================="
echo ""

# 截图里的真实序列（ASR 转写就是这些）
run_step "1. ASR错'周永明'+breakdown解释" \
  "我叫周永明，呃，山东那个邹县的邹，勇敢的勇，明天的明" \
  "<USER_NAME>"

run_step "2. ASR错'周勇明'(系统不应横跳)" \
  "我的名字叫做周勇明，山东邹县的邹，勇敢的勇，明天的明" \
  "<USER_NAME>"

run_step "3. 用户抱怨系统逻辑错(不应再错)" \
  "你明显是逻辑错了嘛，我后面解释那么清楚，你还搞错了" \
  "<USER_NAME>"

run_step "4. ASR错'周永勇'(应拦住)" \
  "好，确认了！你是周永勇" \
  "<USER_NAME>"

run_step "5. 用户拼字母ZOU(应识别)" \
  "这个叫周永明，Z O U，对吧？" \
  "<USER_NAME>"

run_step "6. 问名字(最终答案)" \
  "记住了吗？我叫什么？" \
  "<USER_NAME>"

echo "=========================================="
echo "结果: $PASS PASS, $FAIL FAIL"
echo "=========================================="
echo ""
echo "=== 最终 self_model ==="
cat $VHS_LOG_DIR/zhiji/self_model.json | python3 -c "import sys,json; [print(f'  {x[\"status\"]:10} v{x[\"version\"]} {x[\"text\"]}') for x in json.load(sys.stdin)]"
