README.md

个性化后台实现

一、启动
  go build -o pbackend . && ./pbackend --addr 127.0.0.1:8080 --data-dir ./data --token dev-token
  或：go run . --addr 127.0.0.1:8080 --data-dir ./data --token dev-token
  服务只监听回环地址；绑定非 127.0.0.1 的地址会在启动时直接拒绝并退出。
  鉴权为占位实现：请求需带 Authorization: Bearer <token>，不匹配返回 401。

二、端到端自检
  curl -s http://127.0.0.1:8080/v1/health
  curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Authorization: Bearer dev-token' -H 'Content-Type: application/json' -d '{"text":"帮我记一下明天开会"}'

三、HTTP 端点
  GET  /v1/health    健康检查，返回 status、data_dir、版本等
  POST /v1/process   业务主入口，JSON 进 JSON 出

  /v1/process 请求字段
    text      必填，待处理原始文本
    id        可选，本次请求标识，用于反馈回填
    feedback  可选，取值 ok / bad（✔ / ✘），提供时仅记录反馈并落盘

  /v1/process 响应字段
    id        请求标识
    cleaned   清洗后文本
    corrected 词典纠错后文本（正常文本原样返回，不误改）
    intent    NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE 五类之一
    matched   命中的词典条目
    message   说明信息

四、数据文件（均为 append-only JSONL，重启不覆盖）
  <data-dir>/traces.jsonl    每次 /v1/process 的处理轨迹
  <data-dir>/usage.jsonl     调用用量记录
  <data-dir>/feedback.jsonl  反馈学习记录（✔/✘）
  <data-dir>/dictionary.json 个性化词典条目（增删查，写入后即生效）

  说明：data-dir 由 --data-dir 指定，缺省 ./data，目录不存在时自动创建。
  JSONL 一律追加写入，不重写、不截断。

五、实现要点
  词典：支持条目增、删、查，匹配时按长度优先，纠错走安全替换——仅当命中
        词典且替换后不破坏句式时才改写，正常文本保持原样。
  纠错：先做空白与不可见字符清洗，再做词典纠错。
  意图：按关键词与结构规则归类为 NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE，
        无命中时归入 NOTE。
  反馈：/v1/process 携带 feedback 时追加写入 feedback.jsonl，仅追加不修改历史。