README（语音适配层 / vhs-voice）

一、服务定位
独立 Go 服务 cmd/vhs-voice，仅标准库，零第三方依赖。接收手机端语音识别后的文本，
执行：去噪提意 → 复合指令拆单任务 → 域/对象补全 → 逐个投递主 harness → 语音友好汇总。
上游主 harness 仅调用 POST /v1/tasks 与 GET /v1/tasks/{id}。

二、环境要求
Go 1.21+，UTF-8 终端。仅监听 127.0.0.1，非回环请求拒绝。
无第三方依赖，无 TODO 占位。

三、构建与启动
构建全部包：go build ./...
单独构建：go build -o bin/vhs-voice ./cmd/vhs-voice
启动：go run ./cmd/vhs-voice
生产启动：./bin/vhs-voice

环境变量：
VHS_VOICE_ADDR  监听地址，默认 127.0.0.1:8950
VHS_UPSTREAM    上游主 harness，默认 http://127.0.0.1:8941
VHS_DATA_DIR    数据目录，默认 ./data

示例：
VHS_VOICE_ADDR=127.0.0.1:8950 VHS_UPSTREAM=http://127.0.0.1:8941 VHS_DATA_DIR=./data go run ./cmd/vhs-voice

四、端点清单（须全部注册）
GET  /v1/voice/health
POST /v1/voice/parse
POST /v1/voice/decompose
POST /v1/voice/resolve
POST /v1/voice/run
GET  /v1/voice/tasks/{conversation_id}
GET  /v1/tasks
GET  /v1/tasks/{id}

P0 附加端点：
GET  /v1/health
POST /v1/process

典型调用：
健康检查：curl http://127.0.0.1:8950/v1/voice/health
去噪提意：curl -X POST http://127.0.0.1:8950/v1/voice/parse -d '{"text":"就是那个，帮我跑一下测试对吧"}'
拆单任务：curl -X POST http://127.0.0.1:8950/v1/voice/decompose -d '{"clean":"拉取 GitHub 仓库 xxx 然后跑测试"}'
补全投递：curl -X POST http://127.0.0.1:8950/v1/voice/run -d '{"conversation_id":"c1","text":"..."}'
会话回看：curl http://127.0.0.1:8950/v1/voice/tasks/c1

五、数据文件（全部 append-only，UTF-8）
<dataDir>/voice_sessions/<conversation_id>.jsonl   会话记忆，可审计
<dataDir>/feedback.jsonl                           反馈学习（✔/✘ 回馈）
<dataDir>/traces.jsonl                             调用轨迹
<dataDir>/usage.jsonl                              用量统计
<dataDir>/lexicon.json                             个性化词典（增/删/查条目）

数据目录由 VHS_DATA_DIR 指定，独立于代码目录，可直接备份或归档。

六、能力要点
个性化词典：增/删/查，含匹配与纠错安全，避免正常文本被改坏。
文本纠错：清洗 + 词典纠错两段式。
意图分类：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE 五类。
中文全链路 UTF-8 不乱码。
鉴权可占位，但必须存在。