README.md

个性化后台实现

一、环境与构建
依赖 Go 1.21 及以上版本，无需外部数据库。
构建：go build -o ppb .
（上一轮 main.go 为骨架、无法编译，本轮已补齐完整实现，编译应通过：go build ./... 无输出即成功。）

二、启动
默认监听回环地址，数据目录为 ./data：
  ./ppb

自定义端口与数据目录：
  ./ppb -addr 127.0.0.1:8080 -data-dir ./data

启动即校验监听地址，非 127.0.0.1 直接拒绝启动（防止误暴露到外网）。

三、鉴权
请求头携带 Authorization: Bearer <token>，token 由 -token 指定，默认占位值 dev-token。
仅作占位校验，未匹配返回 401；/v1/health 免鉴权。

四、HTTP 端点
1. GET /v1/health
   返回 {"status":"ok","addr":"127.0.0.1:8080","data_dir":"./data"}

2. POST /v1/process
   请求体 JSON：
     {"text":"...","user_id":"u1","action":"NOTE"}
   返回 JSON：
     {"intent":"NOTE","corrected":"...","cleaned":"...","hits":[{"term":"...","score":0.93}],"trace_id":"..."}
   intent 取值限定为 NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE 五类，无法判定时回落 NOTE。
   清洗与纠错保证正常文本不被改坏：仅在词典命中且满足纠错安全阈值时替换，否则原样返回。

3. 词典增删查
   POST   /v1/dict        {"term":"xxx","alias":["yyy"],"weight":1.0}   新增
   DELETE /v1/dict/{term}                                                删除
   GET    /v1/dict?q=xxx                                                 查询（前缀/模糊匹配）

4. POST /v1/feedback
   {"trace_id":"...","verdict":"up"}  其中 verdict 为 up(✔) 或 down(✘)。

五、数据文件（独立数据目录，默认 ./data，可用 -data-dir 覆盖）
dictionary.json   个性化词典条目，读写型，保存时原子替换
feedback.jsonl    反馈学习结果，append-only，每行一条
traces.jsonl      每次 /v1/process 的处理轨迹，append-only
usage.jsonl       调用量统计（端点、耗时、状态码），append-only

JSONL 一律以追加方式写入并立即 flush，不重写历史行；文件按行解析，坏行跳过不影响后续。

六、快速自检
go build ./... && ./ppb 启动后：
curl http://127.0.0.1:8080/v1/health
curl -X POST http://127.0.0.1:8080/v1/process -H 'Authorization: Bearer dev-token' -H 'Content-Type: application/json' -d '{"text":"记一下 明天开周会","user_id":"u1"}'
tail -n 1 ./data/traces.jsonl