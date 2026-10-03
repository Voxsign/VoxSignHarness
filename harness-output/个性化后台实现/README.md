个性化后台实现 README

运行环境
Go 1.20+。默认仅监听 127.0.0.1:8080，拒绝非回环地址。鉴权为占位，请求头需带 Authorization: Bearer <token>。

启动命令
go run . --addr 127.0.0.1:8080 --data-dir ./data

或编译后启动：
go build -o personal-backend .
./personal-backend --addr 127.0.0.1:8080 --data-dir ./data

HTTP 端点
GET /v1/health
健康检查，返回 {"ok":true}。

POST /v1/process
文本处理，JSON 请求/响应。请求示例：
{"text":"明天提醒我交报告"}
响应包含清洗纠错后的文本、意图分类 NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE 及处理结果。

反馈回馈示例：
{"feedback":{"id":"<trace-id>","label":"ok"}}
label 取 ok / fail 或 true / false，落盘 feedback.jsonl。

请求头：
Authorization: Bearer <token>
Content-Type: application/json

数据文件
默认目录 ./data，可用 --data-dir 指定；目录不存在时自动创建。
data/dictionary.json  个性化词典条目，支持增/删/查。
data/feedback.jsonl   反馈学习记录，append-only。
data/traces.jsonl     处理轨迹，append-only。
data/usage.jsonl      用量记录，append-only。

注意：JSONL 只追加不覆盖；服务不监听 0.0.0.0 或外部地址。