#!/bin/sh
# release_audit.sh —— 核「每个 tag 是否齐备交付三件套」。
#
# ⚠️ 为什么需要它（2026-10-03 实测教训）：
#   `docs/版本台账.md` 自己写着「每个版本必须带（**缺一不可**）」：
#     ① 冻结 sha + git tag   ② GitHub Release   ③ 一条 Issue 回写   ④ 已知缺口清单
#      ⑤ 测过什么/用了什么
#   而我**打了 8 个 tag（test.2..test.9）**：
#     · 台账里**只记了 test.1** ⇒ 漏 7 个
#     · `gh release list` 里**只有 test.1/test.2** ⇒ **漏 7 个 Release**
#   ⇒ **两个缺口都不是技术问题，是"我忘了"。**
#   ⇒ 而"写进文档"对行为没有约束力（今天反复量到）⇒ **把它变成机械检查。**
#
# 用法：  sh scripts/release_audit.sh            # 核最近 10 个 tag
#         sh scripts/release_audit.sh --all      # 核全部 tag
#
# 退出码：0 = 齐备；1 = 有缺口（**列出缺什么**）；2 = 装置不可用（gh 未登录等）
set -u

PATTERN='v0.2.0-test.*'
[ "${1:-}" = "--all" ] && PATTERN='v*'

if ! command -v gh >/dev/null 2>&1; then
  echo "[audit] ❌ 无 gh ⇒ **核不了**（不是"齐备"）"; exit 2
fi
if ! gh auth status >/dev/null 2>&1; then
  echo "[audit] ❌ gh 未登录 ⇒ **核不了**（不是"齐备"）"; exit 2
fi

TAGS=$(git tag -l "$PATTERN" | sort -V)
[ -z "$TAGS" ] && { echo "[audit] ⚠️ 无匹配 tag（pattern=${PATTERN}）⇒ 无对象可核"; exit 0; }

RELEASES=$(gh release list --limit 100 2>/dev/null | awk -F'\t' '{print $3}')

miss_rel=0
miss_led=0
n=0
echo "[audit] 核 tag（pattern=${PATTERN}）"
for t in $TAGS; do
  n=$((n+1))
  sha=$(git rev-parse --short "$t^{}" 2>/dev/null || echo "?")
  has_rel="✗"; has_led="✗"
  printf '%s\n' "$RELEASES" | grep -qx "$t" && has_rel="✅"
  # 台账里出现该 tag 名 或 其 sha 即算记过
  if grep -qF "$t" docs/版本台账.md 2>/dev/null || grep -qF "$sha" docs/版本台账.md 2>/dev/null; then
    has_led="✅"
  fi
  [ "$has_rel" = "✗" ] && miss_rel=$((miss_rel+1))
  [ "$has_led" = "✗" ] && miss_led=$((miss_led+1))
  printf "  %-18s %-9s Release %s  台账 %s\n" "$t" "$sha" "$has_rel" "$has_led"
done

echo "[audit] ---- 共 $n 个 tag · 缺 Release $miss_rel · 缺台账 $miss_led"
if [ "$miss_rel" -gt 0 ] || [ "$miss_led" -gt 0 ]; then
  echo "[audit] ❌ **交付不齐**（台账规定"缺一不可"）"
  [ "$miss_rel" -gt 0 ] && echo "[audit]    ⇒ 补 Release：gh release create <tag> --title <...> --notes-file <...> --prerelease"
  [ "$miss_led" -gt 0 ] && echo "[audit]    ⇒ 补台账：docs/版本台账.md 的表格加行（sha + 内容 + 验证）"
  exit 1
fi
echo "[audit] ✅ 齐备"

# ---- ③④⑤：台账另外三项（"缺一不可"的 ③Issue 回写 ④缺口清单 ⑤测过/用过）----
ISSUE=${VHS_AUDIT_ISSUE:-4}
ISSUE_BODY=$(gh issue view "$ISSUE" --json comments -q '.comments[].body' 2>/dev/null || echo "")
m_iss=0; m_gap=0; m_tst=0
echo "[audit] 核 ③Issue 回写 / ④缺口清单 / ⑤测过用过"
for t in $TAGS; do
  body=$(git tag -l --format='%(contents)' "$t" 2>/dev/null)
  i="✗"; g="✗"; v="✗"
  printf '%s' "$ISSUE_BODY" | grep -qF "$t" && i="✅"
  printf '%s' "$body" | grep -qE '未测|未做|缺口' && g="✅"
  grep -qF '测过' docs/版本台账.md 2>/dev/null && grep -qF "$t" docs/版本台账.md 2>/dev/null && v="✅"
  [ "$i" = "✗" ] && m_iss=$((m_iss+1))
  [ "$g" = "✗" ] && m_gap=$((m_gap+1))
  [ "$v" = "✗" ] && m_tst=$((m_tst+1))
  printf "  %-18s Issue %s  缺口 %s  测过 %s\n" "$t" "$i" "$g" "$v"
done
echo "[audit] ---- ③缺 ${m_iss} · ④缺 ${m_gap} · ⑤缺 ${m_tst}"
if [ "$m_iss" -gt 0 ] || [ "$m_gap" -gt 0 ] || [ "$m_tst" -gt 0 ]; then
  echo "[audit] ❌ **三项中有缺口**（台账规定"缺一不可"）"
  [ "$m_iss" -gt 0 ] && echo "[audit]    ⇒ ③补 Issue 回写：gh issue comment ${ISSUE} --body-file <...>（正文含 tag 名）"
  [ "$m_gap" -gt 0 ] && echo "[audit]    ⇒ ④补缺口清单：tag 说明里写"未测/未做"（**未测 != 通过**）"
  [ "$m_tst" -gt 0 ] && echo "[audit]    ⇒ ⑤补"测过/用过"：docs/版本台账.md 里该版本的对应段"
  exit 1
fi
echo "[audit] ✅ ③④⑤ 也齐备"
