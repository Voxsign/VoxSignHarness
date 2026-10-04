README.md —— 语音适配层（vhs-voice）

一、服务简介
语音适配层是独立 Go 服务（cmd/vhs-voice），仅用标准库、零第三方依赖。手机端语音识别文本先进入本层：去噪提意 → 复合指令拆单任务 → 域/对象补全 → 逐个投递上游主 harness → 语音友好汇总，避免直接投递触发 need_ask 三态拒绝。

二、构建与启动
构建：
go build ./...

启动（默认监听 127.0.0.1:8950，上游 http://127.0.0.1:8941）：
go run ./cmd/vhs-voice

指定参数启动：
go run ./cmd/vhs-voice -addr 127.0.0.1:8950 -data-dir ./data -upstream http://127.0.0.1:8941

可用环境变量覆盖默认值：
VHS_VOICE_ADDR  监听地址，默认 127.0.0.1:8950
VHS_UPSTREAM    上游主 harness 地址，默认 http://127.0.0.1:8941

约束：仅监听 127.0.0.1，非回环地址拒绝；鉴权头可占位但必须存在。

三、HTTP 端点
GET  /v1/health                        基础健康检查
POST /v1/process                       JSON 请求/响应主处理入口
GET  /v1/tasks                         任务列表
GET  /v1/tasks/{id}                    单任务查询
GET  /v1/voice/health                  语音层健康检查，返回 ok/service/upstream/sessions
POST /v1/voice/parse                   口语去噪提意，出参 clean/actions/noise_removed
POST /v1/voice/decompose               复合指令拆单任务，出参 tasks 列表（含 seq/action/target）
POST /v1/voice/resolve                 域/对象补全
POST /v1/voice/run                     编排执行（核心），投递上游并落盘
GET  /v1/voice/tasks/{conversation_id} 按会话回读执行结果，返回 count=已执行任务数

四、数据文件（独立数据目录，data-dir 可配；JSONL 均为 append-only）
<data-dir>/voice_sessions/<conversation_id>.jsonl   会话记忆，可审计
<data-dir>/feedback.jsonl                           反馈学习记录（✔/✘ 回馈落盘）
<data-dir>/traces.jsonl                             调用链路追踪
<data-dir>/usage.jsonl                              用量统计

五、快速验证
curl http://127.0.0.1:8950/v1/voice/health
curl -X POST http://127.0.0.1:8950/v1/voice/parse -d '{"text":"那个，帮我拉取 GitHub 仓库 smithpeter/voicesi 然后测试一下"}'
curl -X POST http://127.0.0.1:8950/v1/voice/run -d '{"conversation_id":"c1","text":"..."}'
curl http://127.0.0.1:8950/v1/voice/tasks/c1

六、注意事项
上游仅调用 POST /v1/tasks 与 GET /v1/tasks/{id} 两个接口。
全链路 UTF-8，中文不乱码；无第三方依赖、无占位实现。