个性化后台实现

运行
go run main.go -addr 127.0.0.1:8080 -data-dir ./data
或
go build -o personalized-server . && ./personalized-server -addr 127.0.0.1:8080 -data-dir ./data

服务只监听 127.0.0.1，非回环地址会拒绝启动。data-dir 不存在时自动创建。

鉴权
默认 -auth-token 为空，仅本机访问。设置后，/v1/process 请求需带：
Authorization: Bearer <token>
/v1/health 可免鉴权用于探活。

端点
GET /v1/health
探活，返回服务状态、data-dir 状态。

POST /v1/process
Content-Type: application/json
请求/响应均为 JSON。支持：
个性化词典：增/删/查条目，含匹配与纠错安全
文本纠错：清洗 + 词典纠错，正常文本不被改坏
意图分类：NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE
反馈学习：✔/✘ 落盘 feedback.jsonl

示例：
curl -s http://127.0.0.1:8080/v1/health
curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -d '{"text":"帮我记一下明天开会","action":"process"}'

数据文件
默认位于 -data-dir 目录，均为 JSONL append-only：
dictionary.jsonl 词典增删事件，启动时重放得到当前词典
feedback.jsonl 反馈学习记录，✔/✘ 追加写入
traces.jsonl 每次 /v1/process 的处理轨迹
usage.jsonl 调用用量统计
不要手工修改；删除/更新以追加事件表达。