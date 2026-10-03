个性化后台实现 README

一、环境与构建
需要 Go 1.21 及以上，无第三方依赖。
在项目根目录执行：
go build -o bin/app .
要求 go vet ./... 与 go build ./... 均零错误。

二、启动
./bin/app -addr 127.0.0.1:8080 -data-dir ./data -token dev-token

参数说明
-addr     监听地址，默认 127.0.0.1:8080；非 127.0.0.1 / ::1 直接拒绝启动
-data-dir 数据目录，默认 ./data，首次启动自动创建
-token    占位鉴权令牌，请求头 Authorization: Bearer <token>；未配置时仅本机放行

三、HTTP 端点
GET  /v1/health
返回 {"status":"ok","time":...,"data_dir":...}，用于存活探测。

POST /v1/process
请求 JSON 字段：
  text     待处理原文（必填）
  action   可选，显式指定意图 NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
  feedback 可选，true 表示 ✔，false 表示 ✘
响应 JSON 字段：
  trace_id  本次处理编号
  cleaned   清洗后文本
  corrected 词典纠错后文本（无命中时与 cleaned 一致，正常文本不被改坏）
  intent    意图分类结果
  hits      命中的词典条目列表
  safe      纠错是否被安全策略拦截（低置信度不改写）

四、数据文件（全部追加写，位于 data-dir 下）
dictionary.json        个性化词典，增删查，写入采用临时文件 + 重命名
traces/traces.jsonl    每次 /v1/process 的输入、输出、命中与耗时
usage/usage.jsonl      用量计数：端点、意图、耗时
feedback/feedback.jsonl ✔/✘ 反馈，append-only，不覆盖不删除

五、词典管理（子命令）
./bin/app dict add -term <词条> -replacement <替换>
./bin/app dict del -term <词条>
./bin/app dict list

六、自检
启动后执行 curl http://127.0.0.1:8080/v1/health，返回 status 为 ok 即视为可用；
随后向 /v1/process 发送一条文本，确认 traces/traces.jsonl 新增一行且 feedback.jsonl 在提交反馈后新增一行。