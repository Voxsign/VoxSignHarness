README.md

语音适配层（vhs-voice）

一、概述
手机端语音转文本后先进入本适配层，完成去噪提意、复合指令拆单任务、域/对象补全、逐个投递主 harness，并返回语音友好汇总。独立 Go 服务，仅标准库，零第三方依赖。

二、启动
默认监听 127.0.0.1:8950，只接受回环地址访问。
go build ./...
go run ./cmd/vhs-voice

可用环境变量覆盖：
VHS_VOICE_ADDR  监听地址，默认 127.0.0.1:8950
VHS_UPSTREAM    上游主 harness，默认 http://127.0.0.1:8941
VHS_DATA_DIR    数据目录，默认 ./data

上游仅调用其 POST /v1/tasks 与 GET /v1/tasks/{id}。

三、端点清单
健康与核心流程
GET  /v1/voice/health                    服务健康，返回 ok/service/upstream/sessions
POST /v1/voice/parse                     口语去噪提意，入参 {"text":"..."}
POST /v1/voice/decompose                 复合指令拆单任务，入参 {"clean":"..."}
POST /v1/voice/resolve                   域/对象补全
POST /v1/voice/run                       编排执行（核心：拆单→补全→逐个投递→汇总）
GET  /v1/voice/tasks/{conversation_id}   回读该会话已执行任务，count 为已执行任务数

P0 通用能力
GET  /v1/health                          服务健康
POST /v1/process                         JSON 请求/响应，含清洗、词典纠错、意图分类、反馈学习

上游代理（供手机端直查，转发至主 harness）
POST /v1/tasks
GET  /v1/tasks/{id}

四、P0 能力
个性化词典：增/删/查条目，含匹配与纠错安全，正常文本不被改坏
文本纠错：清洗 + 词典纠错
意图分类：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE 五类
反馈学习：✔/✘ 回馈落盘，append-only
落盘：traces、usage 等 JSONL 追加写

五、数据文件
<dataDir>/voice_sessions/<conversation_id>.jsonl   会话记忆，可审计，append-only
<dataDir>/feedback.jsonl                            反馈学习记录，append-only
<dataDir>/traces.jsonl                              调用链路追踪
<dataDir>/usage.jsonl                               用量统计
<dataDir>/dictionary.json                           个性化词典条目

全部落盘均为 JSONL 追加写，UTF-8 编码，中文全链路不乱码。