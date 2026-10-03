个性化后台实现

启动
默认只监听 127.0.0.1；若 -addr 配置为非回环地址，服务拒绝启动。

开发运行：
go run ./cmd/server -addr 127.0.0.1:8080 -data-dir ./data -auth-token dev-token

编译运行：
go build -o personalized-backend ./cmd/server
./personalized-backend -addr 127.0.0.1:8080 -data-dir ./data -auth-token dev-token

鉴权
除 GET /v1/health 外，请求需带 Authorization: Bearer dev-token。
鉴权可占位，但必须校验；缺失或错误返回 401。

HTTP 端点
以下端点均已在启动时注册 http.HandleFunc。

GET  /v1/health        健康检查
POST /v1/process       JSON 请求/响应：文本纠错 + 意图分类
                       意图：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
GET  /v1/dict          查询个性化词典，?term=关键词
POST /v1/dict          新增/更新词典条目，JSON: {"term":"...","replacement":"...","weight":1}
DELETE /v1/dict        删除词典条目，?term=关键词
GET  /v1/term          术语匹配与纠错安全查询，?text=文本；返回候选，不自动改写
POST /v1/correct       清洗 + 词典纠错，JSON: {"text":"..."}；正常文本不被改坏
POST /v1/feedback      反馈落盘，JSON: {"trace_id":"...","verdict":"✔|✘"} 或 up/down
GET  /v1/blacklist     查询黑名单
POST /v1/blacklist     新增黑名单，JSON: {"term":"..."}
DELETE /v1/blacklist   删除黑名单，?term=关键词

数据文件
-data-dir 默认 ./data，可配置，独立数据目录。
所有记录 append-only，不覆盖历史。

traces.jsonl      请求处理轨迹
usage.jsonl       调用用量
feedback.jsonl    反馈学习记录
dict.jsonl        个性化词典事件，启动时重放
blacklist.jsonl   黑名单事件，启动时重放

请求/响应示例
POST /v1/process
请求：{"text":"记一下明天开会","user_id":"u1"}
响应：{"intent":"NOTE","corrected":"记一下明天开会","trace_id":"..."}

POST /v1/correct
请求：{"text":"开恵"}
响应：{"cleaned":"开恵","corrected":"开会","changed":true}

POST /v1/feedback
请求：{"trace_id":"...","verdict":"✔"}
响应：{"ok":true}