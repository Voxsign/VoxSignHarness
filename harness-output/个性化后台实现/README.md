个性化后台实现

启动
  go build -o personal-backend .
  ./personal-backend --addr 127.0.0.1:8080 --data-dir ./data --token dev-token

开发运行
  go run . --addr 127.0.0.1:8080 --data-dir ./data --token dev-token

监听与鉴权
  仅监听 127.0.0.1；非回环地址启动会拒绝。
  鉴权为占位实现，token 由 --token 指定。
  请求头：Authorization: Bearer dev-token 或 X-Token: dev-token。

HTTP 端点
  GET /v1/health
    健康检查，返回 {"ok":true}。

  POST /v1/process
    请求与响应均为 JSON。
    文本处理：清洗 + 词典纠错 + 意图分类。
    意图分类：NOTE、QUERY、EDIT、COMMIT、ORCHESTRATE。
    个性化词典：通过 action=dict_add / dict_del / dict_get 增、删、查。
    反馈学习：通过 action=feedback，feedback=true/false 回馈，追加写入 feedback.jsonl。

数据文件
  数据目录由 --data-dir 指定，默认 ./data。
  dictionary.json   个性化词典，支持增/删/查。
  feedback.jsonl    反馈学习记录，append-only。
  traces.jsonl      处理轨迹，append-only。
  usage.jsonl       调用用量，append-only。