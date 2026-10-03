个性化后台实现 · 运行说明

一、环境与构建
需要 Go 1.21 及以上版本。在项目根目录执行：
go build -o personald .
若编译报错，先执行 go mod tidy 拉齐依赖，再重新构建。

二、启动
默认启动（数据目录 ./data，监听 127.0.0.1:8080）：
./personald
指定数据目录与端口：
./personald -data-dir ./data -addr 127.0.0.1:8080
鉴权占位：请求头 X-Auth-Token 需与启动参数 -token 一致（默认 dev-token）。
注意：服务只监听 127.0.0.1，非回环地址（如 0.0.0.0）会被拒绝启动。

三、HTTP 端点
GET  /v1/health
返回 {"status":"ok","data_dir":"...","version":"..."}，用于存活与配置自检。

POST /v1/process
请求头：Content-Type: application/json；X-Auth-Token: <token>
请求体字段：
  text        待处理文本（必填）
  action      note | query | edit | commit | orchestrate，缺省则自动分类
  feedback    optional，"" | "up" | "down"，对应 ✔/✘ 回馈
响应体字段：
  intent      五类之一：NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE
  corrected   纠错后的文本（正常文本原样返回）
  matches     命中的个性化词典条目数组
  trace_id    本次请求链路 ID，与落盘记录一一对应

示例：
curl -s -X POST http://127.0.0.1:8080/v1/process \
  -H "Content-Type: application/json" -H "X-Auth-Token: dev-token" \
  -d '{"text":"帮我把周报记一下","action":"note"}'

四、个性化词典维护
词典通过数据目录下的 dictionary.json 维护（增/删/查），也可用子命令：
./personald dict add "周报" "weekly report"
./personald dict del "周报"
./personald dict list
匹配采用最长优先 + 大小写不敏感，纠错仅在置信度高于阈值时替换，保证正常文本不被改坏。

五、数据落盘（append-only JSONL，目录由 -data-dir 决定）
data/dictionary.json   个性化词典（增删查的唯一真源）
data/feedback.jsonl    反馈学习记录，仅追加，字段：ts, trace_id, intent, corrected, feedback
data/traces.jsonl      请求链路，仅追加，字段：ts, trace_id, intent, matches, latency_ms
data/usage.jsonl       用量统计，仅追加，字段：ts, trace_id, intent, text_len
所有 JSONL 文件只追加不覆盖，按行独立可解析；删除文件即视为清空对应数据。

六、退出与检查
Ctrl+C 优雅退出，落盘写入已 flush。启动后在另一终端执行：
curl -s http://127.0.0.1:8080/v1/health
返回 ok 即表示服务可用；若返回空，检查 -addr 是否为回环地址、-data-dir 是否可写。