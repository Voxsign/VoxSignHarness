#!/bin/bash
# =============================================================================
# ASR 个性化后台服务 · 一键验收脚本（真装配，不许桩）
# 用法:  sh scripts/accept_asr.sh [端口] [产物目录]
# 产物目录默认 harness-output/ASR个性化后台服务（人工补齐版）；可指 harness-output/个性化后台实现（harness 自产契约版）
# 退出码: 0 = 全部判据 PASS；非 0 = 有 FAIL
# 期望: 每行 ✅ 判据名；末尾"RESULT: ALL_PASS / HAS_FAIL"
# =============================================================================
set -u
D="${2:-harness-output/ASR个性化后台服务}"
PORT="${1:-8911}"
DATA="$(mktemp -d /tmp/asr_acc_data.XXXXXX)"
B="http://127.0.0.1:${PORT}"
PASS=0; FAIL=0

ok() { echo "✅ $1"; PASS=$((PASS+1)); }
no() { echo "❌ $1 — $2"; FAIL=$((FAIL+1)); }

echo "==== ASR 个性化后台服务 · 验收开始（端口 ${PORT}，数据目录 ${DATA}）===="

# ---- 判据 1：真编译（仅标准库）----
(cd "$D" && go build -o /tmp/asr_acc . >/dev/null 2>&1) || { echo "❌ go build 失败"; exit 1; }
ok "go build 零错误"

# ---- 起服务（独立进程，干净数据目录）----
/tmp/asr_acc -addr "127.0.0.1:${PORT}" -data-dir "$DATA" >/tmp/asr_acc_run.log 2>&1 &
SRV=$!
sleep 2

# ---- 判据 2：健康检查 ----
R=$(curl -s --max-time 5 "$B/v1/health")
[ "$R" = '{"ok":true}' ] && ok "/v1/health → $R" || no "/v1/health" "$R"

# ---- 判据 3：教词（term → dictionary.jsonl）----
R=$(curl -s --max-time 5 -X POST "$B/v1/term" -H 'Content-Type: application/json' -d '{"term":"曼苏","correction":"Mansour"}')
echo "$R" | grep -q '"ok":true' && ok "/v1/term 教词（曼苏→Mansour）" || no "/v1/term" "$R"

# ---- 判据 4：process 纠错生效（此时无黑名单，曼苏应被纠成 Mansour）----
R=$(curl -s --max-time 5 -X POST "$B/v1/process" -H 'Content-Type: application/json' -d '{"text":"你好 曼苏"}')
echo "$R" | grep -q 'Mansour' && ok "/v1/process 纠错生效 → $R" || no "/v1/process 纠错" "$R"

# ---- 判据 5：correct 纠错 + applied 明细 ----
R=$(curl -s --max-time 5 -X POST "$B/v1/correct" -H 'Content-Type: application/json' -d '{"text":"你好 曼苏"}')
echo "$R" | grep -q '曼苏→Mansour' && ok "/v1/correct 纠错+明细 → $R" || no "/v1/correct" "$R"

# ---- 判据 6：dict 返回全量 ----
R=$(curl -s --max-time 5 "$B/v1/dict")
echo "$R" | grep -q '曼苏' && ok "/v1/dict 全量词典" || no "/v1/dict" "$R"

# ---- 判据 7：feedback 追加 feedback.jsonl ----
R=$(curl -s --max-time 5 -X POST "$B/v1/feedback" -H 'Content-Type: application/json' -d '{"raw":"曼苏","corrected":"Mansour","accepted":false,"reason":"user_marked_wrong"}')
echo "$R" | grep -q '"ok":true' && ok "/v1/feedback 登记" || no "/v1/feedback" "$R"
[ -s "$DATA/feedback.jsonl" ] && ok "feedback.jsonl 落盘（append-only）" || no "feedback.jsonl 落盘" "缺失"

# ---- 判据 8：blacklist 写入 + 纠错拦截 ----
R=$(curl -s --max-time 5 -X POST "$B/v1/blacklist" -H 'Content-Type: application/json' -d '{"term":"曼苏","note":"改错了"}')
echo "$R" | grep -q '"ok":true' && ok "/v1/blacklist 登记" || no "/v1/blacklist" "$R"
[ -s "$DATA/blacklist.json" ] && ok "blacklist.json 落盘" || no "blacklist.json 落盘" "缺失"
R=$(curl -s --max-time 5 -X POST "$B/v1/process" -H 'Content-Type: application/json' -d '{"text":"你好 曼苏"}')
echo "$R" | grep -q 'Mansour' && no "/v1/process 黑名单拦截" "黑名单词仍被纠错: $R" || ok "/v1/process 黑名单拦截（曼苏不再被纠）"

# ---- 判据 9：持久化跨重启 ----
kill "$SRV" 2>/dev/null; sleep 1
/tmp/asr_acc -addr "127.0.0.1:${PORT}" -data-dir "$DATA" >/tmp/asr_acc_run2.log 2>&1 &
SRV=$!
sleep 2
R=$(curl -s --max-time 5 "$B/v1/dict")
echo "$R" | grep -q '曼苏' && ok "重启后词典持久化（dictionary.jsonl 生效）" || no "重启后词典持久化" "$R"
kill "$SRV" 2>/dev/null

echo "==== 验收汇总：${PASS} PASS / ${FAIL} FAIL ===="
[ "$FAIL" -eq 0 ] && echo "RESULT: ALL_PASS" || echo "RESULT: HAS_FAIL"
exit "$FAIL"
