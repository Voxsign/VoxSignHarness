# 个性化后台 · 运行说明

环境要求
Go 1.21 及以上版本，无外部依赖。

构建与启动
go build -o personald .
./personald -addr 127.0.0.1:8080 -data-dir ./data -token dev-token

调试运行（不产物化）
go run . -addr 127.0.0.1:8080 -data-dir ./data

启动参数
-addr      监听地址，默认 127.0.0.1:8080；只接受回环地址，非 127.0.0.1 直接拒绝启动。
-data-dir  数据目录，默认 ./data，自动创建。
-token     鉴权占位令牌；未配置时按空令牌放行。

端点
GET /v1/health
  健康检查，返回 {"status":"ok"}，不需鉴权。

POST /v1/process
  请求头 Authorization: Bearer <token>
  请求体 JSON：{"text":"待处理文本","op":"note|query|edit|commit|orchestrate"}
  op 可省略，省略时由意图分类自动判定。
  响应 JSON：{"intent":"NOTE","corrected":"纠错后文本","hits":[词典命中项],"reply":"处理结果"}
  若 op 为字典操作，用 "dict":"add|del|get" 与 "entry":"词条" 指定，走同一端点。

词典
  增删查均通过 /v1/process 携带 dict 字段完成；条目落盘于 data-dictionary 下的词典文件，进程重启后自动加载。
  纠错为增量的：无把握的片段原样保留，正常文本不会被改写。

数据文件（全部 append-only，位于 -data-dir 指定的目录）
  traces.jsonl    每次 /v1/process 的完整轨迹
  usage.jsonl     调用计数与耗时
  feedback.jsonl  ✔/✘ 反馈回执，仅追加不覆盖

反馈写入
POST /v1/process 后附 {"feedback":"up"} 或 {"feedback":"down"}，追加一行到 feedback.jsonl。

注意事项
监听地址固定回环，不要改成 0.0.0.0。
数据目录可用 -data-dir 指向任意可写路径，多个实例请勿共用同一目录。