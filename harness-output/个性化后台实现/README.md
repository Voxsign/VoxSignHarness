个性化后台 运行说明

一、环境
Python 3.10+，仅标准库（可选依赖见 requirements.txt）。
默认全部数据写入独立数据目录，不在源码目录内落任何文件。

二、启动
python -m app.server --host 127.0.0.1 --port 8088 --data-dir ./data
可选参数：
--token  devtoken            占位鉴权令牌，不传则读环境变量 ADMIN_TOKEN，两者都无则鉴权关闭但仍保留校验入口
--data-dir ./data            数据目录，不存在则自动创建
--dict ./data/dict.json      个性化词典路径，默认取 data-dir/dict.json

仅允许绑定 127.0.0.1 / ::1。传入非回环地址（如 0.0.0.0）直接启动失败并报错退出。
服务起来后打印监听地址、数据目录、鉴权状态。

三、端点
GET  /v1/health
     返回 {"status":"ok","version":"...","data_dir":"...","uptime_s":N}
     不需要鉴权。

POST /v1/process
     请求：{"text":"...","action":"auto","feedback":null}
     响应：{"trace_id":"...","intent":"NOTE","corrected":"...","edits":[...],"hits":[...]}
     intent 取值：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
     corrected 为清洗 + 词典纠错后的文本，正常文本原样返回（无改动则 corrected == text，edits 为空）
     action 可选 dict_add / dict_del / dict_get / dict_list，用于个性化词典增删查；
       dict_add: {"action":"dict_add","term":"...","replacement":"...","note":"..."}
       dict_del: {"action":"dict_del","term":"..."}
       dict_get: {"action":"dict_get","term":"..."}
       dict_list: {"action":"dict_list"}（可选 "limit"/"offset"）

POST /v1/feedback
     请求：{"trace_id":"...","verdict":"up"}  verdict 取 up(✔) / down(✘)，可带 "note"
     以 append-only 方式追加到 feedback.jsonl，用于后续学习。

鉴权（占位）：除 /v1/health 外，均需 Authorization: Bearer <token>。校验函数已留出接口，替换为真实实现即可。

四、数据文件（均为 JSONL append-only，除 dict.json 外）
data/dict.json        个性化词典，结构 {"version":1,"entries":[...]}，写入用临时文件 + 原子替换
data/feedback.jsonl   反馈记录，每行 {"ts","trace_id","verdict","note","snapshot"}
data/traces.jsonl     每次 /v1/process 的完整轨迹：原始文本、清洗结果、纠错 edits、命中词条、最终 intent
data/usage.jsonl      调用用量：ts、endpoint、action、耗时 ms、状态码、是否命中词典

五、自检
GET /v1/health 返回 ok 即服务正常。
发送一条测试请求确认链路：
curl -s -X POST http://127.0.0.1:8088/v1/process -H "Content-Type: application/json" -d '{"text":"帮我记一下明天开会"}'
预期 intent=NOTE，corrected 与原文一致或仅有词典命中替换。
确认数据目录已生成 feedback.jsonl / traces.jsonl / usage.jsonl，且每次调用行数只增不减。