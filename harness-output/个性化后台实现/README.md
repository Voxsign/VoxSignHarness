个性化后台实现 README

启动
默认监听 127.0.0.1:8080，数据目录 ./data：
go run . -addr 127.0.0.1:8080 -data-dir ./data

已编译：
./personal-backend -addr 127.0.0.1:8080 -data-dir ./data

环境变量：
AUTH_TOKEN=dev-token
DATA_DIR=./data

只允许监听 127.0.0.1；0.0.0.0 等非回环地址启动即拒绝。

鉴权
除 GET /v1/health 外，请求头需带 X-API-Key: dev-token 或 Authorization: Bearer dev-token。

端点
GET  /v1/health：健康检查。
POST /v1/process：JSON 处理，返回清洗、纠错、意图等结果。
GET  /v1/dict：查词典；POST /v1/dict：增/删/查，JSON action=add|delete|list。
GET  /v1/term：查单条；POST /v1/term：增/删/查单条。
POST /v1/correct：清洗 + 词典纠错，正常文本不被改坏。
POST /v1/feedback：反馈落盘，label=✔/✘。
GET  /v1/blacklist：查黑名单；POST /v1/blacklist：增/删/查黑名单。

数据文件
默认 data-dir=./data，可用 -data-dir 或 DATA_DIR 配置。
dict.jsonl：个性化词典条目。
blacklist.jsonl：黑名单条目。
feedback.jsonl：反馈 append-only。
traces.jsonl：处理轨迹 append-only。
usage.jsonl：用量 append-only。

请求示例
curl -s http://127.0.0.1:8080/v1/health
curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -H 'X-API-Key: dev-token' -d '{"text":"..."}'