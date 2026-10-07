#!/bin/bash
# tests/run_corpus.sh -- 用用户真实聊天记录批量测 harness。
# 从 /tmp/vhs-real-corpus.txt 抽样，跑 harness，统计通过率。

set -e
export PATH=/Users/sofia/.local/go/bin:$PATH
cd /Users/sofia/voxsign-work/VoxSignHarness

export AIOPS_KEY=aiops-mac-sophia-8cdec6c75e5a1528f2efed609fbbb05f
export VHS_LOG_DIR=/tmp/vhs-corpus-test
rm -rf $VHS_LOG_DIR
mkdir -p $VHS_LOG_DIR/zhiji

# 预置实体
cat > $VHS_LOG_DIR/zhiji/entities.json <<'JSON'
[
  {"id":"e-1","type":"person","canonical":"Amber","aliases":["冀总","老冀"],"desc":"consultant 客户","confidence":0.9,"status":"active"},
  {"id":"e-2","type":"person","canonical":"Peter","aliases":["老Peter"],"desc":"商业伙伴，物流SaaS","confidence":0.9,"status":"active"},
  {"id":"e-3","type":"person","canonical":"Mansour","aliases":["美墅"],"desc":"用户本人，在沙特","confidence":0.9,"status":"active"}
]
JSON

PASS=0
FAIL=0
CLARIFY=0
TOTAL=0

echo "=========================================="
echo "真实语料批量测试（50条抽样）"
echo "=========================================="
echo ""

# 取前 50 条
head -50 /tmp/vhs-real-corpus.txt | while IFS= read -r line; do
  TOTAL=$((TOTAL+1))
  # 跳过空行和太短的
  if [ ${#line} -lt 5 ]; then
    continue
  fi
  echo "--- [$TOTAL] $line"
  out=$(go run . run "$line" 2>&1 | grep "^Result:" | head -1)
  echo "    → $out"
  # 判断：如果包含 "need clarification" 或 "不太确定" 或 "再说"，算 FAIL
  if echo "$out" | grep -q "need clarification\|不太确定\|再说一遍\|没跟上"; then
    echo "    ⚠️  没理解"
    CLARIFY=$((CLARIFY+1))
  else
    echo "    ✅ 理解了"
    PASS=$((PASS+1))
  fi
  echo ""
done

echo "=========================================="
echo "统计: $PASS 理解, $CLARIFY 没理解, 共 $TOTAL 条"
echo "=========================================="
