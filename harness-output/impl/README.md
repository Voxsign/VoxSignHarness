语音适配层（vhs-voice）运行说明

一、启动
1. 编译：go build ./...（仅标准库，无第三方依赖）
2. 运行：go run ./cmd/vhs-voice
3. 监听：默认 127.0.0.1:8950，仅回环地址，非回环请求拒绝
   环境变量 VHS_VOICE_ADDR 可改监听地址，例如 VHS_VOICE_ADDR=127.0.0.1:9000
4. 上游主 harness：默认 http://127.0.0.1:8941，环境变量 VHS_UPSTREAM 可覆盖
   适配层只调用上游的 POST /v1/tasks 与 GET /v1/tasks/{id}
5. 数据目录：环境变量 VHS_VOICE_DATA_DIR（或 data-dir 参数）可配，默认 ./data
   启动前确保目录可写；所有落盘均为 append-only，可审计

二、端点
健康与通用
  GET  /v1/health
  GET  /v1/voice/health          返回 {"ok":true,"service":"vhs-voice","upstream":"...","sessions":N}
  POST /v1/process               JSON 请求/响应，主处理入口

语音链路
  POST /v1/voice/parse           入参 {"text":"..."}，出参 clean / actions / noise_removed
  POST /v1/voice/decompose       入参 {"clean":"..."}，出参 tasks[{seq,action,target,...}]
  POST /v1/voice/resolve         域/对象补全
  POST /v1/voice/run             逐个投递上游并汇总，返回语音友好结果
  GET  /v1/voice/tasks/{conversation_id}   查询某会话下的任务

上游任务代理
  GET  /v1/tasks
  GET  /v1/tasks/{id}

鉴权当前为占位实现，接口保留。

三、数据文件（均在数据目录下，JSONL，append-only）
  voice_sessions/<conversation_id>.jsonl   会话记忆，一会话一文件，可审计
  feedback.jsonl                           反馈学习，✔/✘ 回馈落盘
  traces.jsonl                             请求/调用链路记录
  usage.jsonl                              用量记录

四、快速自检
  1. 起服务后 GET /v1/voice/health 应返回 ok:true
  2. POST /v1/voice/parse 传一句中文口语，检查 clean 去噪、actions 提动作、noise_removed 列被删词
  3. POST /v1/voice/decompose 传复合指令，检查 tasks 按 seq 拆成单任务
  4. POST /v1/voice/run 跑通后，上游 /v1/tasks/{id} 可查到对应任务
  5. 检查数据目录下对应 JSONL 是否新增记录

五、注意
  全链路 UTF-8，中文不乱码；无 TODO 占位；端口被占用时先改 VHS_VOICE_ADDR。