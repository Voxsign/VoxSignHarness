语音适配层 README（运行说明）

启动
- 构建：go build ./...
- 运行：go run ./cmd/vhs-voice
- 或先构建后运行：./vhs-voice
- 默认监听：127.0.0.1:8950，仅监听回环地址，非回环拒绝；可用 VHS_VOICE_ADDR 覆盖。
- 上游主 harness：默认 http://127.0.0.1:8941；可用 VHS_UPSTREAM 覆盖。
- 数据目录：dataDir 可配，示例使用 --data-dir ./data。
- 启动示例：
  VHS_VOICE_ADDR=127.0.0.1:8950 VHS_UPSTREAM=http://127.0.0.1:8941 ./vhs-voice --data-dir ./data

验收端点清单（全部注册）
本服务 vhs-voice，默认 127.0.0.1:8950：
- GET /v1/voice/health
- POST /v1/voice/parse
- POST /v1/voice/decompose
- POST /v1/voice/resolve
- POST /v1/voice/run
- GET /v1/voice/tasks/{conversation_id}

上游主 harness，默认 127.0.0.1:8941，由适配层调用：
- POST /v1/tasks
- GET /v1/tasks/{id}

P0 兼容端点：
- GET /v1/health
- POST /v1/process
- 鉴权可占位但已保留。

数据文件
- 会话记忆：<dataDir>/voice_sessions/<conversation_id>.jsonl
- 反馈学习：<dataDir>/feedback.jsonl
- 追踪：<dataDir>/traces.jsonl
- 用量：<dataDir>/usage.jsonl
- 个性化词典：<dataDir>/dictionary.jsonl
以上 JSONL 均为 append-only、UTF-8，可审计，中文全链路不乱码。