个性化后台实现 README

启动：
go build -o personalized-backend .
./personalized-backend -addr 127.0.0.1:8080 -data-dir ./data
开发运行：
go run . -addr 127.0.0.1:8080 -data-dir ./data

服务仅监听 127.0.0.1；绑定 0.0.0.0 等非回环地址应拒绝。鉴权为占位实现，请求头使用 Authorization: Bearer dev-token。

端点：
GET /v1/health
返回健康状态，示例：{"ok":true}

POST /v1/process
Content-Type: application/json
请求体示例：
{"text":"帮我记一下明天开会","dictionary":{"op":"list"}}
支持能力：dictionary 增/删/查；correct 文本清洗与词典纠错，正常文本不被改坏；intent 分类为 NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE；feedback 反馈回馈。响应含 corrected_text、intent、trace_id、dictionary_result。

调用示例：
curl -s http://127.0.0.1:8080/v1/health
curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -H 'Authorization: Bearer dev-token' -d '{"text":"帮我记一下明天开会"}'

数据文件：
目录由 -data-dir 指定，默认 ./data。
feedback.jsonl：反馈学习记录，append-only。
traces.jsonl：处理轨迹，append-only。
usage.jsonl：调用用量，append-only。
dictionary.json：个性化词典持久化，支持增/删/查。