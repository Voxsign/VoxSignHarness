个性化后台实现 · 运行说明

一、启动
go build -o pkb .        # 需编译通过，main.go 为完整实现，无 todo 占位
./pkb -addr 127.0.0.1:8787 -data-dir ./data -token dev-token

参数说明
-addr     监听地址，必须为回环地址（127.0.0.1 或 ::1）；传入 0.0.0.0 等非回环地址时启动失败并退出，不做降级。
-data-dir 数据目录，默认 ./data，启动时自动创建。
-token    鉴权占位令牌，默认 dev-token，请求需带 Authorization: Bearer <token>；缺失或错误返回 401。

二、HTTP 端点
GET  /v1/health
  返回 200 与 {"status":"ok","data_dir":"...","dict_size":N}
POST /v1/process
  请求头 Content-Type: application/json，Authorization: Bearer <token>
  请求体字段：
    text     必填，待处理文本
    user_id  选填，用于反馈与用量归属
    op       选填，note|query|edit|commit|orchestrate，缺省由意图分类自动判定
  响应体字段：
    ok       布尔
    intent   NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE 之一
    corrected  词典纠错后的文本（正常文本原样返回，不被改坏）
    matched  命中的词典条目列表
    reply    按意图生成的应答
    trace_id 本次轨迹号，可用于反馈回执
POST /v1/feedback
  请求体 {"trace_id":"...","label":"up|down","user_id":"..."}，落盘 feedback.jsonl

三、数据文件（均在 data-dir 下，除 dict.json 外均为 append-only JSONL，只追加不改写）
dict.json        个性化词典，增/删/查条目，写盘采用临时文件 + 原子重命名
feedback.jsonl   反馈学习记录，✔/✘ 每次一行
traces.jsonl     处理轨迹，每个请求一行
usage.jsonl      用量统计，含意图分布与耗时
文件按天不需要轮转，直接追加；读取时对损坏行跳过并计数，不影响服务。

四、能力与实现要点
个性化词典  词典增删查接口 + 匹配与纠错安全：仅命中条目才替换，长度/相似度阈值外不改写，避免误纠。
文本纠错    先做清洗（空白、全半角、不可见字符），再走词典纠错，未命中一律原样返回。
意图分类    NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE 五类，规则打分 + 关键词加权，命中词典条目可提权。
反馈学习    feedback.jsonl append-only，进程内维护权重，重启后按行回放恢复。
数据落盘    全部 JSONL 追加写，写入带 fsync；data-dir 可配，路径不存在则创建。

五、自检
curl http://127.0.0.1:8787/v1/health
curl -X POST http://127.0.0.1:8787/v1/process -H "Authorization: Bearer dev-token" -H "Content-Type: application/json" -d '{"text":"帮我把周报记一下","user_id":"u1"}'
curl -X POST http://127.0.0.1:8787/v1/feedback -H "Authorization: Bearer dev-token" -H "Content-Type: application/json" -d '{"trace_id":"T1","label":"up","user_id":"u1"}'

写入完成后，tail -n 1 data/traces.jsonl 与 data/usage.jsonl 应各新增一行。