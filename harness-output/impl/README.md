# 语音适配层 vhs-voice 运行说明

## 启动
go build ./...

go run ./cmd/vhs-voice -addr 127.0.0.1:8950 -data-dir ./data

环境变量：
VHS_VOICE_ADDR 监听地址，默认 127.0.0.1:8950
VHS_UPSTREAM 主 harness 地址，默认 http://127.0.0.1:8941

说明：
仅监听 127.0.0.1，非回环地址拒绝。

## 端点
GET /v1/voice/health
POST /v1/voice/parse
POST /v1/voice/decompose
POST /v1/voice/resolve
POST /v1/voice/run
GET /v1/voice/tasks/{conversation_id}
POST /v1/tasks
GET /v1/tasks/{id}
GET /v1/health
POST /v1/process

## 数据文件
会话记忆/审计：
<dataDir>/voice_sessions/<conversation_id>.jsonl

反馈学习：
<dataDir>/feedback.jsonl

追踪与用量：
<dataDir>/traces/*.jsonl
<dataDir>/usage/*.jsonl

以上 JSONL 均为 append-only，数据目录由 data-dir 指定，独立存放，可审计。