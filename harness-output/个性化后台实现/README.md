个性化后台实现

运行
环境：Go 1.20+。
启动：
go run . -addr 127.0.0.1:8080 -data-dir ./data -token dev-token

编译后启动：
go build -o personal-backend
./personal-backend -addr 127.0.0.1:8080 -data-dir ./data -token dev-token

仅允许监听 127.0.0.1；addr 为非回环地址时拒绝启动。
鉴权占位：请求头 Authorization: Bearer dev-token。

端点
GET /v1/health
健康检查，返回 {"status":"ok"}。

POST /v1/process
统一业务入口，Content-Type: application/json。
请求示例：{"action":"process","text":"明天下午三点提醒我"}
响应示例：{"intent":"NOTE","corrected_text":"明天下午三点提醒我","trace_id":"..."}
action 支持：process、classify、correct、dict_add、dict_del、dict_get、feedback。
反馈示例：{"action":"feedback","trace_id":"...","value":"✔"}，也接受 "✘"。
词典示例：{"action":"dict_add","term":"会议","replacement":"例会"}；{"action":"dict_del","term":"会议"}；{"action":"dict_get","term":"会议"}。

数据文件
默认数据目录为 ./data，可用 -data-dir 修改。目录不存在时自动创建。
feedback.jsonl：反馈学习记录，append-only，记录 ✔/✘。
traces.jsonl：处理轨迹记录，append-only。
usage.jsonl：用量记录，append-only。
dictionary.json：个性化词典条目，支持增/删/查。