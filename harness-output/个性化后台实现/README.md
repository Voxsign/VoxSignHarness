个性化后台 —— 运行说明

一、前置条件
Go 1.21 及以上；无需外部数据库，所有数据以文件形式落盘。

二、构建
在项目根目录执行：
go build -o bin/personalize .
若输出二进制失败，先执行 go vet ./... 与 go build ./... 定位编译错误。

三、启动
默认（仅监听回环）：
./bin/personalize --data-dir ./data --addr 127.0.0.1:8080

参数与环境变量：
--addr      监听地址，默认 127.0.0.1:8080；非 127.0.0.1/::1 的地址会被拒绝启动
--data-dir  数据目录，默认 ./data，首次启动自动创建
--token     占位鉴权口令，默认空；非空时请求需带 Authorization: Bearer <token>
也可用环境变量覆盖：ADDR、DATA_DIR、AUTH_TOKEN

四、HTTP 端点（请求与响应均为 JSON，UTF-8）
GET  /v1/health
     返回 {"status":"ok","version":"...","data_dir":"..."}

POST /v1/process
     请求 {"user_id":"u1","text":"待处理文本","action":"auto"}
     返回 {"trace_id":"...","intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE",
           "corrected":"纠错后文本","changed":true|false,"hits":[词条...]}
     说明：清洗 + 词典纠错只做安全替换，未命中词典的正常文本原样返回，changed=false。

GET  /v1/dict            查询词典，支持 ?q=关键词
POST /v1/dict            新增条目 {"term":"词","replacement":"替换","note":"备注"}
DELETE /v1/dict?term=词  删除条目

POST /v1/feedback
     请求 {"trace_id":"...","verdict":"up|down","note":"可选"}
     返回 {"ok":true}
     说明：verdict 为 up（✔）或 down（✘）；也可在 /v1/process 请求中携带同名字段一次写入。

鉴权：--token 非空时，除 /v1/health 外均需 Authorization: Bearer <token>；口令错误返回 401。

五、数据文件（均在 --data-dir 指定目录下，append-only JSONL）
data/traces.jsonl    每次 /v1/process 的输入、纠错结果、意图与 trace_id
data/usage.jsonl     调用计量：时间戳、端点、状态码、耗时
data/feedback.jsonl  反馈回馈：trace_id、verdict、note、时间戳
data/dictionary.json 个性化词典快照（条目增删后整体重写，读取时原子替换）

写入规则：每行一条独立 JSON，只追加不修改；文件按需创建，进程异常退出后可继续追加，不会截断历史记录。

六、快速自检
curl http://127.0.0.1:8080/v1/health
curl -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -d '{"user_id":"u1","text":"明天十点开会"}'
curl -X POST http://127.0.0.1:8080/v1/feedback -H 'Content-Type: application/json' -d '{"trace_id":"<上一步返回>","verdict":"up"}'
随后检查 data/traces.jsonl 与 data/feedback.jsonl 是否各新增一行。

注：以上命令假定 main.go 已完成 P0 实现并可编译通过；若当前仍是骨架或存在编译错误，需先补齐实现再按本文档验收。