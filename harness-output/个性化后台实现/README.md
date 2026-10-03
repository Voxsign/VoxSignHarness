个性化后台实现 —— 运行说明

一、环境要求
Go 1.21+，无需外部依赖，本地运行。

二、启动命令
1) 编译
go build -o personald .

2) 运行（默认数据目录 ./data，监听 127.0.0.1:8080）
./personald

3) 自定义端口与数据目录
./personald -addr 127.0.0.1:9090 -data-dir /var/lib/personald

4) 开发态直接跑
go run . -addr 127.0.0.1:8080 -data-dir ./data

说明：仅监听 127.0.0.1 / ::1，非回环地址会拒绝启动；请求需带 Authorization: Bearer <token>（占位鉴权，默认 token 为 dev-token，可用 -token 覆盖）。

三、HTTP 端点
GET  /v1/health
  健康检查，返回 {"status":"ok","time":...}，无需鉴权。

POST /v1/process
  Content-Type: application/json，需鉴权。
  请求字段：
    text      必填，待处理文本
    intent    可选，显式指定 NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE，缺省由分类器判定
    feedback  可选，✔/✘ 反馈，取值 "up" / "down"
    session_id 可选
  响应字段：
    intent      最终意图
    corrected   纠错后文本
    changed     是否发生改动（正常文本应为 false）
    dict_hits   命中的词典条目
    feedback_id 落盘后的反馈记录 id
    trace_id    本次请求追踪 id

四、数据文件（全部 append-only JSONL，位于 -data-dir 下）
traces.jsonl    每次 /v1/process 的请求、纠错、意图与耗时
usage.jsonl     调用计数与维度（intent / 状态码 / 时间）
feedback.jsonl  ✔/✘ 反馈，追加写入，不回写历史
dictionary.json 个性化词典快照（增删查的持久化载体）
logs/app.log    运行日志

五、词典维护
词典提供增（Add）、删（Remove）、查（Lookup）三种能力，纠错仅在安全前提下替换：命中条目长度需达标、上下文边界匹配、替换前后不改变标点与数字，确保正常文本不被改坏。