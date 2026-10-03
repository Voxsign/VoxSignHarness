README.md（中文运行说明）

个性化后台实现 —— 本地优先的个人化文本处理服务

一、环境要求
Go 1.21 或以上；无需数据库，数据全部以 JSONL 文件追加写入本地数据目录。

二、启动命令
构建：go build -o pbackend .
启动（默认）：./pbackend
指定监听与数据目录：./pbackend -addr 127.0.0.1:8080 -data-dir ./data -token dev-token
参数说明：
-addr 监听地址，默认 127.0.0.1:8080，仅允许回环地址，非 127.0.0.1 的地址会被拒绝启动。
-data-dir 数据目录，默认 ./data，不存在时自动创建。
-token 占位鉴权令牌；留空时不校验。请求需带 Authorization: Bearer <token>。

三、HTTP 端点
GET /v1/health
  健康检查，返回 {"status":"ok"}，无需鉴权。

POST /v1/process
  统一业务入口，Content-Type: application/json，需鉴权。
  请求体示例：
  {"text":"明天开会要带合同","intent":"","feedback":null}
  字段：text 待处理文本；intent 可选，指定时跳过分类；feedback 可选，取值 "ok" 或 "bad"。
  响应体示例：
  {"ok":true,"intent":"NOTE","corrected":"明天开会要带合同","edits":[],"trace_id":"..."}
  intent 取值：NOTE、QUERY、EDIT、COMMIT、ORCHESTRATE。

四、个性化词典接口
POST /v1/dict/add    新增条目，body: {"term":"合同","aliases":["合约"],"weight":1}
POST /v1/dict/delete 删除条目，body: {"term":"合同"}
GET  /v1/dict/list   查询全部条目

五、数据文件（均位于 -data-dir 目录，JSONL，append-only，只追加不覆写）
traces.jsonl   每次 /v1/process 的输入、纠错结果、意图与 trace_id
usage.jsonl    调用统计：时间戳、端点、耗时、状态码
feedback.jsonl 反馈学习记录：trace_id、反馈值 ok/bad、时间戳
dict.json      个性化词典快照，供启动时加载

六、行为说明
文本纠错先做清洗（去多余空白、全半角归一），再按词典做匹配替换；未命中词典的文本保持原样，正常文本不会被改坏。
意图分类按关键词与词典权重规则判定，无法判定时归为 QUERY。
反馈回馈仅追加写入 feedback.jsonl，不阻塞主流程，写入失败只记录日志。