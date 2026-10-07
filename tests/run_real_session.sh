#!/bin/bash
# tests/run_real_session.sh -- 拿一个真实session完整连续对话测试。
# 不是孤立句子，是真实连续对话——测代指resolve和上下文理解。

set -e
export PATH=/Users/sofia/.local/go/bin:$PATH
cd /Users/sofia/voxsign-work/VoxSignHarness

export AIOPS_KEY=aiops-mac-sophia-8cdec6c75e5a1528f2efed609fbbb05f
export VHS_LOG_DIR=/tmp/vhs-real-session
rm -rf $VHS_LOG_DIR
mkdir -p $VHS_LOG_DIR/zhiji

# 提取真实session的用户输入
python3 <<'PYEOF'
import json
traj = "/Users/sofia/Library/Application Support/DoubaoWork/Default/.doubaowork/agent_mode/workspace/.sessions/38443058492523778/agents/m_0cwpvqMHFIp/system/trajectory.jsonl"
inputs = []
with open(traj) as f:
    for line in f:
        obj = json.loads(line)
        role = obj.get("role", obj.get("type", ""))
        content = obj.get("content", "")
        if role == "user" and content and len(str(content)) > 10:
            text = str(content).strip()
            if not text.startswith("{") and not text.startswith("<") and "tool_result" not in text[:20]:
                inputs.append(text)
with open("/tmp/vhs-session-inputs.txt", "w") as f:
    for i, inp in enumerate(inputs[:50]):  # 先测前50条
        f.write(inp + "\n")
print(f"提取了 {len(inputs[:50])} 条真实连续对话")
PYEOF

PASS=0
FAIL=0

echo "=========================================="
echo "真实连续对话测试（前50条）"
echo "=========================================="
echo ""

i=0
while IFS= read -r line; do
  i=$((i+1))
  if [ ${#line} -lt 10 ]; then
    continue
  fi
  out=$(go run . run "$line" 2>&1 | grep "^Result:" | head -1)
  # 判断：如果包含"need clarification"或"没理解"或"再说一遍"，算FAIL
  if echo "$out" | grep -q "need clarification\|没理解\|再说一遍\|没跟上"; then
    echo "[$i] ❌ $line → $out"
    FAIL=$((FAIL+1))
  else
    echo "[$i] ✅ $line → $out" | head -c 120
    echo ""
    PASS=$((PASS+1))
  fi
done < /tmp/vhs-session-inputs.txt

echo ""
echo "=========================================="
echo "结果: $PASS PASS, $FAIL FAIL"
echo "通过率: $(python3 -c "print(f'{$PASS/($PASS+$FAIL)*100:.1f}%')")"
echo "=========================================="
