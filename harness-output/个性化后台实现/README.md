# 个性化后台实现（README）

## 一、环境与构建

要求：Go 1.21+（仅标准库，无外部依赖）。

构建命令：
go build -o p13n ./...

如无 go.mod 需先初始化：
go mod init p13n

编译必须通过（go build ./... 退出码为 0），源码中不得残留 todo/占位实现。

## 二、启动命令

默认监听 127.0.0.1:8099，数据目录 ./data：

go run . -addr 127.0.0.1:8099 -data-dir ./data

可选参数：
-addr        监听地址，仅允许回环地址（127.0.0.1 / ::1 / localhost），非回环启动即拒绝
-data-dir    独立数据目录，落盘文件均在该目录下创建（不存在则自动创建）
-token       鉴权占位令牌，默认 dev-token；请求头 X-Auth-Token 或 Authorization: Bearer <token>

示例：
go run . -addr 127.0.0.1:9000 -data-dir ./var -token mytoken

## 三、HTTP 端点

1) 健康检查
GET /v1/health
响应：{"status":"ok","time":"...","data_dir":"..."}

2) 业务处理
POST /v1/process
请求体（JSON）：
{"text":"明天三点提醒我开会","action":"","feedback":"","entry":{}}
字段说明：
- text     待处理文本（必填）
- action   可选：dict_add / dict_del / dict_query / feedback
- entry    词典操作条目 {"term":"...","replacement":"...","intent":"..."}
- feedback 反馈标记："ok" 或 "bad"（对应 ✔/✘），落盘 feedback.jsonl

响应体（JSON）：
{"code":0,"corrected":"...","intent":"NOTE","dictionary_hits":[...],"trace_id":"..."}

意图取值固定为五类：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE

3) 词典管理（可走同一处理器）
POST /v1/process，action 为：
- dict_add   增条目，响应含 {"ok":true}
- dict_del   删条目
- dict_query 查条目，支持前缀/包含匹配

鉴权：所有 /v1/*（除 /v1/health）需携带令牌，缺失或错误返回 401。

## 四、P0 能力对应

① 个性化词典：add/del/query 三操作，词条做全词/边界匹配，避免子串误替换；纠错仅替换命中且安全的片段
② 文本纠错：先清洗（去多余空白、统一全半角、去控制字符），再按词典纠错；未命中的正常文本原样返回，不做破坏性改写
③ 意图分类：关键词+规则打分输出 NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE
④ 反馈学习：feedback=ok/bad 追加写入 feedback.jsonl（append-only），同时影响词条权重
⑤ 数据落盘：全部 JSONL append-only 写文件，每次写入 O_APPEND，不覆盖历史
⑥ HTTP 端点：/v1/health、/v1/process
⑦ 仅监听 127.0.0.1，非回环拒绝；鉴权占位但强制校验

## 五、数据文件（均在 -data-dir 下）

data/dictionary.jsonl   词典条目，append-only，增删以事件形式记录，启动时重放
data/feedback.jsonl     反馈回执，append-only
data/traces.jsonl       每次 /v1/process 的请求-响应追踪
data/usage.jsonl        调用量与耗时统计

每条记录均为单行 JSON，末尾换行，字段首列带 ts（RFC3339 纳秒）与 type。

## 六、快速自检

启动后执行：
curl -s http://127.0.0.1:8099/v1/health
curl -s -X POST http://127.0.0.1:8099/v1/process -H "X-Auth-Token: dev-token" -H "Content-Type: application/json" -d "{\"text\":\"明天三点提醒我开会\"}"
curl -s -X POST http://127.0.0.1:8099/v1/process -H "X-Auth-Token: dev-token" -H "Content-Type: application/json" -d "{\"action\":\"dict_add\",\"entry\":{\"term\":\"回意\",\"replacement\":\"回忆\"}}"

预期：健康检查返回 status=ok；process 返回 corrected 与 intent；相应 JSONL 文件各新增一行。