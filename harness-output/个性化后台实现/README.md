个性化后台实现 README

一、运行环境
Go 1.21 及以上。仅监听回环地址 127.0.0.1，非回环地址直接拒绝。

二、启动
默认启动（数据目录 ./data，端口 8787）：
go run . 

指定端口与数据目录：
go run . -addr 127.0.0.1:8787 -data-dir ./data

编译后运行：
go build -o app .
./app -addr 127.0.0.1:8787 -data-dir ./data

参数说明
-addr      监听地址，只允许 127.0.0.1 或 localhost，其他地址启动即退出
-data-dir  数据目录，不存在会自动创建，存放全部 JSONL 与词典文件

三、HTTP 端点
GET /v1/health
  健康检查，返回服务状态、版本、数据目录路径。

POST /v1/process
  请求头：Content-Type: application/json
  可选鉴权头：Authorization: Bearer <token>（占位，默认放行）
  请求体字段：
    text        string  必填，待处理文本
    user_id     string  可选，用户标识，用于个性化词典与反馈归集
    feedback    string  可选，取值 ok 或 bad，回写反馈
  响应体字段：
    intent      string  NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
    text        string  纠错清洗后的文本
    corrections array   命中的词典纠错项，含原词与替换词
    trace_id    string  本次请求追踪号

四、数据文件（均为 append-only JSONL，落在 data-dir 下）
dictionary.json   个性化词典，含条目增删查结果
traces.jsonl      每次 /v1/process 的请求与响应快照
usage.jsonl       调用统计，含端点、耗时、意图分布
feedback.jsonl    用户 ✔/✘ 反馈，只追加不覆写

五、结束语
停止服务使用 Ctrl+C。数据目录可整体迁移或备份，各 JSONL 文件按行读取即可解析。