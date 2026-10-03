个性化后台实现 README

环境：Go 1.22+。服务只监听 127.0.0.1，默认 127.0.0.1:8080；指定非回环地址应拒绝启动。鉴权为占位：请求头 Authorization: Bearer dev-token，默认 token 可用 AUTH_TOKEN 或 --auth-token 配置。

启动：
go run . --addr 127.0.0.1:8080 --data-dir ./data --auth-token dev-token
或
go build -o personalized-backend . && DATA_DIR=./data AUTH_TOKEN=dev-token ./personalized-backend

端点：
GET /v1/health：健康检查，返回 {"ok":true}。
POST /v1/process：主入口，JSON 请求/响应，需鉴权。请求体示例：{"action":"process","text":"明天下午三点开会","feedback":null}。action 支持 process、dict.add、dict.remove、dict.list、feedback。响应含 ok、intent（NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE）、corrected、request_id 等。

数据文件：
默认目录 DATA_DIR 或 --data-dir，默认 ./data。JSONL 均按 append-only 追加：
feedback.jsonl：反馈学习记录
traces.jsonl：请求/处理轨迹
usage.jsonl：调用用量
dictionary.jsonl：个性化词典事件/条目，如实现为快照则为 dictionary.json

自检：
go build ./... 必须成功，main.go 不得保留 todo/占位。
curl http://127.0.0.1:8080/v1/health
curl -H "Authorization: Bearer dev-token" -H "Content-Type: application/json" -d '{"action":"process","text":"明天下午三点开会"}' http://127.0.0.1:8080/v1/process