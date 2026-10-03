个性化后台 运行说明

一、环境与启动
1. 依赖：Python 3.10+，仅标准库即可运行；如需 HTTP 层可装 fastapi/uvicorn。
2. 默认启动（监听回环 127.0.0.1，默认端口 8080）：
   python -m app.main --host 127.0.0.1 --port 8080 --data-dir ./data
3. 等价环境变量方式：
   APP_HOST=127.0.0.1 APP_PORT=8080 APP_DATA_DIR=./data python -m app.main
4. 鉴权占位：请求头 Authorization: Bearer <token>，token 由 --api-token 或 APP_API_TOKEN 指定；未配置时跳过校验但仍保留鉴权中间件。
5. 非回环地址（如 0.0.0.0、局域网 IP）一律拒绝启动并报错退出，如需外部访问请自行加反向代理，本服务不放开监听。

二、HTTP 端点
GET  /v1/health
    返回 {"status":"ok","host":"127.0.0.1","data_dir":"...","version":"..."}
POST /v1/process
    请求 JSON：
    {
      "text": "原始文本",
      "intent_hint": null,
      "user_id": "u1",
      "feedback": null        // 可选，取值 "up" 或 "down"，即 ✔/✘
    }
    响应 JSON：
    {
      "intent": "NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE",
      "cleaned_text": "清洗后文本",
      "corrected_text": "词典纠错后文本",
      "matches": [{"term":"...","action":"keep|replace|reject","score":0.0}],
      "feedback_id": "trace 关联 id",
      "trace_id": "..."
    }
    说明：五类意图固定为 NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE；无法判定时回落到 NOTE 并置 confidence 低位。
    纠错安全：仅命中个性化词典且未命中保护词（数字、URL、代码块、专有名词表）时才替换，其余原样返回，保证正常文本不被改坏。

词典管理（同一次启动提供，可选调用）：
POST /v1/dict/add      {"term":"...","replacement":"...","enabled":true}
POST /v1/dict/remove   {"term":"..."}
GET  /v1/dict/lookup?term=...

三、数据文件（全部 append-only，位于 data-dir 下）
data-dir/
  dict.json         个性化词典条目（可整体重写，服务启动时加载）
  feedback.jsonl    反馈学习记录，每行一条：{"ts","feedback_id","trace_id","text","intent","vote":"up|down"}
  traces.jsonl      处理链路：{"ts","trace_id","user_id","intent","input","cleaned","corrected","matches"}
  usage.jsonl       调用计量：{"ts","endpoint","status","latency_ms","user_id"}
  corrections.jsonl 纠错明细（可选，便于回溯误改）

约定：
1. JSONL 只追加不修改不删除，按天滚动由外部脚本负责；每行必须为合法 JSON，写入失败不影响主流程。
2. feedback.jsonl 用于后续统计对/错，vote=down 的条目可作为词典修正或规则调整依据。
3. data-dir 通过 --data-dir 或 APP_DATA_DIR 指定，不存在时自动创建；建议与代码目录分离，便于备份与清理。

四、常见操作
1. 健康检查：curl http://127.0.0.1:8080/v1/health
2. 处理一条文本：
   curl -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -d '{"text":"记一下明天开会","user_id":"u1"}'
3. 反馈：
   curl -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -d '{"text":"记一下明天开会","user_id":"u1","feedback":"up"}'
4. 查看反馈落盘：tail -f data/feedback.jsonl

五、注意事项
1. 服务仅绑定 127.0.0.1，容器或本机外部不可直连，请在调用侧或代理层解决暴露问题。
2. 鉴权为占位实现，生产使用请替换为真实 token 校验与限流。
3. 数据目录权限设为仅当前用户可读写，feedback 与 traces 含原始文本，注意脱敏与留存策略。