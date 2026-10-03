# 个性化后台实现

## 启动
go run . --addr 127.0.0.1:8080 --data-dir ./data --api-key local-dev-token

或编译后：
go build -o personal-backend .
./personal-backend --addr 127.0.0.1:8080 --data-dir ./data --api-key local-dev-token

服务仅监听 127.0.0.1；使用非回环地址会拒绝启动。鉴权头占位：Authorization: Bearer local-dev-token，可用 --api-key 修改。

## 端点
GET /v1/health
健康检查。返回 {"status":"ok"}。

POST /v1/process
文本清洗、词典纠错、意图分类、反馈记录。请求与响应均为 JSON。
请求头：Authorization: Bearer local-dev-token；Content-Type: application/json。
请求示例：{"text":"明天下午三点提醒我开会","feedback":null}
反馈示例：{"text":"明天下午三点提醒我开会","feedback":true}
意图分类：NOTE、QUERY、EDIT、COMMIT、ORCHESTRATE。
响应示例：{"intent":"NOTE","corrected":"明天下午三点提醒我开会","dictionary_hits":[],"trace_id":"..."}

## 数据文件
默认数据目录：./data，可用 --data-dir 修改，启动时自动创建。
dictionary.json：个性化词典条目，支持增/删/查。
feedback.jsonl：反馈学习，append-only，✔/✘ 回执落盘。
traces.jsonl：处理轨迹，append-only。
usage.jsonl：调用用量，append-only。
JSONL 只追加，不覆盖。