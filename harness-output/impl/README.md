README.md · VHS 语音适配层

服务简介
独立 Go 服务，只依赖标准库，零第三方依赖。把手机端语音识别文本去噪提意、拆单任务、域对象补全后，逐个投递上游主 harness，并做语音友好汇总。默认只监听 127.0.0.1。

构建与启动
go build ./...
go run ./cmd/vhs-voice
或直接运行二进制：
./vhs-voice -addr 127.0.0.1:8950 -data-dir ./data

可配置项（flag 优先，环境变量兜底）
-addr      监听地址，默认 127.0.0.1:8950（环境变量 VHS_VOICE_ADDR 可覆盖，非回环地址拒绝）
-data-dir  数据目录，默认 ./data
VHS_UPSTREAM  上游主 harness 地址，默认 http://127.0.0.1:8941，仅调用其 POST /v1/tasks 与 GET /v1/tasks/{id}

端点清单
GET  /v1/health                     健康检查
POST /v1/process                    JSON 请求/响应的统一处理入口
GET  /v1/tasks                      任务列表
GET  /v1/tasks/{id}                 任务详情
POST /v1/voice/health               语音层健康检查，返回 ok/service/upstream/sessions
POST /v1/voice/parse                口语去噪提意，返回 clean/actions/noise_removed
POST /v1/voice/decompose            复合指令拆单任务，返回 tasks 数组（seq/action/target）
POST /v1/voice/resolve              指代与对象补全；无会话历史且无明确对象时返回 unresolved/pending_resolve
POST /v1/voice/run                  编排任务并投递上游 harness，响应含 task_id/summary/total 等任务引用
GET  /v1/voice/tasks/{conversation_id}  按会话回读任务状态

典型调用顺序
1. POST /v1/voice/parse        { "text": "<口语原文>" }
2. POST /v1/voice/decompose    { "clean": "<去噪后文本>" }
3. POST /v1/voice/resolve      { "conversation_id": "...", "target": "..." }
4. POST /v1/voice/run          投递上游，返回 task_id 列表
5. GET  /v1/voice/tasks/{conversation_id}  轮询汇总

数据文件（全部 JSONL，append-only，UTF-8）
<dataDir>/voice_sessions/<conversation_id>.jsonl   会话记忆，可审计
<dataDir>/feedback.jsonl                            ✔/✘ 反馈学习记录
<dataDir>/traces.jsonl                              处理轨迹
<dataDir>/usage.jsonl                               调用用量
<dataDir>/dictionary.jsonl                          个性化词典增删查条目

说明
- 会话记忆按 conversation_id 分文件，无历史时不臆造对象，直接进入 pending_resolve。
- 上游不可达时 run 返回明确错误，不静默吞掉，任务状态保持可回读。
- 文本纠错只做清洗 + 词典纠错，正常文本原样返回，不会被改坏。