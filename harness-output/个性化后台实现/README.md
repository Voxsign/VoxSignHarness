# 个性化后台实现

前置：确保 go build 通过，main.go 无 todo/占位。

构建：
go build -o personal-backend .

启动：
./personal-backend --addr 127.0.0.1:8080 --data-dir ./data --auth-token dev-token

仅监听 127.0.0.1；addr 非回环会拒绝启动。鉴权为占位，请求需带 Authorization: Bearer dev-token。

端点：
GET /v1/health：健康检查。
POST /v1/process：JSON 请求/响应；执行清洗、词典纠错、意图分类。intent 取值为 NOTE、QUERY、EDIT、COMMIT、ORCHESTRATE。

请求示例：
{"user_id":"u1","text":"要 note 一下","action":"auto"}

响应示例：
{"request_id":"...","cleaned_text":"...","intent":"NOTE","dictionary_hits":[],"corrected":false}

反馈：提交 ✔/✘ 后追加写入 feedback.jsonl，不重写历史。

数据文件，默认 ./data，可用 --data-dir 修改：
dictionary.json：个性化词典条目。
traces.jsonl：处理轨迹，append-only。
usage.jsonl：调用用量，append-only。
feedback.jsonl：反馈学习，append-only。

快速验证：
curl http://127.0.0.1:8080/v1/health
curl -X POST http://127.0.0.1:8080/v1/process -H "Content-Type: application/json" -H "Authorization: Bearer dev-token" -d '{"user_id":"u1","text":"要 note 一下"}'