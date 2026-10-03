个性化后台 —— 运行说明

一、环境与构建
依赖 Go 1.21+。在项目根目录执行：
go build -o bin/personal-server ./cmd/server
（若入口为根目录 main.go，则 go build -o bin/personal-server .）

二、启动命令
./bin/personal-server --addr 127.0.0.1:8787 --data-dir ./data --token dev-token
参数说明：
--addr 监听地址，默认 127.0.0.1:8787。仅允许回环地址，非 127.0.0.1/::1 将拒绝启动。
--data-dir 数据目录，默认 ./data，启动时自动创建。
--token 占位鉴权令牌，请求需带 Authorization: Bearer <token>；留空则仅回环校验。
启动后终端打印 "listening on 127.0.0.1:8787, data-dir=./data"。

三、HTTP 端点
GET  /v1/health
     返回 {"status":"ok","version":"...","data_dir":"..."}，用于存活探针。

POST /v1/process
     请求头：Content-Type: application/json，Authorization: Bearer <token>
     请求体示例：
     {"session_id":"s1","text":"帮我记一下 明天十点开会","action":"","feedback":""}
     字段：text 必填；session_id 可选（缺省生成）；action 可选，取值 correct|classify|dict；feedback 可选，取值 ok|bad，用于反馈学习。
     响应示例：
     {"ok":true,"session_id":"s1","intent":"NOTE","corrected":"帮我记一下 明天十点开会",
      "changes":[{"from":"...","to":"...","type":"dict"}],"elapsed_ms":3}
     intent 取值：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE。
     corrected 仅在确有改动时与原文本不同，未命中词典时原样返回，保证正常文本不被改坏。

四、数据文件（全部 append-only JSONL，位于 data-dir）
data/dictionary.json   个性化词典，JSON 数组，支持增/删/查；写入采用临时文件+原子替换，避免半行损坏。
data/feedback.jsonl    反馈学习记录，每行一条：{"ts":...,"session_id":...,"before":...,"after":...,"intent":...,"feedback":"ok|bad"}。
data/traces.jsonl      每次 /v1/process 的调用轨迹，含入参摘要、命中词典、意图、耗时。
data/usage.jsonl       用量统计，每行一条：{"ts":...,"endpoint":"/v1/process","status":200,"elapsed_ms":...}。
以上 JSONL 仅追加不覆盖，按行解析；单行损坏时跳过并计数告警，不影响服务。

五、词典条目格式
{"term":"开会","canonical":"开会","aliases":["开 会"],"priority":10}
term 为匹配词，canonical 为纠错输出的规范形，aliases 为别名，priority 越大越优先。纠错只做等长/受控替换与安全边界校验，不跨标点、不改变数字与专有名词。

六、快速自检
curl -s http://127.0.0.1:8787/v1/health
curl -s -X POST http://127.0.0.1:8787/v1/process -H 'Content-Type: application/json' -H 'Authorization: Bearer dev-token' -d '{"text":"帮我记一下明天十点开会"}'
tail -n 5 data/traces.jsonl