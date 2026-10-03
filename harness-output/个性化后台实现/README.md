README（个性化后台）

一、构建与启动

  构建：go build -o p13n ./cmd/server
  或直接运行：go run ./cmd/server

  默认监听 127.0.0.1:8080（仅回环地址，来自非回环的请求一律拒绝）。
  数据目录默认 ./data，可通过参数或环境变量指定：
  go run ./cmd/server -addr 127.0.0.1:8080 -data-dir ./data
  P13N_ADDR=127.0.0.1:8080 P13N_DATA_DIR=./data go run ./cmd/server

  鉴权为占位实现：请求头 X-API-Key 可选校验，留空表示不校验（占位，可后续替换为真实鉴权）。

二、HTTP 端点

  GET  /v1/health
       返回服务状态、数据目录、词典条目数，例如 {"status":"ok","data_dir":"./data","dict_size":12}

  POST /v1/process
       请求体 JSON：{"text":"把明天的会议记一下","user_id":"u1","feedback":null}
       响应体 JSON：
       {
         "text_clean": "清洗后的文本",
         "text_corrected": "纠错后的文本",
         "corrected": true/false,
         "intent": "NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE",
         "confidence": 0.0-1.0,
         "trace_id": "...",
         "items": [...]
       }
       feedback 字段为可选：传 true/false 表示对该次结果打 ✔/✘，会追加写入 feedback.jsonl。

  词典操作（P0：增/删/查）
  POST /v1/dict/add     {"term":"术语","canonical":"规范写法","aliases":["别名"]}
  POST /v1/dict/delete  {"term":"术语"}
  GET  /v1/dict/list    列出全部条目
  GET  /v1/dict/get?term=术语   查询单条

三、数据文件（全部 append-only JSONL，位于 data-dir 内）

  data/dictionary.json   个性化词典（增删改时整体重写，读取时加载到内存）
  data/feedback.jsonl    反馈学习记录，每行一条 {"ts":...,"trace_id":...,"ok":true}
  data/traces.jsonl      每次 /v1/process 的处理轨迹，每行一条
  data/usage.jsonl       接口调用计数与耗时，每行一条

  目录与文件首次启动自动创建，不会覆盖已有内容；JSONL 仅追加，不修改历史行。

四、能力说明（P0 对应）

  ① 词典：增/删/查，匹配时按最长优先，纠错仅在命中词典或安全规则时替换，避免改坏正常文本。
  ② 纠错：先清洗（去多余空白、全角半角归一、URL/邮箱保护），再做词典纠错；未命中则原样返回。
  ③ 意图分类：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE 五类，规则优先 + 关键词加权。
  ④ 反馈学习：✔/✘ 追加落盘 feedback.jsonl，用于后续调整词典权重。
  ⑤ 落盘：traces/usage/feedback 均为 JSONL append-only。
  ⑥ 端点：/v1/health、/v1/process（JSON 请求/响应）。
  ⑦ 仅监听 127.0.0.1，鉴权占位但已存在。

五、自检

  go build ./...  应无错误；启动后 curl http://127.0.0.1:8080/v1/health 应返回 status=ok。