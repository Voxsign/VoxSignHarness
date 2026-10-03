#!/bin/sh
# board.sh —— 全量看板。**必须能读出 pass 数**（口径：用 -v 数 "--- PASS"）。
# 教训：非 verbose 只打印 ok、不打印 "--- PASS" ⇒ 曾把"没统计到"读成"没有 PASS"。
set -e
printf '%-16s %5s %5s %5s\n' tag red pass skip
for spec in \
  "asrharness:./asr" "vhs002:./asr" "vhsext:./world" "vhsplan:./plan" "vhsplanmodel:./plan" \
  "vhswm:./plan" "vhscache:./hotcache" "vhsrecog:./recog" "vhsreal:./recog" "vhsroute:./route" "vhsref:./ref"; do
  tag=${spec%%:*}; pkg=${spec##*:}
  out=$(go test -tags "$tag" "$pkg" -v 2>&1 || true)
  r=$(printf '%s' "$out" | grep -c '^--- FAIL' || true)
  p=$(printf '%s' "$out" | grep -c '^--- PASS' || true)
  k=$(printf '%s' "$out" | grep -c '^--- SKIP' || true)
  printf '%-16s %5s %5s %5s\n' "$tag" "$r" "$p" "$k"
done
go test ./... >/dev/null 2>&1 && echo "default: exit 0" || echo "default: exit NONZERO"
