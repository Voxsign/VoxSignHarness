个性化后台实现

启动
- 编译：go build -o personal-backend .
- 运行：./personal-backend --addr 127.0.0.1:8080 --data-dir ./data
- 开发：go run . --addr 127.0.0.1:8080 --data-dir ./data
- 环境变量：ADDR、DATA_DIR、AUTH_TOKEN 可替代对应参数；AUTH_TOKEN 为占位鉴权，未设置时本地不校验。
- 网络限制：默认只监听 127.0.0.1；非回环地址会拒绝启动。

端点
- GET /v1/health：健康检查。示例：curl http://127.0.0.1:8080/v1/health。响应：{"status":"ok"}。
- POST /v1/process：清洗、词典纠错、意图分类。请求头：Content-Type: application/json；如设 AUTH_TOKEN，加 Authorization: Bearer <token>。
  请求体示例：{"text":"帮我记一下明天买牛奶"}
  响应体示例：{"intent":"NOTE","corrected":"帮我记一下明天买牛奶","changes":[],"trace_id":"..."}
  意图取值：NOTE、QUERY、EDIT、COMMIT、ORCHESTRATE。
  反馈示例：{"text":"...","feedback":"✔"} 或 {"feedback":"✘"}，写入 feedback.jsonl。

数据文件
- 默认数据目录：./data，可用 --data-dir 或 DATA_DIR 修改；不存在时自动创建。
- data/traces.jsonl：请求/响应/中间轨迹，append-only。
- data/usage.jsonl：调用与耗时统计，append-only。
- data/feedback.jsonl：✔/✘ 反馈，append-only。
- data/dictionary.jsonl：个性化词典增删事件，append-only；启动时重放。