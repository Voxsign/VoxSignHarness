个性化后台实现 —— 运行说明

环境要求
Go 1.21+，无需外部数据库，数据全部以 JSONL 落盘。

编译
go build -o p13n .

启动
默认启动：./p13n
指定数据目录与端口：./p13n -data-dir ./data -addr 127.0.0.1:8080
参数说明：
-data-dir  数据目录，默认 ./data，首次启动自动创建
-addr      监听地址，默认 127.0.0.1:8080；仅允许回环地址，非 127.0.0.1 的地址会被拒绝启动
-token     鉴权占位令牌，默认空（空表示不校验，仅作占位接口）

端点
GET  /v1/health
  返回 {"status":"ok","time":"...","data_dir":"..."}，用于存活探测。

POST /v1/process
  请求：Content-Type: application/json
  {
    "text": "待处理文本",
    "action": "correct | intent | dict_add | dict_del | dict_get | feedback",
    "key": "词典条目（dict_* 时使用）",
    "value": "词典释义/别名词（可选）",
    "feedback": true/false,
    "trace_id": "可选，缺省自动生成"
  }
  响应：
  {
    "trace_id": "...",
    "intent": "NOTE | QUERY | EDIT | COMMIT | ORCHESTRATE",
    "corrected": "纠错后文本",
    "cleaned": "清洗后文本",
    "dict_hits": [...],
    "ok": true
  }
  说明：
  - action=correct 走清洗 + 词典纠错，命中词典才替换，未命中不改动原文，保证正常文本不被改坏；
  - action=intent 只做意图分类，输出五类之一；
  - action=dict_add/dict_del/dict_get 完成词典增、删、查；
  - action=feedback 携带 feedback=true/false，写入反馈文件。

鉴权
请求头可选 Authorization: Bearer <token>。token 为空时不校验；配置了 token 则必须匹配，否则返回 401。

数据文件（均在 -data-dir 目录下，JSONL 一律 append-only，只追加不改写）
traces.jsonl    每次 /v1/process 的输入输出轨迹
usage.jsonl     调用计数与耗时统计
feedback.jsonl  反馈学习记录（✔/✘ 回馈）
dictionary.json 个性化词典快照（增删查的唯一真源，重启后加载）

运维提示
查看健康：curl http://127.0.0.1:8080/v1/health
文本处理：curl -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -d '{"text":"帮我记一下明天开会","action":"correct"}'
日志与数据分离，删除 data-dir 即完成重置。