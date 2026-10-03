# 个性化后台实现

## 启动
go run . -addr 127.0.0.1:8080 -data-dir ./data -token dev-token

或先编译：
go build -o personal-backend .
./personal-backend -addr 127.0.0.1:8080 -data-dir ./data -token dev-token

参数：
- -addr：默认 127.0.0.1:8080，仅允许回环地址，非 127.0.0.1 启动会拒绝。
- -data-dir：默认 ./data，保存词典、反馈、轨迹、用量等数据。
- -token：占位鉴权令牌，可空；配置后请求需带 Authorization: Bearer <token>。

## HTTP 端点
GET /v1/health
返回：{"ok":true,"service":"personal-backend","time":"..."}

POST /v1/process
请求头：Content-Type: application/json；Authorization: Bearer dev-token（若配置 token）
请求体示例：
{"text":"帮我记一下明天开会","feedback":null,"dict_op":null}

响应体示例：
{"intent":"NOTE","corrected_text":"帮我记一下明天开会","dictionary_hit":false,"trace_id":"...","feedback_saved":false}

能力说明：
- 个性化词典增/删/查：dict_op 支持 add、del、get，例如 {"op":"add","term":"旧词","replacement":"新词"}。
- 文本纠错：先清洗，再按词典安全替换；正常文本不会被改坏。
- 意图分类：NOTE、QUERY、EDIT、COMMIT、ORCHESTRATE 五类。
- 反馈学习：feedback 传 true 或 false，✔/✘ 追加写入 feedback.jsonl。
- 鉴权：-token 可空，但请求路径已预留 Authorization 校验。

## 数据文件
数据目录由 -data-dir 指定，默认 ./data：
- dictionary.json：个性化词典条目。
- feedback.jsonl：反馈学习记录，append-only。
- traces.jsonl：请求处理轨迹，append-only。
- usage.jsonl：调用统计，append-only。
- corrections.jsonl：纠错记录，append-only。

## 快速验证
curl -s http://127.0.0.1:8080/v1/health

curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -H 'Authorization: Bearer dev-token' -d '{"text":"帮我记一下明天开会"}'