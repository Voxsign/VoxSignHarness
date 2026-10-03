个性化后台实现 运行说明

一、环境与构建
依赖：Go 1.21+
构建：go build -o p13n .
或直接运行：go run .

二、启动命令
默认启动（监听回环地址，数据目录 ./data）：
go run . --addr 127.0.0.1:8787 --data-dir ./data

常用参数：
--addr      监听地址，默认 127.0.0.1:8787；非回环地址（如 0.0.0.0:8787）会被直接拒绝启动
--data-dir  数据目录，默认 ./data，不存在时自动创建
--token     鉴权占位令牌，默认 dev-token

启动成功输出示例：
p13n listening on 127.0.0.1:8787, data-dir=./data

三、HTTP 端点
1. 健康检查
GET /v1/health
返回：{"status":"ok","version":"...","data_dir":"..."}

2. 业务处理
POST /v1/process
请求头：Authorization: Bearer dev-token（占位鉴权，缺失或错误返回 401）
请求体（JSON）：

{
  "text": "帮我把这条笔记存一下",
  "user_id": "u1",
  "feedback": null
}

字段说明：
text     必填，待处理文本
user_id  可选，用于分用户落盘
feedback 可选，✔/✘ 反馈回执，取值为 "up" 或 "down"

响应体（JSON）：

{
  "intent": "NOTE",
  "corrected": "帮我把这条笔记存一下",
  "matched_terms": [],
  "trace_id": "..."
}

intent 取值：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE

四、词典管理（HTTP）
GET    /v1/dict            查询全部条目
GET    /v1/dict?q=关键字   按词条查询
POST   /v1/dict            新增条目，体：{"term":"...","alias":["..."]}
DELETE /v1/dict?term=...   删除条目

匹配与纠错安全约定：仅做精确与别名匹配，长度阈值与相似度阈值不足时不做替换；正常文本原样返回，不被改坏。

五、数据文件（append-only，均位于 data-dir 下）
data/dictionary.json  个性化词典（原子整写，非 append）
data/feedback.jsonl   反馈学习记录，每行一条 {"ts","user_id","intent","text","feedback"}
data/traces.jsonl     处理链路追踪，每行一条 {"ts","trace_id","user_id","intent","stage"}
data/usage.jsonl      用量统计，每行一条 {"ts","user_id","endpoint","count"}

三个 .jsonl 文件均为追加写入，只增不改不删，可直接 tail -f data/traces.jsonl 观察。

六、快速验证
curl -s http://127.0.0.1:8787/v1/health
curl -s -X POST http://127.0.0.1:8787/v1/process -H "Authorization: Bearer dev-token" -H "Content-Type: application/json" -d '{"text":"帮我查一下昨天的记录","user_id":"u1"}'
curl -s -X POST http://127.0.0.1:8787/v1/process -H "Authorization: Bearer dev-token" -H "Content-Type: application/json" -d '{"text":"帮我查一下昨天的记录","user_id":"u1","feedback":"up"}'