个性化后台 运行说明

构建
go build -o personalized-backend .

启动
./personalized-backend -addr 127.0.0.1:8080 -data-dir ./data -auth-token dev-token

开发运行
go run . -addr 127.0.0.1:8080 -data-dir ./data -auth-token dev-token

说明
服务仅允许监听 127.0.0.1。传入非回环地址会拒绝启动。
所有请求需带鉴权占位头：X-Auth-Token: dev-token。可用 -auth-token 修改；未配置时默认 dev-token。

HTTP 端点
GET /v1/health
用途：健康检查。
响应：{"status":"ok"}

POST /v1/process
用途：文本清洗、词典纠错、意图分类；可选反馈。
请求 JSON 示例：
{"text":"明天下午三点开会","user_id":"u1","action":"auto"}
反馈示例：
{"text":"明天下午三点开会","user_id":"u1","feedback":"ok","trace_id":"..."}

响应 JSON 示例：
{"trace_id":"...","intent":"NOTE","corrected_text":"明天下午三点开会","matched":[],"changed":false}

intent 取值：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
feedback 取值：ok / bad

数据文件
默认位于 -data-dir 指定目录，示例 ./data：
dictionary.json   个性化词典，增删查后的当前快照
feedback.jsonl    反馈学习记录，append-only
traces.jsonl      处理轨迹，append-only
usage.jsonl       用量记录，append-only

JSONL 每行一个 JSON 对象，含 ts、trace_id、user_id、intent、text 等字段。写入采用追加模式，不覆盖历史。备份或迁移时直接复制整个 data-dir。

注意
鉴权为占位实现，生产部署前请替换为真实鉴权。服务仅监听回环地址，不对外暴露。