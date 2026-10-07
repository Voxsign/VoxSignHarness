#!/bin/bash
# tests/run_e2e.sh -- VoxSign Harness 端到端测试套件。
#
# 用法: bash tests/run_e2e.sh
# 从干净状态开始，跑一系列真实对话场景，自动判定 pass/fail。

set -e
export PATH=/Users/sofia/.local/go/bin:$PATH
HARNESS=/Users/sofia/voxsign-work/VoxSignHarness
cd $HARNESS

export AIOPS_KEY=aiops-mac-sophia-8cdec6c75e5a1528f2efed609fbbb05f
export VHS_LOG_DIR=/tmp/vhs-test-suite
rm -rf $VHS_LOG_DIR
mkdir -p $VHS_LOG_DIR/zhiji

# 预置实体（模拟长期使用后已记住的）
cat > $VHS_LOG_DIR/zhiji/entities.json <<'JSON'
[
  {"id":"e-1","type":"person","canonical":"Amber","aliases":["冀总","老冀"],"desc":"consultant 客户","confidence":0.9,"status":"active"},
  {"id":"e-2","type":"person","canonical":"Peter","aliases":["老Peter"],"desc":"商业伙伴，物流SaaS项目","confidence":0.9,"status":"active"}
]
JSON

PASS=0
FAIL=0

# run_case <编号> <描述> <用户输入> <grep关键词>
run_case() {
  local id="$1" desc="$2" input="$3" expect="$4"
  echo "--- [$id] $desc"
  echo "    用户: $input"
  out=$(go run . run "$input" 2>&1 | grep "^Result:" | head -1)
  echo "    回复: $out"
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
echo "VoxSign Harness 端到端测试"
echo "=========================================="
echo ""

# === 组1: 用户身份记忆 + ASR 纠错 ===
echo "=== 组1: 用户身份记忆 + ASR 同音字纠错 ==="
run_case 1.1 "自我介绍(ASR错+breakdown)" "我叫周永明，山东邹县的邹，勇敢的勇，明天的明" "邹勇明"
run_case 1.2 "ASR错'周勇明'(应拦住)" "我叫周勇明" "邹勇明"
run_case 1.3 "ASR错'周永勇'(应拦住)" "我叫周永勇" "邹勇明"
run_case 1.4 "问名字" "我叫什么名字？" "邹勇明"

# === 组2: 实体 resolve ===
echo "=== 组2: 命名实体 resolve ==="
run_case 2.1 "问冀总是谁" "冀总是谁？" "Amber"
run_case 2.2 "问老冀" "老冀那边怎么样了？" "Amber"
run_case 2.3 "问Peter" "Peter是谁？" "物流"

# === 组3: 代指 resolve ===
echo "=== 组3: 代指理解 ==="
run_case 3.1 "提到Amber" "今天跟 Amber 开了个会" "Amber"
run_case 3.2 "提到Peter" "Peter 刚发了邮件" "Peter"
run_case 3.3 "代指'那个'(应反问)" "那个项目最近怎么样了？" "Peter\|Amber\|冀总"

# === 汇总 ===
echo "=========================================="
echo "测试结果: $PASS PASS, $FAIL FAIL"
echo "=========================================="
