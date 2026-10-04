语音适配层 vhs-voice · 运行说明

一、启动
  go build ./...
  go run ./cmd/vhs-voice
  仅标准库，零第三方依赖；启动后仅监听回环地址 127.0.0.1。

二、环境变量
  VHS_VOICE_ADDR   监听地址，默认 127.0.0.1:8950
  VHS_UPSTREAM     上游主 harness，默认 http://127.0.0.1:8941
  VHS_VOICE_DATA   数据目录，默认 ./data

三、HTTP 端点
  GET  /v1/voice/health                     健康检查，返回 ok/service/upstream/sessions
  GET  /v1/health                           通用健康检查
  POST /v1/voice/parse                      口语去噪提意
  POST /v1/voice/decompose                  复合指令拆单任务
  POST /v1/voice/resolve                    域/对象补全
  POST /v1/voice/run                        投递主 harness 并汇总
  GET  /v1/voice/tasks/{conversation_id}    会话任务查询
  GET  /v1/tasks                            主 harness 任务列表
  GET  /v1/tasks/{id}                       主 harness 单任务查询
  POST /v1/process                          文本处理（清洗+词典纠错+意图分类）

四、数据文件（均 append-only，可审计）
  <dataDir>/voice_sessions/<conversation_id>.jsonl   会话记忆
  <dataDir>/feedback.jsonl                           ✔/✘ 反馈学习
  <dataDir>/traces.jsonl                             调用轨迹
  <dataDir>/usage.jsonl                              用量统计

五、快速自检
  curl http://127.0.0.1:8950/v1/voice/health
  curl -X POST http://127.0.0.1:8950/v1/voice/parse -d '{"text":"就是那个，帮我拉取一下 GitHub 仓库 foo/bar 然后跑测试"}'
  curl -X POST http://127.0.0.1:8950/v1/voice/decompose -d '{"clean":"拉取 GitHub 仓库 foo/bar 并运行测试"}'