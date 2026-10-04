语音适配层 README

启动
go build ./...
go run ./cmd/vhs-voice
默认监听 127.0.0.1:8950，仅回环；上游主 harness 默认 http://127.0.0.1:8941。

可配置项
VHS_VOICE_ADDR=127.0.0.1:8950
VHS_UPSTREAM=http://127.0.0.1:8941
VHS_DATA_DIR=/path/to/data 或 --data-dir /path/to/data

示例
VHS_VOICE_ADDR=127.0.0.1:8950 VHS_UPSTREAM=http://127.0.0.1:8941 go run ./cmd/vhs-voice

端点
GET /v1/health
POST /v1/process
GET /v1/voice/health
POST /v1/voice/parse
POST /v1/voice/decompose
POST /v1/voice/resolve
POST /v1/voice/run
GET /v1/voice/tasks/{conversation_id}
POST /v1/tasks
GET /v1/tasks/{id}

数据文件
<dataDir>/voice_sessions/<conversation_id>.jsonl
<dataDir>/feedback.jsonl
<dataDir>/traces.jsonl
<dataDir>/usage.jsonl
全部 UTF-8、append-only，可审计。

健康检查
curl http://127.0.0.1:8950/v1/voice/health