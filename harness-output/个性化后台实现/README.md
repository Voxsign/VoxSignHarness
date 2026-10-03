个性化后台实现 — 运行说明

一、构建与启动

编译：go build -o pbackend .
启动：./pbackend -addr 127.0.0.1:8080 -data-dir ./data
参数说明：
-addr 监听地址，仅允许回环地址（127.0.0.1 或 ::1），非回环直接拒绝启动，默认 127.0.0.1:8080
-data-dir 数据目录，可配置，程序启动时自动创建，默认 ./data
-token 鉴权占位令牌，可留空；留空时所有请求放行，非空时校验 Authorization: Bearer <token>
端口占用或数据目录不可写会直接退出并打印原因。

二、HTTP 端点

1. GET /v1/health
   返回服务存活状态、版本、数据目录路径。
   响应示例：{"status":"ok","version":"0.1.0","data_dir":"./data"}

2. POST /v1/process
   请求体 JSON，字段：
   - text 必填，待处理文本
   - action 可选，字典操作：add / delete / query，缺省走纠错与意图流程
   - term 可选，词典条目（action 为 add/delete/query 时使用）
   - feedback 可选，取值为 up 或 down，用于反馈学习
   响应体 JSON，字段：
   - corrected 纠错后的文本（正常文本原样返回，不被改坏）
   - intent 意图分类结果，取值 NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
   - dict 命中的词典条目（数组，可为空）
   - ok 布尔，是否成功
   - trace_id 本次调用链路 ID，可用于查日志

三、数据文件

全部位于 -data-dir 指定目录，均为 JSONL 格式，append-only 追加写，不覆写不改写：
- data/dictionary.jsonl 个性化词典条目，含增删记录，加载时按末条状态回放
- data/feedback.jsonl 反馈学习记录，每条含 trace_id、文本、corrected、intent、✔/✘ 标记
- data/traces.jsonl 每次 /v1/process 请求的输入、输出、耗时、trace_id
- data/usage.jsonl 调用计数与端点用量统计

写入策略：每行一条完整 JSON，写完即 flush；文件不存在时自动创建；磁盘写失败不影响接口返回，仅在响应中省略 ok 提示并记录到 stderr。

四、自检

启动后执行：curl -s http://127.0.0.1:8080/v1/health 应返回 status 为 ok；
再执行一条 /v1/process 请求，确认 data/traces.jsonl 新增一行。