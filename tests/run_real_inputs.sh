#!/bin/bash
# tests/run_real_inputs.sh -- 用用户真实输入测试 harness。
# 所有用例都来自用户在对话中真实说过的话。

set -e
export PATH=/Users/sofia/.local/go/bin:$PATH
cd /Users/sofia/voxsign-work/VoxSignHarness

export AIOPS_KEY=$AIOPS_KEY
export VHS_LOG_DIR=/tmp/vhs-real-inputs
rm -rf $VHS_LOG_DIR
mkdir -p $VHS_LOG_DIR/zhiji

# 预置用户真实实体（来自用户偏好）
cat > $VHS_LOG_DIR/zhiji/entities.json <<'JSON'
[
  {"id":"e-1","type":"person","canonical":"Amber","aliases":["冀总","老冀"],"desc":"consultant 客户","confidence":0.9,"status":"active"},
  {"id":"e-2","type":"person","canonical":"Peter","aliases":["老Peter"],"desc":"商业伙伴，物流SaaS项目","confidence":0.9,"status":"active"},
  {"id":"e-3","type":"person","canonical":"<USER>","aliases":["<ALIAS>"],"desc":"用户本人，在沙特常住","confidence":0.9,"status":"active"},
  {"id":"e-4","type":"project","canonical":"UniFusion","aliases":["unifusion-bsc.com"],"desc":"Sofia的项目，Google OAuth归属","confidence":0.9,"status":"active"}
]
JSON

PASS=0
FAIL=0

run() {
  local desc="$1" input="$2" expect="$3"
  echo "--- $desc"
  echo "  用户: $input"
  out=$(go run . run "$input" 2>&1 | grep "^Result:" | head -1)
  echo "  回复: $out"
  if echo "$out" | grep -q "$expect"; then
    echo "  ✅ PASS"
    PASS=$((PASS+1))
  else
    echo "  ❌ FAIL (期望: $expect)"
    FAIL=$((FAIL+1))
  fi
  echo ""
}

echo "=========================================="
echo "真实输入测试套件"
echo "=========================================="
echo ""

# === A. 用户身份（截图真实对话）===
echo "=== A. 用户身份记忆（截图真实场景）==="
run "A1 自我介绍ASR错+breakdown" "我叫张大山，呃，某个地方的某，某个形容词的某，某个时间的某" "<USER_NAME>"
run "A2 ASR错张大山" "我的名字叫做张大山，某个地方的某，某个形容词的某，某个时间的某" "<USER_NAME>"
run "A3 用户抱怨逻辑错" "你明显是逻辑错了嘛，我后面解释那么清楚，你还搞错了" "<USER_NAME>"
run "A4 ASR错张大山" "我叫张大山" "<USER_NAME>"
run "A5 拼字母ZOU" "这个叫张大山，Z O U，对吧？" "<USER_NAME>"
run "A6 问名字" "我叫什么名字？" "<USER_NAME>"

# === B. 实体 resolve（用户真实提到的人）===
echo "=== B. 实体理解 ==="
run "B1 问冀总是谁" "冀总是谁来着？" "Amber"
run "B2 问老冀" "老冀那边怎么样了？" "Amber"
run "B3 问Peter" "Peter是谁？" "物流"
run "B4 问<ALIAS>是谁" "<ALIAS>是谁？" "<USER>"
run "B5 问UniFusion" "UniFusion是什么项目？" "Sofia"

# === C. 代指理解 ===
echo "=== C. 代指理解 ==="
run "C1 提到Amber" "今天跟 Amber 开了个会" "Amber"
run "C2 提到Peter" "Peter 刚发了邮件给我" "Peter"
run "C3 代指那个(应反问)" "那个项目最近怎么样了？" "Peter\|Amber\|冀总"

echo "=========================================="
echo "结果: $PASS PASS, $FAIL FAIL"
echo "=========================================="
echo ""
echo "=== self_model ==="
cat $VHS_LOG_DIR/zhiji/self_model.json | python3 -c "import sys,json; [print(f'  {x[\"status\"]:10} {x[\"text\"]}') for x in json.load(sys.stdin)]"
echo ""
echo "=== entities ==="
cat $VHS_LOG_DIR/zhiji/entities.json | python3 -c "import sys,json; [print(f'  {e[\"canonical\"]:12} ({\"/\".join(e[\"aliases\"])}) — {e[\"desc\"]}') for e in json.load(sys.stdin)]"
