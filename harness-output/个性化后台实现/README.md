个性化后台实现 README

一、环境与构建
依赖 Go 1.21+（仅标准库）。在项目根目录执行：
go build -o pserver .
构建产物为单文件可执行程序 pserver；若构建失败，请先修复编译错误再启动。

二、启动命令
./pserver --addr 127.0.0.1:8080 --data-dir ./data --token dev-token
参数说明：
--addr 监听地址，必须为回环地址（127.0.0.1 或 ::1）。传入 0.0.0.0、局域网 IP 等非回环地址时进程直接拒绝启动并退出。
--data-dir 独立数据目录，默认 ./data，不存在时自动创建。
--token 鉴权令牌占位实现；未配置时仅回环内可用，仍要求携带请求头。

三、HTTP 端点
1) GET /v1/health
   健康检查，无需鉴权。返回 {"status":"ok","time":...,"data_dir":...}。

2) POST /v1/process
   业务主入口，需鉴权。请求头：Authorization: Bearer <token>；Content-Type: application/json。
   请求体：{"text":"帮我记一下 明天开会","trace_id":"可选","feedback":"可选, ok|ng"}
   响应体：{"intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE","corrected":"清洗纠错后文本","changes":[...],"matches":[...],"trace_id":"..."}
   处理流程：文本清洗 → 个性化词典匹配与纠错（正常文本保持原样，不改坏）→ 意图五分类 → 落盘 traces/usage → 可选反馈落盘。

3) 词典管理（同一鉴权要求）
   GET /v1/dict/list 查全部条目
   POST /v1/dict/add 请求 {"term":"...","alias":"..."} 增条目
   POST /v1/dict/delete 请求 {"term":"..."} 删条目

四、数据文件（全部 append-only，位于 --data-dir 目录）
data/traces.jsonl   每次 /v1/process 的请求、意图、纠错结果、耗时
data/usage.jsonl    调用量与端点维度的使用记录
data/feedback.jsonl 反馈学习记录，来源为请求中的 feedback 字段或 /v1/process 的 ✔/✘ 回馈
data/dictionary.json 个性化词典条目的增删查持久化快照
JSONL 均为逐行追加写，不重写、不截断；文件不存在时自动创建。

五、快速自检
curl -s http://127.0.0.1:8080/v1/health
curl -s -X POST http://127.0.0.1:8080/v1/process -H "Authorization: Bearer dev-token" -H "Content-Type: application/json" -d "{\"text\":\"帮我记一下明天开会\"}"
返回 intent 为 NOTE，且 data/traces.jsonl 新增一行，即视为 P0 通路正常。