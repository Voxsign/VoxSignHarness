#!/bin/bash
# tests/run_corpus_full.sh -- 跑全部真实语料，输出统计报告。

set -e
export PATH=/Users/sofia/.local/go/bin:$PATH
cd /Users/sofia/voxsign-work/VoxSignHarness

export AIOPS_KEY=aiops-mac-sophia-8cdec6c75e5a1528f2efed609fbbb05f
export VHS_LOG_DIR=/tmp/vhs-corpus-full
rm -rf $VHS_LOG_DIR
mkdir -p $VHS_LOG_DIR/zhiji

cat > $VHS_LOG_DIR/zhiji/entities.json <<'JSON'
[
  {"id":"e-1","type":"person","canonical":"Amber","aliases":["冀总","老冀"],"desc":"consultant 客户","confidence":0.9,"status":"active"},
  {"id":"e-2","type":"person","canonical":"Peter","aliases":["老Peter"],"desc":"商业伙伴，物流SaaS","confidence":0.9,"status":"active"},
  {"id":"e-3","type":"person","canonical":"Mansour","aliases":["美墅"],"desc":"用户本人，在沙特","confidence":0.9,"status":"active"}
]
JSON

REPORT=/tmp/vhs-corpus-report.txt
> $REPORT

i=0
while IFS= read -r line; do
  i=$((i+1))
  if [ ${#line} -lt 5 ]; then
    continue
  fi
  out=$(go run . run "$line" 2>&1 | grep "^Result:" | head -1)
  # 分类
  if echo "$out" | grep -q "need clarification\|不太确定\|再说一遍\|没跟上\|没听明白\|没记住\|再跟我\|再说一下"; then
    verdict="MISUNDERSTAND"
  elif echo "$out" | grep -q "pending\|passed space\|BOUNDARY\|Not executed"; then
    verdict="ROUTED"
  else
    verdict="UNDERSTOOD"
  fi
  echo "[$i] $verdict | $line → $out" | head -c 200 >> $REPORT
  echo "" >> $REPORT
  echo "[$i] $verdict"
done < /tmp/vhs-real-corpus.txt

echo ""
echo "=== 汇总 ==="
grep -c "UNDERSTOOD" $REPORT | xargs echo "UNDERSTOOD:"
grep -c "MISUNDERSTAND" $REPORT | xargs echo "MISUNDERSTAND:"
grep -c "ROUTED" $REPORT | xargs echo "ROUTED(路由到动作):"
