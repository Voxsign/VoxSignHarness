个性化后台实现

启动
go run . -addr 127.0.0.1:8080 -data-dir ./data
或
go build -o personalized-backend .
./personalized-backend -addr 127.0.0.1:8080 -data-dir ./data

环境变量
ADDR=127.0.0.1:8080
DATA_DIR=./data
AUTH_TOKEN=dev-token

监听与鉴权
仅监听 127.0.0.1；传入 0.0.0.0、局域网 IP 等非回环地址时拒绝启动或拒绝绑定。
鉴权为占位但必须携带，支持：
Authorization: Bearer dev-token
或
X-API-Key: dev-token

端点
GET /v1/health
健康检查。
示例：
curl -s http://127.0.0.1:8080/v1/health -H "Authorization: Bearer dev-token"
响应：
{"status":"ok"}

POST /v1/process
统一处理文本：清洗、词典纠错、意图分类、反馈学习、JSONL 落盘。
请求头：
Content-Type: application/json
Authorization: Bearer dev-token
请求体示例：
{"text":"明天下午三点提醒我开会","feedback":null}
响应体示例：
{"ok":true,"intent":"NOTE","corrected_text":"明天下午三点提醒我开会","matched_terms":[],"trace_id":"..."}
意图枚举：
NOTE、QUERY、EDIT、COMMIT、ORCHESTRATE

P0 能力
个性化词典：增/删/查条目，含匹配与纠错安全。
文本纠错：清洗 + 词典纠错，正常文本不被改坏。
意图分类：NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE 五类。
反馈学习：✔/✘ 回馈追加写入 feedback.jsonl，append-only。
数据落盘：traces、usage 等 JSONL 追加写入，append-only。
数据目录独立，可用 -data-dir 或 DATA_DIR 配置。

数据文件
默认目录：./data
data/dictionary.json：个性化词典，启动加载，变更落盘。
data/feedback.jsonl：反馈学习记录，append-only。
data/traces.jsonl：请求处理轨迹，append-only。
data/usage.jsonl：调用统计，append-only。

最小调用
go run . -addr 127.0.0.1:8080 -data-dir ./data
curl -s http://127.0.0.1:8080/v1/health -H "Authorization: Bearer dev-token"
curl -s http://127.0.0.1:8080/v1/process -H "Authorization: Bearer dev-token" -H "Content-Type: application/json" -d '{"text":"把这个词加入我的词典"}'

注意
启动参数名以实际 main.go 为准；若使用环境变量，则无需重复传参。
监听地址必须保持为 127.0.0.1。