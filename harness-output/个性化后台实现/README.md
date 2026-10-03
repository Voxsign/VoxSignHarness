个性化后台实现 README

启动
开发运行：
go run . --addr 127.0.0.1:8080 --data-dir ./data

编译后运行：
go build -o personalized-backend .
./personalized-backend --addr 127.0.0.1:8080 --data-dir ./data

服务仅监听 127.0.0.1；传入非回环地址应拒绝启动。鉴权为占位但保留校验入口，请求头使用 Authorization: Bearer <token>。

端点
GET /v1/health
健康检查，返回 {"ok":true}。

POST /v1/process
JSON 请求/响应，执行文本清洗、词典纠错、意图分类；feedback 为 true/false 时追加反馈学习。意图分类为 NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE。

示例：
curl -s http://127.0.0.1:8080/v1/health
curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -H 'Authorization: Bearer dev-token' -d '{"text":"这各方案有问题","feedback":null}'

响应示例：
{"intent":"EDIT","corrected_text":"这个方案有问题","dictionary_hits":["这各->这个"],"trace_id":"..."}

数据文件
--data-dir 默认 ./data，可配置。

dictionary.json：个性化词典条目，支持增/删/查。
traces.jsonl：处理轨迹，append-only。
usage.jsonl：调用用量，append-only。
feedback.jsonl：✔/✘ 反馈回馈，append-only。

注意：若 main.go 仍为骨架、含 todo 或 go build 失败，需先补齐 P0 实现后再按上述命令运行。