# 个性化后台实现

## 启动
依赖 Go 1.21+。

编译：
go build -o personalized-backend .

运行：
./personalized-backend --addr 127.0.0.1:8080 --data-dir ./data

开发运行：
go run . --addr 127.0.0.1:8080 --data-dir ./data

默认只监听 127.0.0.1；非回环地址拒绝启动。鉴权占位：Authorization: Bearer <token>，可通过配置或环境变量启用。

## 端点
GET /v1/health：健康检查，返回 {"status":"ok"}。

POST /v1/process：统一业务入口，JSON 请求/响应。通过 action 调用词典增/删/查、文本纠错、意图分类（NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE）、反馈学习。响应含 corrected、intent、trace_id 等。

示例：
curl http://127.0.0.1:8080/v1/health

curl -X POST http://127.0.0.1:8080/v1/process -H "Content-Type: application/json" -d "{\"action\":\"classify\",\"text\":\"记一下明天开会\"}"

curl -X POST http://127.0.0.1:8080/v1/process -H "Content-Type: application/json" -d "{\"action\":\"feedback\",\"trace_id\":\"...\",\"ok\":true}"

## 数据文件
默认数据目录 ./data，可用 --data-dir 指定。

data/dictionary.json：个性化词典条目。
data/feedback.jsonl：反馈 ✔/✘，append-only。
data/traces.jsonl：请求轨迹，append-only。
data/usage.jsonl：调用用量，append-only。

文件首次写入自动创建；JSONL 只追加、不覆盖。