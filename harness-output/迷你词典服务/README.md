迷你词典服务 README

一、环境与安装
Python 3.11+，建议虚拟环境。
  python -m venv .venv && source .venv/bin/activate
  pip install -r requirements.txt

二、启动
  python -m minidict --host 127.0.0.1 --port 8080 --data-dir ./data --auth-token dev-token
只监听 127.0.0.1，非回环地址（0.0.0.0、外网 IP 等）一律拒绝启动。鉴权为占位实现：请求头 Authorization: Bearer <auth-token>，未配置 token 时仅接受回环请求并打印告警。

三、HTTP 端点
GET  /v1/health
  返回 {"status":"ok","data_dir":"./data","version":"..."}，无鉴权要求，仅回环可访问。
POST /v1/process
  请求头：Authorization: Bearer dev-token；Content-Type: application/json
  请求体：
    {"text":"...", "intent":"auto", "session_id":"可选"}
  intent 为 auto 时自动分类为 NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE 五类之一。
  响应体：
    {"intent":"...","clean_text":"...","corrected_text":"...","entries":[...],"note":"...","trace_id":"..."}

四、请求示例（不加围栏，注意单引号转义）
  curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Authorization: Bearer dev-token' -H 'Content-Type: application/json' -d '{"text":"把 彼得周点com 加入词典","intent":"auto"}'

五、数据文件（全部 append-only JSONL，位于 --data-dir）
  dict.json          个性化词典主数据（增/删/查，删除写 tombstone，不物理丢历史）
  traces.jsonl       每次 /v1/process 的输入、意图、纠错前后的文本
  usage.jsonl        端点调用与延迟等用量记录
  feedback.jsonl     反馈学习记录，仅追加，不覆盖

反馈写入格式（feedback.jsonl）：
  {"ts":"...","trace_id":"...","verdict":"ok|bad","target":"...","before":"...","after":"..."}
  verdict 为 ok（✔ 确认）表示该纠正/条目正确并进入词典；bad（✘ 标错）表示该映射被否定并加入黑名单。

六、词典与纠错约定（已沉淀的已知偏好）
  1) 拼写映射：彼得周点com → model.peterzou.com（已 ✔ 确认，直接命中纠错）
  2) 教词：Mansour 为正确词条，加入个性化词典
  3) 标错：曼苏 → Mansour 的旧写法已 ✘ 标错，曼苏 进入黑名单，不作正确词条、不再作为输出
  4) 清洗阶段只做安全规整（空白、全半角、标点、大小写规整），正常文本必须原样保留；任何改动都要记录在 traces.jsonl 中可回溯
  5) 未命中词典的文本一律不改写，避免把正常文本改坏

七、最小自检
  curl -s http://127.0.0.1:8080/v1/health
  curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Authorization: Bearer dev-token' -H 'Content-Type: application/json' -d '{"text":"彼得周点com 怎么拼","intent":"auto"}'
  预期：intent 为 QUERY，corrected_text 含 model.peterzou.com，且 traces.jsonl 新增一行。