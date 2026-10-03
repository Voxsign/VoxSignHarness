个性化后台实现 运行说明

一、环境与构建
依赖：Go 1.21+（模块模式），无外部数据库依赖。
构建：go build -o p13n ./cmd/p13n
自检：go vet ./... 与 go test ./... 应全部通过，源码中不得残留 todo 占位。

二、启动
默认启动（仅监听回环地址 127.0.0.1:8080）：
  ./p13n --addr 127.0.0.1:8080 --data-dir ./data

参数说明：
  --addr      监听地址，强制回环；传入 0.0.0.0、局域网 IP 或公网 IP 时启动即报错退出。
  --data-dir  数据目录，默认 ./data，首次启动自动创建。
  --token     可选，占位鉴权令牌；为空时开放本机访问，非空时请求需带 Authorization: Bearer <token>。

数据目录结构（全部 append-only，进程追加写，不做原地改写）：
  data/traces.jsonl     每次 /v1/process 的调用轨迹（入参、命中词典、纠错差异、意图、耗时）
  data/usage.jsonl      用量统计，按请求逐行追加
  data/feedback.jsonl   反馈学习落盘，✔/✘ 逐行追加
  data/dict.json        个性化词典快照（增删后原子重写该文件）

三、HTTP 端点
1) 健康检查
   GET /v1/health
   响应：{"status":"ok","uptime_ms":1234,"data_dir":"./data","dict_size":12}

2) 文本处理（核心业务）
   POST /v1/process
   请求头：Content-Type: application/json
   请求体：
   {
     "text": "帮我把明天十点會的纪要走查一遍",
     "actions": ["clean","correct","intent"],
     "session_id": "s-001"
   }
   字段说明：
     text       必填，待处理原文。
     actions    可选，默认全部；取值 clean/correct/intent，可任意组合。
     session_id 可选，用于轨迹串联。

   响应体：
   {
     "ok": true,
     "text_raw": "帮我把明天十点會的纪要走查一遍",
     "text_clean": "帮我把明天十点會的纪要走查一遍",
     "text_corrected": "帮我把明天十点会议的纪要查一遍",
     "corrections": [{"from":"會的","to":"会议","source":"dict","safe":true}],
     "intent": "QUERY",
     "intent_scores": {"NOTE":0.03,"QUERY":0.86,"EDIT":0.08,"COMMIT":0.02,"ORCHESTRATE":0.01},
     "trace_id": "t-7f3a91"
   }

3) 词典增删查
   POST /v1/dict        新增条目 {"term":"会议","aliases":["會的","回议"],"note":"常用词"}
   GET  /v1/dict?q=会   查询（按词面与前缀匹配，返回 term/aliases/命中次数）
   DELETE /v1/dict      删除条目 {"term":"会议"}
   约束：term 去空白后非空且不重复；同义词冲突时以先注册者为准并返回 409。

四、能力实现要点（对应 P0 清单）
① 个性化词典：内存索引 + dict.json 落盘，增/删/查三个动作幂等；同义词做冲突检测。
② 文本纠错：先清洗（归一化全半角、压缩多余空白、去零宽字符），再做词典纠错；
   纠错仅替换词典明确命中的同义词，禁止模糊替换整句；无可替换时原文原样返回，
   保证正常文本不被改坏（响应中 corrections 为空即未改动）。
③ 意图分类：NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE 五类，规则+词典加权打分，
   取最高分；低于阈值时回退 QUERY 并在 intent_scores 中体现。
④ 反馈学习：✔/✘ 通过 POST /v1/feedback 落盘
   {"trace_id":"t-7f3a91","verdict":"up","intent":"QUERY"}
   verdict 取 up/down，逐行追加写 feedback.jsonl，不在原文件上改动。
⑤ 数据落盘：traces/usage/feedback 三份 JSONL 均为 append-only，独立 --data-dir。
⑥ 端点：/v1/health、/v1/process 均已实现并有测试覆盖。
⑦ 监听：仅绑定 127.0.0.1（含 ::1），非回环地址一律拒绝启动；鉴权以 Bearer 令牌占位，接口预留不裸奔。

五、快速自测
  curl -s http://127.0.0.1:8080/v1/health
  curl -s -X POST http://127.0.0.1:8080/v1/process \
       -H 'Content-Type: application/json' \
       -d '{"text":"帮我把明天的纪要去查一遍"}'
  curl -s -X POST http://127.0.0.1:8080/v1/dict \
       -H 'Content-Type: application/json' \
       -d '{"term":"会议","aliases":["會的"]}'
  curl -s -X POST http://127.0.0.1:8080/v1/feedback \
       -H 'Content-Type: application/json' \
       -d '{"trace_id":"t-7f3a91","verdict":"up"}'

六、常见问题
端口占用：换 --addr 127.0.0.1:8090 重启。
启动即退出并提示非回环：说明 --addr 写了对外地址，属预期拒绝行为，改回 127.0.0.1 即可。
纠错未生效：确认词条已通过 /v1/dict 注册，且待纠错片段与别名完全一致。
数据损坏排查：JSONL 每行一条记录，可用 tail -n 20 data/traces.jsonl 逐行核对。