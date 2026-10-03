个性化后台实现 · 运行说明

一、启动

  go build -o backend .
  ./backend -addr 127.0.0.1:8080 -data-dir ./data -token dev-token

  -addr     监听地址，仅允许 127.0.0.1 / ::1 / localhost，非回环地址直接拒绝启动
  -data-dir 数据目录，默认 ./data，不存在时自动创建
  -token    鉴权占位令牌，默认 dev-token；请求需带 Authorization: Bearer <token>

  开发调试：go run . -data-dir ./data

二、HTTP 端点

  GET /v1/health
    返回 {"status":"ok","version":"...","data_dir":"..."}，无需鉴权。

  POST /v1/process
    Content-Type: application/json，需鉴权，请求/响应均为 JSON。

    1) 文本处理（清洗 + 词典纠错 + 意图分类）
       请求 {"text":"明天下午三点开个会"}
       响应 {"corrected":"明天下午三点开个会","intent":"NOTE","hits":[],"trace_id":"..."}
       intent 取值：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
       正常文本原样返回，不误改。

    2) 词典增删查
       增  {"op":"dict_add","term":"阿理","replacement":"阿里"}
       删  {"op":"dict_del","term":"阿理"}
       查  {"op":"dict_get","term":"阿理"}
       列表 {"op":"dict_list"}
       匹配与纠错带安全校验：长度、空白、自替换、命中重叠均跳过。

    3) 反馈学习
       请求 {"op":"feedback","text":"明天开会","corrected":"明天下午开会","intent":"NOTE","verdict":"up"}
       verdict 为 up / down，仅追加写入，不修改历史记录。

三、数据文件（均在 -data-dir 下，JSONL 一律 append-only）

  dictionary.json   个性化词典条目（增删查的唯一权威源）
  feedback.jsonl    反馈学习记录，每行一条
  traces.jsonl      每次 /v1/process 的请求、结果、耗时
  usage.jsonl       按端点与意图的调用计数与用量

  行格式示例
    {"ts":"2026-01-01T10:00:00Z","trace_id":"...","text":"...","corrected":"...","intent":"NOTE","ms":3}
    {"ts":"2026-01-01T10:00:01Z","verdict":"up","term":"阿理","replacement":"阿里"}

四、curl 自检

  curl -s http://127.0.0.1:8080/v1/health
  curl -s -H 'Authorization: Bearer dev-token' -H 'Content-Type: application/json' \
    -d '{"text":"明天下午三点开个会"}' http://127.0.0.1:8080/v1/process
  curl -s -H 'Authorization: Bearer dev-token' -H 'Content-Type: application/json' \
    -d '{"op":"dict_add","term":"阿理","replacement":"阿里"}' http://127.0.0.1:8080/v1/process
  tail -n 5 data/traces.jsonl