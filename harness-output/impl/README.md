语音适配层（vhs-voice）运行说明

一、构建与启动
  go build ./...
  go run ./cmd/vhs-voice
  或直接运行产物：./vhs-voice

  默认监听 127.0.0.1:8950，只绑定回环地址，非回环来源拒绝（返回 403）。
  启动参数（与环境变量等价，flag 优先）：
    -addr      监听地址，等同于 VHS_VOICE_ADDR，默认 127.0.0.1:8950
    -data-dir  数据目录，默认 ./data
    -upstream  上游主 harness，等同于 VHS_UPSTREAM，默认 http://127.0.0.1:8941
  适配层只调用上游的 POST /v1/tasks 与 GET /v1/tasks/{id}。

二、端点清单（全部已注册，启动时打印路由表）
  GET  /v1/voice/health                    健康检查，返回 ok/service/upstream/sessions
  POST /v1/voice/parse                     口语去噪提意，入参 {"text":"..."}
  POST /v1/voice/decompose                 复合指令拆单任务，入参 {"clean":"..."}
  POST /v1/voice/resolve                   指代与对象补全；无会话历史且无明确对象时返回
                                           unresolved=true / status=pending_resolve，
                                           不返回空数组了事
  POST /v1/voice/run                       端到端执行：解析→拆解→补全→逐个投递上游
  GET  /v1/voice/tasks/{conversation_id}   查询某会话下已投递任务及状态
  POST /v1/tasks                           上游任务创建（透传）
  GET  /v1/tasks/{id}                      上游任务查询（透传）
  GET  /v1/health                          P0 服务健康
  POST /v1/process                         P0 统一处理入口（JSON 请求/响应）

  鉴权：请求头 Authorization 为占位校验，未配置密钥时不拦截，配置后校验。

三、P0 能力
  个性化词典增/删/查；文本清洗加词典纠错（正常文本不改坏）；
  意图分类 NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE；
  反馈学习以 ✔/✘ 落盘 feedback.jsonl；traces、usage 同步落盘。

四、数据文件（均在 data-dir 下，JSONL append-only，UTF-8，可直接审计）
  <dataDir>/voice_sessions/<conversation_id>.jsonl   会话记忆，逐条追加
  <dataDir>/feedback.jsonl                           反馈回流
  <dataDir>/traces.jsonl                             调用轨迹
  <dataDir>/usage.jsonl                              用量统计

五、最小自检
  curl -s http://127.0.0.1:8950/v1/voice/health
  curl -s -X POST http://127.0.0.1:8950/v1/voice/parse -d '{"text":"那个，就是帮我跑一下测试"}'
  curl -s -X POST http://127.0.0.1:8950/v1/voice/decompose -d '{"clean":"拉取 GitHub 仓库 a/b 并跑测试"}'
  curl -s -X POST http://127.0.0.1:8950/v1/voice/resolve -d '{"conversation_id":"c1"}'
  curl -s -X POST http://127.0.0.1:8950/v1/voice/run -d '{"conversation_id":"c1","text":"帮我部署服务 x"}'
  curl -s http://127.0.0.1:8950/v1/voice/tasks/c1

六、约束
  仅标准库，零第三方依赖；无 TODO 占位；全链路 UTF-8 中文不乱码。