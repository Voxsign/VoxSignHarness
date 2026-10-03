个性化后台实现 — 运行说明

一、环境与启动
1. 安装依赖：pip install -r requirements.txt
2. 启动服务：python -m app.server --host 127.0.0.1 --port 8787 --data-dir ./data
3. 仅监听回环地址：默认绑定 127.0.0.1；传入非回环 IP 会被拒绝并退出。启动日志会打印实际监听地址与数据目录。
4. 鉴权为占位实现：请求头 Authorization: Bearer <token>，未配置 token 时跳过校验，配置后校验失败返回 401。

二、HTTP 端点
GET  /v1/health
  返回 {"status":"ok","version":"...","data_dir":"...","uptime_s":N}

POST /v1/process   （Content-Type: application/json）
  请求：{"text":"...", "user_id":"u1", "session_id":"s1", "feedback":null}
  feedback 可为 null 或 true/false，用于对上一次结果回馈（✔/✘）。
  响应：{"intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE",
         "corrected_text":"清洗+词典纠错后的文本",
         "changes":[{"from":"...","to":"...","reason":"dict|clean"}],
         "matched_terms":["..."],
         "trace_id":"..."}
  intent 五类：NOTE 记录、QUERY 查询、EDIT 修改、COMMIT 提交、ORCHESTRATE 编排。
  纠错安全：仅替换词典命中项与明显噪声字符，未命中不动原文，正常文本保持原样返回（changes 为空）。

三、数据文件（data-dir 下，均 append-only JSONL）
dictionary.jsonl   个性化词典条目，含增/删/查，删除以 tombstone 追加记录实现
feedback.jsonl     反馈回执，每行 {"trace_id","verdict":"up|down","ts"}
traces.jsonl       每次 /v1/process 的输入、意图、纠错结果与耗时
usage.jsonl        调用计量，每行含端点、user_id、状态码、耗时
data-dir 通过 --data-dir 或环境变量 APP_DATA_DIR 指定；目录不存在时自动创建。

四、最小验证
curl -s http://127.0.0.1:8787/v1/health
curl -s -X POST http://127.0.0.1:8787/v1/process -H 'Content-Type: application/json' -d '{"text":"帮我记一下明天开会","user_id":"u1"}'