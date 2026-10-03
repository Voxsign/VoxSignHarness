个性化后台 运行说明

一、启动
  go build -o bin/personalize .
  ./bin/personalize --data-dir ./data --addr 127.0.0.1:8080
  或：go run . --data-dir ./data --addr 127.0.0.1:8080

  参数/环境变量（命令行优先）：
  --data-dir / DATA_DIR   数据目录，默认 ./data，不存在则自动创建
  --addr     / ADDR       监听地址，默认 127.0.0.1:8080，非回环地址直接拒绝启动
  --token    / API_TOKEN  鉴权令牌占位；为空时不校验

二、HTTP 端点
  GET  /v1/health
       返回 {"status":"ok","data_dir":"...","uptime_s":N}
  POST /v1/process
       请求 {"text":"...","user_id":"u1","action":"note|query|edit|commit|orchestrate"}
       响应 {"intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE","corrected":"...",
             "dict_hits":[{"term":"...","op":"..."}],"trace_id":"..."}
  POST /v1/feedback
       请求 {"trace_id":"...","label":"ok|bad","note":"..."}，追加写入 feedback.jsonl
  GET  /v1/dictionary?q=关键词        查询词典条目
  POST /v1/dictionary                 新增/更新条目 {"term":"...","to":"..."}
  DELETE /v1/dictionary               删除条目 {"term":"..."}

  鉴权：请求头 Authorization: Bearer <token>；未配置 token 时仅记录不拦截。

三、数据文件（均在 data-dir 下，JSONL append-only，仅追加不覆写）
  dictionary.json   个性化词典条目（增删查的唯一真源）
  feedback.jsonl    反馈学习记录，每行一条 ✔/✘ 回馈
  traces.jsonl      每次 /v1/process 的输入、命中、意图、耗时
  usage.jsonl       端点调用计数与延迟

四、调用示例
  curl -s http://127.0.0.1:8080/v1/health
  curl -s -X POST http://127.0.0.1:8080/v1/process \
       -H 'Content-Type: application/json' \
       -d '{"text":"明天开会导致项目复盘","user_id":"u1"}'
  curl -s -X POST http://127.0.0.1:8080/v1/feedback \
       -H 'Content-Type: application/json' \
       -d '{"trace_id":"<上一步返回>","label":"ok"}'

五、实现要点（代码位置）
  main.go                启动、路由、127.0.0.1 校验、鉴权占位
  internal/dictionary    词典增删查（dictionary/dict）
  internal/correct       文本清洗 + 词典纠错（correct），命中才替换，正常文本原样返回
  internal/intent        五类意图分类（intent）
  internal/feedback      反馈学习落盘（feedback）
  internal/store         JSONL append-only 落盘（jsonl/append），带文件锁
  词典与纠错均做边界匹配，避免把正常词改坏；未命中即不修改。

六、退出
  Ctrl+C，等待落盘 flush 完成后进程结束。