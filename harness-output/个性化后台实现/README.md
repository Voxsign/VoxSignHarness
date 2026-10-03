个性化后台运行说明

一、环境与构建
需求：Go 1.21 及以上。
构建：go build -o pbackend ./...
若上一轮 main.go 仍有 todo 占位，请先替换为完整实现再构建，否则编译会失败（exit status 1）。

二、启动
默认（监听 127.0.0.1:8080，数据目录 ./data）：
  ./pbackend
自定义端口与数据目录：
  ./pbackend -addr 127.0.0.1:9090 -data-dir /var/lib/pbackend
可选鉴权占位：设环境变量 PB_TOKEN=xxxx，请求头带 Authorization: Bearer xxxx。
注意：仅接受回环地址，绑定到 0.0.0.0 或其他非 127.0.0.1 地址时进程直接拒绝启动。

三、端点
GET  /v1/health
  返回 {"status":"ok","data_dir":"...","version":"..."}
POST /v1/process
  请求体 JSON：{"text":"...","action":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE","feedback":null}
  响应体 JSON：{"intent":"...","corrected":"...","matched":["..."],"trace_id":"...","ok":true}
  说明：text 先做清洗与词典纠错，再做五类意图分类；feedback 传 true/false 时写入反馈日志。

四、命令行子功能（可选）
  ./pbackend dict add <词> <释义>
  ./pbackend dict del <词>
  ./pbackend dict get <词>
  词典文件位于 <data-dir>/dictionary.json，匹配与纠错均做安全约束，正常文本不会被改写。

五、数据文件（均为 append-only JSONL，位于 data-dir）
  traces.jsonl    每次 /v1/process 的处理轨迹
  usage.jsonl     调用统计（时间、意图、耗时）
  feedback.jsonl  ✔/✘ 反馈记录
  dictionary.json 个性化词典条目

六、验证
  curl http://127.0.0.1:8080/v1/health
  curl -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' \
       -d '{"text":"记一下明天开会","action":"NOTE"}'