个性化后台实现 README

一、环境
Python 3.10+，仅使用标准库，无第三方依赖。默认数据目录 ./data，启动时自动创建。

二、启动

python -m app.server --host 127.0.0.1 --port 8765 --data-dir ./data

或

python server.py --port 8765 --data-dir ./data

可配项（命令行参数优先于环境变量）：
--host 默认 127.0.0.1，仅允许回环地址，传入 0.0.0.0 或其他非回环地址时启动即报错退出
--port 默认 8765
--data-dir 默认 ./data
PB_AUTH_TOKEN 默认 off，设为任意字符串即开启鉴权校验

三、鉴权
开启后请求需带请求头：Authorization: Bearer <token>
未开启时该请求头可为空，此实现为占位，不校验内容强度。

四、HTTP 端点
GET /v1/health
返回 {"status":"ok","version":"...","data_dir":"..."}

POST /v1/process
请求体 JSON：{"text":"...","id":"可选","feedback":"up|down|可选"}
响应体 JSON：{"id":"...","intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE","corrected":"...","cleaned":"...","dict_hits":[...],"message":"..."}

词典条目管理（同一端点，可选扩展路由）：
POST /v1/process 携带 {"op":"dict_add|dict_del|dict_get","term":"...","alias":"..."}

调用示例

curl -s http://127.0.0.1:8765/v1/health

curl -s -X POST http://127.0.0.1:8765/v1/process -H "Content-Type: application/json" -d '{"text":"记一下 明天开会"}'

curl -s -X POST http://127.0.0.1:8765/v1/process -H "Content-Type: application/json" -H "Authorization: Bearer dev" -d '{"id":"t1","feedback":"up"}'

五、数据文件（均在 --data-dir 下，独立目录，可整体迁移）
data/dict.json      个性化词典，增删改后整体原子写回
data/feedback.jsonl 反馈记录，append-only，每行一条 {"ts","id","feedback"}
data/traces.jsonl   处理轨迹，append-only，每行一条请求与判定结果
data/usage.jsonl    调用与用量统计，append-only，每行一条

落盘约定：JSONL 一律追加写入、不重写、不回读修改；每次写入后 flush。词典为读写型文件，仅通过接口修改。

六、行为要点
文本纠错先清洗再查词典，未命中条目保持原样，正常文本不被改写。
意图分类固定输出五类之一，无法判定时回落 NOTE。
反馈写入失败不影响本次响应，仅记录到 traces.jsonl。