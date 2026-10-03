个性化后台实现 README（运行说明）

一、启动
安装依赖：pip install -r requirements.txt
启动服务：python -m app.main --host 127.0.0.1 --port 8000 --data-dir ./data
说明：服务只监听回环地址，传入非 127.0.0.1 / ::1 的地址会直接拒绝启动。
鉴权为占位实现但必须携带：请求头 Authorization: Bearer <token>，token 取环境变量 APP_TOKEN，默认 dev-token。

二、HTTP 端点
GET  /v1/health   返回 {"status":"ok","dict_size":N,"data_dir":"..."}
POST /v1/process  JSON 请求体：{"text":"...","session_id":"...","feedback":"ok|bad"}
                  响应体：{"intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE","corrected_text":"...","changes":[...],"trace_id":"..."}

三、能力对应
个性化词典：条目支持增/删/查，可选匹配方式（精确/前缀/正则），正则与非法输入做转义与纠错安全校验，避免误替换。
文本纠错：先清洗（空白、全半角、控制字符），再按词典做纠错；未命中条目时原样返回，保证正常文本不被改坏。
意图分类：NOTE、QUERY、EDIT、COMMIT、ORCHESTRATE 五类，规则+词典加权，结果写入 traces。
反馈学习：请求携带 feedback=ok/bad（✔/✘）时追加落盘 feedback.jsonl，只追加不覆写。

四、数据文件（均在 data-dir 下，JSONL 或 JSON，append-only）
dictionary.json   个性化词典条目（增删查）
feedback.jsonl    反馈回馈，✔/✘ 逐行追加
traces.jsonl      处理轨迹：原文、纠错后文本、命中词典、意图、耗时
usage.jsonl       调用用量：时间、端点、状态码、token 数

五、快速自检
curl -s http://127.0.0.1:8000/v1/health -H "Authorization: Bearer dev-token"
curl -s -X POST http://127.0.0.1:8000/v1/process -H "Authorization: Bearer dev-token" -H "Content-Type: application/json" -d "{\"text\":\"帮我记一下明天开会\",\"feedback\":\"ok\"}"

六、注意
data-dir 可用参数或环境变量 DATA_DIR 覆盖，服务启动时自动创建目录与文件。
所有写入均为 append-only，进程重启后数据保留，不做原地修改。