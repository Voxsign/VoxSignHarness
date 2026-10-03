README.md

个性化后台实现 — 运行说明

一、构建与启动
1. 构建：go build -o personald .
2. 启动：./personald --addr 127.0.0.1:8080 --data-dir ./data
3. 参数说明：
   --addr     监听地址，仅允许回环地址（127.0.0.1 / ::1）。填入非回环地址将直接拒绝启动并退出。
   --data-dir 数据目录，默认 ./data，不存在时自动创建。
4. 鉴权为占位实现：请求头携带 Authorization 即可，未配置时不校验；预留开关 --auth-token，设置后按该值比对。

二、HTTP 端点
1. GET /v1/health
   返回 {"status":"ok","version":"...","data_dir":"..."}，可用于探活。

2. POST /v1/process
   请求体 JSON：
   {"text":"待处理文本","action":"correct|intent|both","feedback":"ok|bad","session_id":"可选"}
   响应体 JSON：
   {"cleaned":"清洗后文本","corrected":"纠错后文本","intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE","changed":true,"trace_id":"..."}
   说明：action 省略时执行清洗 + 纠错 + 意图分类；feedback 字段用于回写反馈。

三、数据文件（均为 append-only，除词典外只追加不重写）
1. data/dict.json        个性化词典条目，支持增/删/查；重启后加载。
2. data/feedback.jsonl   反馈学习记录，每行一条 {"ts","session_id","text","intent","feedback"}。
3. data/traces.jsonl     每次 /v1/process 的调用轨迹。
4. data/usage.jsonl      用量统计（调用次数、耗时、命中词典条数）。

四、行为约束
1. 文本纠错只做安全替换：命中的词典条目才改写，未命中一律保留原文，不破坏正常文本。
2. 词典匹配需支持精确与模糊两种方式，模糊匹配带相似度阈值，避免误纠。
3. 进程只监听 127.0.0.1，外部网络不可直接访问。