README.md

服务名：语音适配层（vhs-voice）
作用：接收手机端语音识别文本，去噪提意、复合指令拆单任务、域/对象补全，再逐个投递上游主 harness，最后做语音友好汇总。

一、启动
默认监听 127.0.0.1:8950，上游默认 http://127.0.0.1:8941。
go build ./...
go run ./cmd/vhs-voice

可选环境变量：
VHS_VOICE_ADDR=127.0.0.1:8950
VHS_UPSTREAM=http://127.0.0.1:8941
VHS_VOICE_DATA_DIR=./data

只监听回环地址；非 127.0.0.1 请求拒绝。鉴权可先占位，但入口必须保留校验位。

二、端点
GET  /v1/voice/health
POST /v1/voice/parse
POST /v1/voice/decompose
POST /v1/voice/resolve
POST /v1/voice/run
GET  /v1/voice/tasks/{cid}
GET  /v1/voice/tasks/{conversation_id}
GET  /v1/tasks
GET  /v1/tasks/{id}

说明：
/v1/voice/tasks/{cid} 与 /v1/voice/tasks/{conversation_id} 指向同一路由能力，必须注册，用于按会话 ID 查询任务。
上游只调用 POST /v1/tasks 与 GET /v1/tasks/{id}。

基础能力端点：
GET  /v1/health
POST /v1/process

三、数据文件
数据目录默认 ./data，可用 VHS_VOICE_DATA_DIR 覆盖。
voice_sessions/<conversation_id>.jsonl
 会话记忆，append-only，可审计。
feedback.jsonl
 反馈学习，✔/✘ 回馈落盘，append-only。
traces.jsonl
 调用链与处理轨迹，append-only。
usage.jsonl
 使用统计，append-only。
个性化词典文件位于同一数据目录，支持增/删/查。

四、调用顺序
1. POST /v1/voice/parse 去噪提意
2. POST /v1/voice/decompose 拆单任务
3. POST /v1/voice/resolve 补全域/对象
4. POST /v1/voice/run 投递上游
5. GET /v1/voice/tasks/{conversation_id} 查询会话任务
6. GET /v1/voice/health 健康检查

五、验证
go build ./...
go test ./...
curl http://127.0.0.1:8950/v1/voice/health
curl -X POST http://127.0.0.1:8950/v1/voice/parse -H 'Content-Type: application/json' -d '{"text":"那个，就是帮我拉取 GitHub 仓库 smithpeter/voicesi"}'