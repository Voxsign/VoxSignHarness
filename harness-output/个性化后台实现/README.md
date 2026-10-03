个性化后台 运行说明（README）

一、环境与构建
要求：Go 1.21+
构建：
  go build -o p13n ./cmd/p13n
  （若为单文件入口，则 go build -o p13n .）
运行：
  go run . -addr 127.0.0.1:8787 -data-dir ./data -token dev-token

启动参数
  -addr       监听地址，默认 127.0.0.1:8787；非回环地址（如 0.0.0.0、局域网 IP）启动即拒绝并退出非零
  -data-dir   数据目录，默认 ./data，首次启动自动创建
  -token      鉴权占位令牌，默认 dev-token；请求头 X-Auth-Token 必须匹配，否则 401
  说明：鉴权为占位实现（静态令牌），仅用于占位与联调，不构成生产级认证。

二、HTTP 端点（均为 JSON 请求/响应）

1) 健康检查
  GET /v1/health
  200 {"status":"ok","version":"...","data_dir":"./data","uptime_s":12}

2) 主处理端点
  POST /v1/process
  请求 {"text":"明天三点开会 记一下","user_id":"u1","session_id":"s1"}
  响应 {"intent":"NOTE","corrected":"明天三点开会 记一下","changed":false,
        "matches":[{"term":"开会","kind":"dict"}],"trace_id":"...","usage":{"tokens_in":9,"tokens_out":9}}
  处理链路：清洗 -> 词典纠错（命中即替换，未命中保守保留原文）-> 意图分类（NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE）
  安全约束：正常文本不被改坏；纠错仅在词典命中且满足匹配规则时生效，否则原样返回且 changed=false。

3) 个性化词典
  POST   /v1/dict      增：{"term":"开会","alias":["开个会"],"note":"..."} -> 201
  GET    /v1/dict      查：可选 ?q=开会；不传则列表
  DELETE /v1/dict      删：{"term":"开会"} -> 200 {"deleted":1}

4) 反馈学习
  POST /v1/feedback
  请求 {"trace_id":"...","label":"ok"}   // label: ok = ✔ / bad = ✘
  200 {"recorded":true}

三、数据文件（独立数据目录，全部 append-only JSONL，仅追加、不重写）
  <data-dir>/traces.jsonl    每次 /v1/process 一条：trace_id、时间、intent、原文、纠错后文本、changed、匹配项
  <data-dir>/usage.jsonl     每次请求一条：端点、耗时、tokens、状态码
  <data-dir>/feedback.jsonl  每条反馈一条：trace_id、label(ok/bad)、时间
  <data-dir>/dictionary.json 词典持久化快照（增删后原子重写，非 JSONL）

追加语义：以 O_APPEND 打开并写入完整单行 JSON，进程崩溃不产生半行破坏；文件按需创建，权限 0600。
查看示例：
  tail -n 5 ./data/feedback.jsonl
  grep '"intent":"COMMIT"' ./data/traces.jsonl

四、快速自检
  curl -s http://127.0.0.1:8787/v1/health -H 'X-Auth-Token: dev-token'
  curl -s -X POST http://127.0.0.1:8787/v1/dict -H 'X-Auth-Token: dev-token' \
       -d '{"term":"开会","alias":["开个会"]}'
  curl -s -X POST http://127.0.0.1:8787/v1/process -H 'X-Auth-Token: dev-token' \
       -d '{"text":"开个会吧","user_id":"u1"}'
  curl -s -X POST http://127.0.0.1:8787/v1/feedback -H 'X-Auth-Token: dev-token' \
       -d '{"trace_id":"<上一步返回>","label":"ok"}'

五、验收对应
  词典增/删/查：/v1/dict 三个方法 + dictionary.json
  纠错：process 链路中的 clean + dict correct，保守不破坏正常文本
  意图分类：五类 NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE
  反馈落盘：feedback.jsonl append-only
  数据落盘：traces.jsonl、usage.jsonl，data-dir 可配
  HTTP：/v1/health、/v1/process
  监听安全：仅 127.0.0.1 回环，非回环拒绝；X-Auth-Token 占位鉴权
  编译：go build 通过