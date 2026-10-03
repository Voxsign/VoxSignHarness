# 个性化后台实现 —— 运行说明

## 启动

go run . --addr 127.0.0.1:8080 --data-dir ./data

或先编译再运行：

go build -o personald . && ./personald --addr 127.0.0.1:8080 --data-dir ./data

参数说明：
--addr      监听地址，默认 127.0.0.1:8080。仅允许回环地址，传入非 127.0.0.1/::1 时启动直接报错退出。
--data-dir  数据目录，默认 ./data。启动时自动创建，所有落盘文件都在此目录下，可整体替换/备份。
--token     鉴权占位令牌，默认空。为空时不校验；非空时请求需带 `Authorization: Bearer <token>`。

## 端点

GET  /v1/health
     健康检查。返回 200 与 {"status":"ok","data_dir":"...","version":"..."}。

POST /v1/process
     业务入口，JSON 请求 / JSON 响应。请求字段：
       text      待处理文本（必填）
       op        可选操作：dict_add / dict_del / dict_get / correct / intent / feedback
       entry     词典条目，dict_add / dict_del 时使用
       verdict   反馈结果，feedback 时取 "up" 或 "down"
     不传 op 时按流水线执行：清洗 -> 词典纠错 -> 意图分类，返回纠正后文本、是否被改动、命中词条、意图类别。
     意图类别固定五类：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE。

## 数据文件（均在 --data-dir 下）

dictionary.json   个性化词典，含条目与增删改时间戳。启动时加载，写入后立即回写。
feedback.jsonl    反馈学习记录，append-only，每行一条 {ts, text, corrected, verdict}。
traces.jsonl      每次 /v1/process 调用的输入输出轨迹，append-only。
usage.jsonl       调用计数与耗时统计，append-only。

以上三个 JSONL 只追加、不覆写，可直接做增量采集。

## 行为约束

纠错只替换词典中明确命中的错误词形，未命中或替换后语义风险高的文本原样返回，正常文本不被改坏。
文本清洗仅做空白归一、全半角与不可见字符处理，不改动有效内容。
所有落盘为同步追加写入，进程异常退出不丢已确认记录。

## 最小验证

curl http://127.0.0.1:8080/v1/health

curl -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -d '{"text":"帮我记一下明天的会","op":"intent"}'

返回中 intent 应为 NOTE；随后查看 data/traces.jsonl 与 data/usage.jsonl 是否新增行。