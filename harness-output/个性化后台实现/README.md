个性化后台实现

运行环境
Python 3.10+，依赖见 requirements.txt。默认数据目录 ./data，首次启动自动创建。

启动
python -m personal_backend --data-dir ./data --port 8787
可选参数：--auth-token <token> 开启鉴权占位（请求头 X-Auth-Token，默认关闭但校验链路已接好）。
服务强制只监听 127.0.0.1；若传入非回环地址（如 0.0.0.0）将直接拒绝启动。

端点
GET  /v1/health
  返回 {"status":"ok","version":"...","data_dir":"..."}，用于存活探测。

POST /v1/process
  请求体 JSON：{"action":"process"|"dict.add"|"dict.delete"|"dict.list","text":"...","entry":{...}}
  - action 缺省为 process。
  - process 返回：{"intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE","corrected":"...","corrections":[...],"trace_id":"..."}
  - dict.add/dict.delete 返回：{"ok":true,"entry":{...}}；dict.list 返回：{"entries":[...]}
  - 文本纠错只做安全的词典级替换与清洗，命中不确定时保持原文，正常文本不被改坏。
  所有请求与响应均为 JSON，UTF-8。

数据文件（均在 data-dir 下，append-only，写入即落盘）
dictionary.json   个性化词典条目（增删改查的唯一真相源）
traces.jsonl      每次 /v1/process 的输入、意图、纠错结果与耗时
usage.jsonl       调用计数与统计口径记录
feedback.jsonl    ✔/✘ 用户回馈，每行一条，只追加不修改

约定
- traces/usage/feedback 为 append-only，不提供删除接口；如需归档请直接迁移文件。
- 切换数据目录不会迁移历史文件，建议启动前确认 --data-dir 指向预期位置。
- 反馈写入以 trace_id 关联对应 traces 记录。