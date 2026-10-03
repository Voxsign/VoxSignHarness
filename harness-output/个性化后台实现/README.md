个性化后台实现 · 运行说明

一、环境与构建
依赖 Go 1.21+。编译：go build -o bin/backend ./cmd/server
若编译失败请先执行 go mod tidy 拉齐依赖。

二、启动
默认启动：go run ./cmd/server
指定数据目录与端口：go run ./cmd/server --data-dir ./data --addr 127.0.0.1:8787
或使用已编译产物：./bin/backend --data-dir ./data --addr 127.0.0.1:8787

启动参数
--data-dir  数据目录，默认 ./data，不存在时自动创建
--addr      监听地址，默认 127.0.0.1:8787
--token     鉴权占位令牌，默认 dev-token；请求带 Authorization: Bearer <token>
--help      查看全部参数

安全约束：服务仅绑定回环地址，地址非 127.0.0.1/::1 时启动直接拒绝并退出，不降级监听。

三、HTTP 端点
GET  /v1/health    健康检查，返回 {"status":"ok","version":...}
POST /v1/process   主处理入口，请求体 JSON：
                   {"text":"...","action":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE","feedback":"ok|bad","item":"...","op":"add|del|get|list"}
                   action 省略时由意图分类自动判定；op 用于个性化词典的增/删/查/列。
                   响应 JSON 含 corrected、intent、matched、dict_hits、trace_id 等字段。
POST /v1/feedback  反馈落盘，等价于 /v1/process 中带 feedback 字段。
所有端点除 /v1/health 外均需 Bearer 令牌。

四、能力说明
个性化词典：条目增（op=add）、删（op=del）、查（op=get）、列（op=list）；命中采用词边界与长度优先匹配，纠错仅在置信度达标时替换，避免改坏正常文本。
文本纠错：先清洗（空白、全半角、控制字符），再走词典纠错，无命中则原样返回。
意图分类：NOTE、QUERY、EDIT、COMMIT、ORCHESTRATE 五类，规则+词典加权打分。
反馈学习：✔/✘ 结果以 append-only 方式写入 feedback.jsonl，用于后续词典权重调整。

五、数据文件（均位于 --data-dir，全部 append-only，不做原地改写）
dictionary.json  个性化词典条目（唯一允许整体重写，写入采用临时文件+原子替换）
feedback.jsonl   反馈记录，追加写
traces.jsonl     每次 /v1/process 的完整轨迹，追加写
usage.jsonl      调用计数与耗时，追加写

六、快速验证
启动后执行：curl -i http://127.0.0.1:8787/v1/health
处理请求：curl -X POST http://127.0.0.1:8787/v1/process -H "Authorization: Bearer dev-token" -H "Content-Type: application/json" -d '{"text":"帮我记一下明天开会"}'
查词典：curl -X POST http://127.0.0.1:8787/v1/process -H "Authorization: Bearer dev-token" -d '{"op":"add","item":"开会"}'
确认落盘：查看 data/traces.jsonl 与 data/feedback.jsonl 是否新增行。