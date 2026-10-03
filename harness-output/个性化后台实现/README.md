# 个性化后台实现

仅监听 127.0.0.1 的本地服务，所有数据以 append-only JSONL 落盘到可配置的 data 目录。

## 启动

切换到仓库根目录（含 main.go），二选一：

go run . -addr 127.0.0.1:8080 -data-dir ./data -token local-dev

go build -o personald . && ./personald -addr 127.0.0.1:8080 -data-dir ./data -token local-dev

参数
- -addr 监听地址，默认 127.0.0.1:8080；配置成非回环地址（如 0.0.0.0:8080）时进程直接拒绝启动并报错退出。
- -data-dir 数据目录，默认 ./data，不存在则自动创建。
- -token 鉴权占位令牌，默认 local-dev；传空字符串表示暂不校验。

## 鉴权

所有 /v1/* 请求需带请求头 Authorization: Bearer <token>。
缺失或不匹配返回 401 {"error":"unauthorized"}。
该实现为占位鉴权，仅用于本地自用，不构成生产安全边界。

## 端点

GET /v1/health
返回 200 与 {"status":"ok","data_dir":"./data","time":"<RFC3339>"}，不写任何数据文件。

POST /v1/process
请求体 JSON：
{"text":"明天九点提醒我开会","feedback":null,"dict":null}

字段说明
- text 必填，待处理文本。
- feedback 可选，取值 "up"（✔）或 "down"（✘），落盘反馈。
- dict 可选，词典操作对象，见下。

响应体 JSON：
{"trace_id":"...","intent":"NOTE","corrected":"明天9点提醒我开会","changed":true,"dict_hit":["提醒"],"errors":[]}

- intent 取值固定为 NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE，无法判定时回落到 NOTE 并在 errors 中注明。
- corrected 为清洗 + 词典纠错后的文本；无把握修改时保持原文不变，只做空白与全半角等安全清洗。
- changed 表示 corrected 与原文是否有差异。

词典操作写在同一端点上，dict 字段形如：
{"op":"add","term":"工单","alias":["公单","工蛋"]}
{"op":"del","term":"工单"}
{"op":"get","term":"工单"}
增删会同步落盘 dict.jsonl；get 只读不落盘。纠错时按词条与别名匹配，命中即替换，并对高频错形做安全纠错（不跨标点、不改数字与英文大小写）。

## 数据文件（均位于 data-dir，JSONL append-only，不覆写不删行）

- dict.jsonl 词典变更日志：{"op":"add|del","term":"...","alias":[...],"ts":"..."}
- feedback.jsonl 反馈日志：{"trace_id":"...","feedback":"up|down","text":"...","ts":"..."}
- traces.jsonl 处理轨迹：{"trace_id":"...","intent":"...","raw":"...","corrected":"...","changed":true,"ts":"..."}
- usage.jsonl 调用计量：{"trace_id":"...","endpoint":"/v1/process","latency_ms":3,"ts":"..."}

重启后词典与反馈从上述 JSONL 重放恢复，因此追加写即可持久化。

## 自检

curl -s http://127.0.0.1:8080/v1/health

curl -s -X POST http://127.0.0.1:8080/v1/process -H "Authorization: Bearer local-dev" -H "Content-Type: application/json" -d "{\"text\":\"明天九点提醒我开会\"}"