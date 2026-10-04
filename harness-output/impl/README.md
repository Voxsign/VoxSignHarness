# 语音适配层

## 启动
构建：
go build ./...

启动服务：
go run ./cmd/vhs-voice

或构建后运行：
go build -o vhs-voice ./cmd/vhs-voice
./vhs-voice --data-dir ./data

默认监听 127.0.0.1:8950，仅回环地址，非回环拒绝。
可用 VHS_VOICE_ADDR 覆盖监听地址：VHS_VOICE_ADDR=127.0.0.1:8950
上游主 harness 默认 http://127.0.0.1:8941，可用 VHS_UPSTREAM 覆盖。
数据目录默认 ./data，可用 --data-dir 或 VHS_VOICE_DATA_DIR 覆盖。
鉴权占位：Authorization: Bearer <token>，未配置时本地回环放行。

## 端点
全部已注册：

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

说明：/v1/tasks 与 /v1/tasks/{id} 用于与主 harness 对接/透传任务。

## 数据文件
数据目录默认 ./data，UTF-8 持久化，append-only 可审计。

<dataDir>/voice_sessions/<conversation_id>.jsonl 会话记忆
<dataDir>/feedback.jsonl 反馈学习
<dataDir>/traces.jsonl 轨迹
<dataDir>/usage.jsonl 用量
<dataDir>/dictionary.json 个性化词典