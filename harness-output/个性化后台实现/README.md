个性化后台 README（运行说明）

一、环境与编译
需要 Go 1.21 及以上版本。
编译：go build -o pserver ./...
（若 main.go 有未使用变量或未定义符号会直接编译失败，请先修复后再构建。）

二、启动命令
默认启动（监听 127.0.0.1:8080，数据目录 ./data，鉴权占位）：
  ./pserver

常用参数：
  -addr 127.0.0.1:8080   监听地址，非回环地址会被拒绝启动
  -data-dir ./data       数据目录，启动时自动创建
  -token dev-token       占位鉴权令牌，请求头 X-Auth-Token 需匹配

示例：
  ./pserver -addr 127.0.0.1:9090 -data-dir /var/lib/personalizer -token secret

三、HTTP 端点
1) GET /v1/health
   返回 {"status":"ok","data_dir":"...","uptime_s":N}

2) POST /v1/process
   请求头：Content-Type: application/json，X-Auth-Token: <token>
   请求体字段：
     text      string  必填，待处理文本
     action    string  可选，note/query/edit/commit/orchestrate 显式指定意图
     feedback  string  可选，up/down（或 ✔/✘）写入反馈
  响应体字段：
     cleaned   清洗后文本
     corrected 词典纠错后文本
     intent    五类之一：NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE
     trace_id  本次追踪 ID

四、数据文件（全部 JSONL，append-only，位于 -data-dir 下）
  dictionary.json   个性化词典条目（增/删/查持久化）
  traces.jsonl      每次 /v1/process 的请求与结果追踪
  usage.jsonl       调用计数与耗时统计
  feedback.jsonl    ✔/✘ 反馈记录，只追加不覆盖

五、注意事项
服务仅绑定 127.0.0.1 等回环地址，其他地址拒绝启动。
鉴权当前为占位实现，生产环境请替换为真实校验逻辑。
所有落盘均为追加写，请勿手工改写历史文件。