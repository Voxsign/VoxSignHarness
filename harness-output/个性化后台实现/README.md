个性化后台实现 —— 运行说明

一、构建与启动

构建：
go build -o bin/personalize ./cmd/server

启动（默认仅监听回环）：
./bin/personalize -addr 127.0.0.1:8080 -data-dir ./data -token dev-token

参数说明：
-addr      监听地址，默认 127.0.0.1:8080；非回环地址（如 0.0.0.0、公网 IP）会在启动时直接拒绝并退出。
-data-dir  数据目录，默认 ./data，首次启动自动创建。
-token     鉴权占位令牌，默认 dev-token；请求需带 Authorization: Bearer <token>。

开发期也可直接跑：
go run ./cmd/server -data-dir ./data

二、HTTP 端点

GET /v1/health
返回服务状态、版本、数据目录可写性与各 JSONL 文件行数。
示例响应：{"status":"ok","version":"0.1.0","data_dir":"./data","writable":true}

POST /v1/process
统一业务入口，Content-Type: application/json。按 action 分发，所有子能力共用同一端点。

请求体通用结构：
{"action":"<名称>","session_id":"<可选>","payload":{...}}

action 取值与 payload：

intent —— 意图分类，五类：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
payload: {"text":"记一下明天十点开会"}
响应:    {"intent":"NOTE","confidence":0.92}

correct —— 文本纠错（清洗 + 词典纠错，命中不确定时保持原文不改写）
payload: {"text":"今天看一下 kubernets 文档"}
响应:    {"text":"今天看一下 Kubernetes 文档","changed":true,"hits":[{"from":"kubernets","to":"Kubernetes"}]}

dict.add —— 新增词条
payload: {"term":"Kubernetes","aliases":["kubernets","k8s"],"tags":["tech"]}

dict.del —— 删除词条（按 term 精确匹配）
payload: {"term":"Kubernetes"}

dict.get —— 查询词条（term 为空时列出全部，支持前缀匹配）
payload: {"term":"Kube","prefix":true}

feedback —— 反馈学习，落盘 append-only
payload: {"session_id":"s1","trace_id":"t1","signal":"up","note":"分类正确"}
signal 取值：up（✔）/ down（✘）。

process —— 串联一步：清洗 → 纠错 → 意图分类（可选写 traces/usage）
payload: {"text":"...","write_trace":true}

响应统一形如：
{"ok":true,"action":"correct","result":{...},"trace_id":"t1"}

错误响应：{"ok":false,"error":{"code":"bad_request","message":"..."}}

三、数据文件（全部 append-only JSONL，位于 data-dir）

dictionary.jsonl  词典事件流（add/del），启动时回放重建内存索引
feedback.jsonl    反馈记录，✔/✘ 逐行追加
traces.jsonl      请求链路：trace_id、action、耗时、命中结果
usage.jsonl       调用计数：action、session_id、时间戳

示例：
data/dictionary.jsonl
{"ts":"2026-01-01T10:00:00Z","op":"add","term":"Kubernetes","aliases":["kubernets","k8s"]}
data/feedback.jsonl
{"ts":"2026-01-01T10:00:05Z","session_id":"s1","trace_id":"t1","signal":"up"}

说明：文件只追加不重写；删除通过追加 op=del 事件生效；异常中断后可安全重启，回放按时间顺序合并。

四、安全与边界

仅绑定 127.0.0.1 / ::1；检测到非回环监听地址即拒绝启动。
鉴权为占位实现，校验 Bearer 令牌一致性，缺失或错误返回 401。
纠错采用高置信度替换策略：词典未命中或存在歧义时保留原文，确保正常文本不被改坏。