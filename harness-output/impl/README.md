# 语音适配层（vhs-voice）运行说明

一、构建与启动

  构建：go build ./...
  启动：go run ./cmd/vhs-voice
  默认监听：127.0.0.1:8950（仅回环，非回环地址会被拒绝）
  上游主 harness：http://127.0.0.1:8941（仅调用其 POST /v1/tasks 与 GET /v1/tasks/{id}）

  可用参数与环境变量：
  -addr            监听地址，默认 127.0.0.1:8950，等价环境变量 VHS_VOICE_ADDR
  -data-dir        数据目录，默认 ./data，等价环境变量 VHS_VOICE_DATA_DIR
  VHS_UPSTREAM     上游地址，默认 http://127.0.0.1:8941

  示例：
  VHS_UPSTREAM=http://127.0.0.1:8941 go run ./cmd/vhs-voice -addr 127.0.0.1:8950 -data-dir ./data

二、端点清单（全部已注册）

  语音适配层
  GET  /v1/voice/health                     健康检查，返回 ok/service/upstream/sessions
  POST /v1/voice/parse                      口语去噪提意，入参 {"text":"..."}
  POST /v1/voice/decompose                  复合指令拆单任务，入参 {"clean":"..."}
  POST /v1/voice/resolve                    域/对象补全
  POST /v1/voice/run                        编排执行（核心），逐个投递上游并汇总
  GET  /v1/voice/tasks/{conversation_id}    会话任务回读，返回 count=已执行任务数

  通用能力
  GET  /v1/health                           本地健康检查
  POST /v1/process                          JSON 请求/响应主入口（清洗、纠错、意图分类、反馈）

  上游任务代理
  POST /v1/tasks                            透传上游创建任务
  GET  /v1/tasks/{id}                       透传上游查询任务

三、数据文件（独立数据目录，均 append-only，可审计）

  <dataDir>/voice_sessions/<conversation_id>.jsonl   会话记忆，按会话分文件
  <dataDir>/feedback.jsonl                           反馈学习，✔/✘ 回馈追加写入
  <dataDir>/traces.jsonl                             调用轨迹
  <dataDir>/usage.jsonl                              用量统计
  <dataDir>/dictionary.json                          个性化词典条目

四、说明

  全链路 UTF-8 中文，无第三方依赖，仅标准库实现。
  日志与落盘均为追加写，不覆盖历史记录。
  鉴权为占位实现，保留校验入口，默认本机回环调用即可通过。