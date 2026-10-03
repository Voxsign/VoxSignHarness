个性化后台实现 README

一、构建
go build -o pbackend .
（若单文件实现：go build -o pbackend main.go）

二、启动
默认监听 127.0.0.1:8080，数据目录 ./data：
./pbackend

指定端口与数据目录：
./pbackend -addr 127.0.0.1:9000 -data-dir /var/lib/pbackend

等价环境变量：
PB_ADDR=127.0.0.1:9000 PB_DATA_DIR=/var/lib/pbackend ./pbackend

鉴权（占位，可覆盖）：
PB_TOKEN=devtoken ./pbackend
请求需带请求头 Authorization: Bearer devtoken；未配置 PB_TOKEN 时放行。

仅监听回环：若 -addr 的 host 不是 127.0.0.1、::1 或 localhost，进程直接报错退出，非回环地址一律拒绝。

三、端点
GET  /v1/health
  返回 200，JSON：{"status":"ok","time":"<RFC3339>","data_dir":"<路径>"}

POST /v1/process
  请求体 JSON：{"text":"...","user_id":"u1","action":"process"}
  action 可选：process（默认，走清洗+纠错+意图分类）、dict_add、dict_del、dict_get、feedback
  响应 JSON 字段：intent（NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE）、clean_text、corrected_text、corrections[]、dict_hits[]、trace_id

示例：
  curl -s -H "Authorization: Bearer devtoken" -H "Content-Type: application/json" -d '{"text":"帮我记一下明天开会","user_id":"u1"}' http://127.0.0.1:8080/v1/process
  返回意图 NOTE，corrected_text 与原文一致（正常文本不被改坏）。

词典管理（同一端点，action 区分）：
  增：{"action":"dict_add","term":"后台","aliases":["后抬","后太"]}
  删：{"action":"dict_del","term":"后台"}
  查：{"action":"dict_get"} 或 {"action":"dict_get","term":"后台"}（term 为空返回全部）

反馈学习：
  {"action":"feedback","trace_id":"<上一步返回的 trace_id>","label":"up"}
  label 取值 up/down（对应 ✔/✘），追加写入 feedback.jsonl。

四、数据文件（全部 append-only JSONL，位于 data-dir 下，文件不存在自动创建）
data/dictionary.json   个性化词典（条目快照，走原子替换写入，便于重启加载）
data/traces.jsonl      每次 /v1/process 的输入、输出、trace_id
data/usage.jsonl       每次请求的耗时、状态、端点、user_id
data/feedback.jsonl    反馈记录：trace_id、label、时间戳

落盘策略：每条记录一行 JSON，只追加不修改；写入即 fsync，进程崩溃不影响已落盘内容。

五、纠错与意图说明
清洗：去多余空白、统一全角/半角标点、去零宽字符；不改变语义。
纠错：仅在命中词典别名或已知错字表时替换，替换结果记录在 corrections[]，未命中的文本原样保留。
意图分类：按关键词与句式规则划分 NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE，兜底为 QUERY。

六、快速自检
1) ./pbackend 启动后 health 返回 ok
2) dict_add 后 dict_get 能查到该条目
3) 用别名文本调用 process，corrected_text 被纠正为词典主词
4) 发送正常句子，corrected_text 与 clean_text 不被改动
5) 五类意图样例分别返回对应 intent
6) 调用 feedback 后 data/feedback.jsonl 新增一行
7) 用 -addr 0.0.0.0:8080 启动应直接失败退出