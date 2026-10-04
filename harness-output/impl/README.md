README · 语音适配层（vhs-voice）

一、服务形态
独立 Go 服务，入口 cmd/vhs-voice，仅标准库，无第三方依赖。
默认监听 127.0.0.1:8950，环境变量 VHS_VOICE_ADDR 可覆盖（如 127.0.0.1:9000）。
上游主 harness 默认 http://127.0.0.1:8941，环境变量 VHS_UPSTREAM 可覆盖。
适配层仅调用上游的 POST /v1/tasks 与 GET /v1/tasks/{id}。
数据目录由 -data-dir 指定（默认 ./data），JSONL 均为 append-only，可审计。

二、启动命令
1) 编译全部：
go build ./...
2) 直接运行：
go run ./cmd/vhs-voice -data-dir ./data
3) 编译后运行：
go build -o vhs-voice ./cmd/vhs-voice
./vhs-voice -data-dir ./data
4) 指定端口与上游：
VHS_VOICE_ADDR=127.0.0.1:8950 VHS_UPSTREAM=http://127.0.0.1:8941 ./vhs-voice -data-dir ./data

三、HTTP 端点（全部已注册，无 501 占位）
GET  /v1/health
POST /v1/process
GET  /v1/voice/health
POST /v1/voice/parse
POST /v1/voice/decompose
POST /v1/voice/resolve
POST /v1/voice/run
GET  /v1/voice/tasks/{conversation_id}
另兼容任务读取：GET /v1/tasks、GET /v1/tasks/{id}

四、常用调用示例（文本描述）
健康检查：curl http://127.0.0.1:8950/v1/voice/health
口语解析：POST /v1/voice/parse，body {"text":"就是那个，帮我跑一下测试"}
复合拆单：POST /v1/voice/decompose，body {"clean":"拉取 GitHub 仓库 smithpeter/voicesi 并跑测试"}
对象补全：POST /v1/voice/resolve
编排执行：POST /v1/voice/run
任务回读：GET /v1/voice/tasks/{conversation_id}，返回 count 为已执行任务数

五、数据文件（位于 -data-dir 指定目录）
voice_sessions/<conversation_id>.jsonl  会话记忆与编排记录
traces.jsonl                            调用链/处理轨迹
usage.jsonl                             用量记录
feedback.jsonl                          反馈学习（✔/✘ 回馈，append-only）
dictionary.json                         个性化词典条目（增/删/查）
upstream_calls.jsonl                    对上游主 harness 的投递与轮询记录

六、行为要点
只监听回环地址，非回环请求拒绝；鉴权为占位但已保留校验位。
parse 删除填充词并提取动作/对象；decompose 将复合指令拆为单任务；resolve 补全域与对象；run 逐个投递上游并汇总为语音友好文本。
文本纠错仅做清洗与词典纠错，正常文本不被改坏；中文全链路 UTF-8，无乱码。
无第三方依赖、无 TODO/panic 占位逻辑。