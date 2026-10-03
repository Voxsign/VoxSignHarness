个性化后台实现 — 运行说明

环境
Go 1.21+，单机运行，无外部依赖（数据仅落本地文件）。

构建与启动
go build -o p13n .
./p13n -addr 127.0.0.1:8080 -data-dir ./data -token dev-token
调试时可直接：go run main.go -addr 127.0.0.1:8080 -data-dir ./data

启动参数
-addr      监听地址，默认 127.0.0.1:8080；仅接受回环地址（127.0.0.1 / ::1），传入非回环地址直接报错退出。
-data-dir  数据目录，默认 ./data，不存在会自动创建。
-token     鉴权占位令牌，默认 dev-token；请求需带 Authorization: Bearer <token>，不匹配返回 401。

HTTP 端点
GET  /v1/health   健康检查，返回 {"status":"ok","data_dir":"...","time":"..."}
POST /v1/process  业务入口，JSON 请求 / JSON 响应，Content-Type: application/json

/v1/process 请求示例
{"action":"process","text":"帮我记一下明天开会","feedback":"ok","meta":{}}
action 取值：
  process   清洗 + 词典纠错 + 意图分类（默认）
  dict_add / dict_del / dict_list   个性化词典增删查
  feedback  反馈回写（等价于请求体带 feedback 字段）
返回示例
{"ok":true,"intent":"NOTE","corrected":"帮我记一下明天开会","hits":[],"trace_id":"..."}
intent 取值：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE；无法判定时回退 NOTE 并在 ok 之外给出 warn 说明。
纠错仅在词典命中且置信度达标时替换，正常文本原样返回。

数据文件（均在 data-dir 下，全部 append-only JSONL，只追加不覆写）
dict.jsonl        个性化词典条目，增/删/查均以追加事件记录，查询时按时间重放取最新状态。
traces.jsonl      每次 /v1/process 的输入、纠错结果、意图、耗时。
usage.jsonl       调用计数与用量统计。
feedback.jsonl    ✔/✘ 反馈，仅追加，用于后续学习。
如需重置数据，直接停服后归档或删除整个 data-dir，勿原地改写文件。

安全
默认只监听 127.0.0.1，非回环地址拒绝启动；鉴权为占位实现，生产接入前请替换为真实令牌校验。