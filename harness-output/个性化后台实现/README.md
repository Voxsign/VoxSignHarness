个性化后台实现 — 运行说明

一、启动
构建：go build -o pbackend .
启动：./pbackend --data-dir ./data --addr 127.0.0.1:8080
调试：go run . --data-dir ./data

参数说明
--data-dir  数据目录，默认 ./data，不存在则自动创建（可配）
--addr      监听地址，默认 127.0.0.1:8080；非回环地址（如 0.0.0.0）直接拒绝启动
--auth-token 可选，鉴权占位串；配置后请求需带 Authorization 头

服务只监听 127.0.0.1，无外网暴露；鉴权为占位实现，但端点校验必须存在。

二、HTTP 端点
GET  /v1/health
     返回 {"status":"ok","version":"...","data_dir":"...","time":"..."}，用于存活探针。

POST /v1/process
     请求 JSON：{"text":"原始文本","feedback":"ok|bad（可选）","top_k":3（可选）}
     处理链：清洗 → 词典纠错（安全匹配，正常文本不被改坏）→ 意图分类
     意图取值：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
     响应 JSON：{"intent":"...","corrected":"...","changed":true|false,"hits":[...],"trace_id":"..."}
     单次请求同时触发 traces.jsonl 与 usage.jsonl 追加写入。

三、数据文件（均在 --data-dir 下，全部 append-only）
dictionary.json   个性化词典条目：增/删/查，进程内加载 + 写回落盘
feedback.jsonl    反馈学习流：每行一条 ✔/✘ 回馈，只追加、不覆盖、不截断
traces.jsonl      每次 /v1/process 的处理轨迹，按 trace_id 串联
usage.jsonl       调用计量（时间、意图、耗时、结果码）

JSONL 写入采用 O_APPEND 单行完整写，重启后从尾部续写，历史记录不丢。

四、自检要点
1. 启动成功后 curl 127.0.0.1:8080/v1/health 返回 200。
2. 用 0.0.0.0 或非回环地址启动应报错退出。
3. 连续调用 /v1/process，确认 traces.jsonl 与 usage.jsonl 行数单调递增。
4. 词典增删查后重启服务，条目保持一致。
5. 正常文本经纠错后 corrected 与原文一致（changed=false），不被改坏。