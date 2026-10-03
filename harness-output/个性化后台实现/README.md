个性化后台实现 README 运行说明

运行
1. 准备数据目录：mkdir -p ./data
2. 启动服务：go run . --addr 127.0.0.1:8080 --data-dir ./data
   或先编译：go build -o personalized-backend . && ./personalized-backend --addr 127.0.0.1:8080 --data-dir ./data
3. 服务仅监听 127.0.0.1；传入非回环地址会拒绝启动。
4. 鉴权为占位实现：除健康检查外，请求头需带 Authorization: Bearer dev-token。可通过 --auth-token 覆盖占位值。

HTTP 端点
GET /v1/health
用途：健康检查。
响应示例：{"status":"ok"}

POST /v1/process
用途：文本清洗与纠错、意图分类、个性化词典增删查、反馈学习。
请求头：Content-Type: application/json；Authorization: Bearer dev-token
请求示例：{"text":"帮我记一下明天开会"}
响应示例：{"intent":"NOTE","corrected_text":"帮我记一下明天开会","dictionary_hits":[],"trace_id":"..."}
反馈示例：{"text":"帮我记一下明天开会","feedback":"✔"}
词典操作示例：{"dict_action":"add","key":"开会","value":"会议"}；dict_action 支持 add/delete/get。

数据文件
默认数据目录为启动参数 --data-dir 指定值，未指定时使用 ./data。
feedback.jsonl：反馈学习记录，append-only。
traces.jsonl：处理追踪记录，append-only。
usage.jsonl：调用统计记录，append-only。
dictionary.json：个性化词典持久化文件，位于同一数据目录。

验证
curl http://127.0.0.1:8080/v1/health
curl -X POST http://127.0.0.1:8080/v1/process -H "Content-Type: application/json" -H "Authorization: Bearer dev-token" -d '{"text":"帮我记一下明天开会"}'