个性化后台 · 运行说明

一、启动

默认（监听 127.0.0.1:8080，数据目录 ./data）：
go run . 

指定监听地址与数据目录：
go run . -addr 127.0.0.1:8080 -data-dir ./data

编译产物：
go build -o botd . && ./botd -addr 127.0.0.1:8080 -data-dir ./data

说明：
- -addr 仅允许回环地址，非 127.0.0.1 / ::1 / localhost 直接启动失败退出。
- -data-dir 不存在时自动创建，指数与 JSONL 均落在此目录，可随时改配置切换。
- 鉴权为占位实现：请求头 Authorization: Bearer <token>，token 由 -token 指定，缺省 <dev-token>；缺失/错误返回 401。

二、端点

GET /v1/health
返回 {"status":"ok","uptime_s":..,"data_dir":".."}

POST /v1/process
请求 JSON：
{"text":"...","user_id":"u1","feedback":"up|down|null","op":"process|dict_add|dict_del|dict_get"}
响应 JSON：
{"trace_id":"..","intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE","corrected_text":"..","changed":true|false,"dict_hits":[".."],"usage":{"ms":..}}

字段语义：
- text 必填；op 缺省 process。
- intent 五类：NOTE（记录）、QUERY（查询）、EDIT（修改）、COMMIT（提交）、ORCHESTRATE（编排调度）。
- corrected_text 为清洗+词典纠错后的结果；正常文本原样返回，changed=false。
- dict_add 传 {"term":"..","alias":[".."]}；dict_del 传 {"term":".."}；dict_get 传 {"term":".."} 或空返回全部。
- feedback 传 up/down 才会写反馈文件，null 或缺失不写。

三、数据文件（均为 append-only，同目录，进程重启不截断）

data/dictionary.json   个性化词典条目（增删查的持久层）
data/feedback.jsonl    反馈明细，每行 {"ts":..,"trace_id":..,"user_id":..,"text":..,"intent":..,"vote":"up|down"}
data/traces.jsonl      处理轨迹，每行一次 /v1/process 的输入/输出/耗时
data/usage.jsonl       用量统计，每行 {"ts":..,"endpoint":..,"intent":..,"ms":..,"status":..}

查看：
tail -f data/traces.jsonl

四、快速自测

curl -s 127.0.0.1:8080/v1/health

curl -s -X POST 127.0.0.1:8080/v1/process \
  -H 'Authorization: Bearer dev-token' -H 'Content-Type: application/json' \
  -d '{"text":"帮我记一下明天开会","user_id":"u1"}'

curl -s -X POST 127.0.0.1:8080/v1/process \
  -H 'Authorization: Bearer dev-token' -H 'Content-Type: application/json' \
  -d '{"op":"dict_add","term":"开例会","alias":["开会"]}'

验证落盘（应各新增一行）：
wc -l data/traces.jsonl data/usage.jsonl data/feedback.jsonl