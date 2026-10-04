语音适配层 README 运行说明

启动
构建：go build ./...
启动：go run ./cmd/vhs-voice
默认监听 127.0.0.1:8950，可用环境变量 VHS_VOICE_ADDR 覆盖。
上游主 harness 默认 http://127.0.0.1:8941，可用环境变量 VHS_UPSTREAM 覆盖。
数据目录可配（data-dir）；落盘均为 append-only JSONL。

端点
GET  /v1/voice/health
POST /v1/voice/parse
POST /v1/voice/decompose
POST /v1/voice/resolve
POST /v1/voice/run
GET  /v1/voice/tasks/{conversation_id}
POST /v1/tasks
GET  /v1/tasks/{id}
P0 端点：
GET  /v1/health
POST /v1/process

数据文件
<dataDir>/voice_sessions/<conversation_id>.jsonl：会话记忆，可审计
<dataDir>/feedback.jsonl：反馈学习，append-only
<dataDir>/traces/：追踪记录 JSONL，append-only
<dataDir>/usage/：用量记录 JSONL，append-only

行为要点
resolve 无会话历史且文本含指代词时，返回 unresolved/pending_resolve，不返回默认目标。
run 编排任务并投递上游 harness，响应包含 task_id、submitted、summary.total。