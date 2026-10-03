#!/bin/sh
# spec_code_align.sh —— 核「**规范里写的**」与「**代码里做的**」是否对齐。
#
# ⚠️ 为什么需要它（2026-10-03 · 我为此追错 10 轮）
#   `docs/SPEC-v2-可执行规格书.md` §4.1 的 space_check **伪代码把四步判定简化成一句**
#   （`allowedTools := … ∩ …; if == ∅ ⇒ default_deny`），而**实际代码是逐项判定**：
#     !Grant.Authorized ⇒ default_deny · !Perms.Read ⇒ default_deny
#     needsWrite && !Perms.Write ⇒ default_deny · 逐 cap ⇒ boundary_violation · 跨域 ⇒ cross_ref_deny
#   ⇒ 我**据规范伪代码推理** ⇒ 以为"cap 不匹配 ⇒ default_deny"
#     ⇒ 于是把实测的 `boundary_violation` 读成「越界（范围问题）」
#     ⇒ 而它是「工具未授权」⇒ **两种修法完全不同** ⇒ **追错 10 轮**
#
#   ⇒ 根因不是我不够仔细，而是：**规范用"简化"写，读者会当"完整"用。**
#   ⇒ 而"下次读代码"没用（今天反复量到：写进文档对行为没有约束力）
#   ⇒ **做成一条命令：拿规范当尺子之前，先量它有多长。**
#
# 用法：  sh scripts/spec_code_align.sh
# 退出码：0 = 对齐；1 = **有不对齐**（列出「只在代码里」的判定项）；2 = 装置不可用
set -u

SPEC="docs/SPEC-v2-可执行规格书.md"
[ -f "$SPEC" ] || { echo "[align] ❌ 找不到 ${SPEC}"; exit 2; }

# 判定项清单：<显示名>|<代码里的模式（**ERE，特殊字符须转义**）>|<规范里应出现的串>
# ⚠️ 新增判定项时**必须同步加一行** —— 否则它不会被核（与 SK-CHK-1 同一纪律）
CHECKS='
unknown_space|unknown_space|unknown_space
drift|v.Reason = "drift"|drift
scope∩exclude ⇒ boundary_violation|overlap\(m\.Scope, m\.Exclude\)|Boundary.Exclude
逐 cap ⇒ boundary_violation|!contains\(m\.Tools, cap\)|cap ∉ space.Tools
契约 caps 越界|ccaps != nil && !ccaps\[cap\]|∪contract.Caps
Grant.Authorized ⇒ default_deny|!in\.Grant\.Authorized|Grant.Authorized
Perms.Read ⇒ default_deny|!m\.Perms\.Read|Perms.Read
needsWrite ⇒ default_deny|needsWrite\(in\.Intent\)|needsWrite
cross_ref_deny|v.Reason = "cross_ref_deny"|cross_ref_deny
'

miss=0
n=0
echo "[align] 核「代码有的判定」是否都写在规范里"
echo "[align]   规范：${SPEC}"
printf '%s\n' "$CHECKS" | while IFS= read -r line; do
  [ -z "$line" ] && continue
  name=$(printf '%s' "$line" | cut -d'|' -f1)
  cpat=$(printf '%s' "$line" | cut -d'|' -f2)
  spat=$(printf '%s' "$line" | cut -d'|' -f3)
  in_code=✗; in_spec=✗
  git grep -qE -- "$cpat" -- '*.go' 2>/dev/null && in_code=✅
  grep -qF -- "$spat" "$SPEC" 2>/dev/null && in_spec=✅
  printf "  %-34s 代码 %s  规范 %s\n" "$name" "$in_code" "$in_spec"
  if [ "$in_code" = "✅" ] && [ "$in_spec" = "✗" ]; then
    echo "       ⚠️ **只在代码里** ⇒ 规范漏了它 ⇒ 读者会以为它不存在"
  fi
done

# 计数（与上面同一份清单，单独算，避免子 shell 丢变量）
tot=0; code_n=0; spec_n=0
while IFS= read -r line; do
  [ -z "$line" ] && continue
  cpat=$(printf '%s' "$line" | cut -d'|' -f2)
  spat=$(printf '%s' "$line" | cut -d'|' -f3)
  tot=$((tot+1))
  git grep -qE -- "$cpat" -- '*.go' 2>/dev/null && code_n=$((code_n+1))
  grep -qF -- "$spat" "$SPEC" 2>/dev/null && spec_n=$((spec_n+1))
done <<EOF
$(printf '%s\n' "$CHECKS" | grep .)
EOF

echo "[align] ---- 判定项 ${tot} · 代码含 ${code_n} · 规范含 ${spec_n}"
if [ "$spec_n" -lt "$code_n" ]; then
  echo "[align] ❌ **规范比代码少 $((code_n - spec_n)) 项** ⇒ 拿规范当尺子会漏判"
  echo "[align]    ⇒ 处置：把缺的判定**补进 ${SPEC}**（与本项目既有做法一致）"
  exit 1
fi
echo "[align] ✅ 对齐（规范覆盖了代码里的全部判定项）"
