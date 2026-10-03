个性化后台实现 — 运行说明

一、环境与构建
需要 Go 1.21 或以上。
构建：go build -o personald .
直接运行（不编译）：go run .

二、启动命令
默认启动：./personald
指定数据目录与端口：./personald -data-dir ./data -addr 127.0.0.1:8080
常用参数：
  -data-dir   数据目录，默认 ./data，首次启动自动创建
  -addr       监听地址，默认 127.0.0.1:8080，非回环地址启动即拒绝
  -token      占位鉴权令牌，默认 dev-token，可留空但鉴权头必须存在

三、HTTP 端点
GET  /v1/health
  返回 {"status":"ok","version":"...","uptime_s":N}
  用于存活探测，无需请求体。

POST /v1/process
  请求：Content-Type: application/json，头 Authorization: Bearer <token>
  body 字段：text（必填，待处理文本）、user_id（可选，默认 anonymous）、
             action（可选，note/query/edit/commit/orchestrate，缺省走意图分类）
  响应：{"ok":true,"intent":"NOTE","corrected":"...","matches":[...],"trace_id":"..."}
  说明：清洗与词典纠错只替换命中词条，未命中文本原样保留；
        空文本或超长文本返回 400，鉴权失败返回 401。

POST /v1/feedback
  请求：{"trace_id":"...","verdict":"up|down"} 或 {"trace_id":"...","ok":true|false}
  作用：把 ✔/✘ 反馈追加写入 feedback.jsonl，用于后续学习。

词典管理（供 ① 增/删/查）：
GET    /v1/dict            查询全部条目
GET    /v1/dict?q=关键词    按词或别名查询
POST   /v1/dict            新增 {"term":"...","aliases":["..."],"correct":"..."}
DELETE /v1/dict?term=词     删除条目

四、数据文件（均在 -data-dir 下，全部 append-only）
dictionary.json   个性化词典，增删改后整体重写，写入前做临时文件替换
feedback.jsonl    反馈学习记录，每行一条，只追加不修改
traces.jsonl      每次 /v1/process 的处理链路，每行一条，只追加
usage.jsonl       端点调用与耗时统计，每行一条，只追加

五、行为约束
服务只监听 127.0.0.1（或 ::1）；传入其他地址时进程直接退出并提示。
所有写盘操作先追加后返回，进程中断不丢已确认记录。
纠错为保守策略：词典未命中的片段一律不改，避免正常文本被改坏。
意图分类输出固定五类：NOTE、QUERY、EDIT、COMMIT、ORCHESTRATE。
鉴权当前为占位实现（Bearer token 比对），后续可替换为真实鉴权。