个性化后台实现 — 运行说明

一、启动
  go build -o pbd . && ./pbd --addr 127.0.0.1:8080 --data-dir ./data
  或直接：go run . --addr 127.0.0.1:8080 --data-dir ./data

  参数说明
  --addr      监听地址，默认 127.0.0.1:8080；非回环地址（如 0.0.0.0、外部 IP）启动即拒绝
  --data-dir  数据目录，默认 ./data，首次运行自动创建

二、HTTP 端点（均为 JSON 请求/响应）
  GET  /v1/health
       健康检查，返回 {"ok":true,"version":"..."}
       无需鉴权

  POST /v1/process
       请求头：Authorization: Bearer <token>（当前为占位校验，未配置时放行）
       请求体字段：
         text      待处理文本（必填）
         action    correct | intent | dict  （默认 correct）
         feedback  true | false  （可选，回写反馈）
       响应体字段：
         ok        布尔
         text      清洗+纠错后文本
         intent    NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
         corrections  纠错明细列表（原词、替换词、命中词典/规则）
         trace_id  本次请求追踪 ID

三、数据文件（全部位于 --data-dir，JSONL 一律 append-only，不重写不覆盖）
  dictionary.json   个性化词典条目（增/删/查），写入时原子替换
  feedback.jsonl    ✔/✘ 反馈记录，每行一条，只追加
  traces.jsonl      每次 /v1/process 的处理轨迹，只追加
  usage.jsonl       调用量/耗时统计，只追加

四、词典操作
  通过 /v1/process 传 action=dict 完成增/删/查；
  条目字段：term（词条）、replacement（可选纠错替换）、enabled（布尔）。
  纠错仅命中已启用条目，且做边界与安全校验，正常文本原样返回不改坏。

五、退出
  Ctrl+C，进程收到中断信号后刷盘并关闭监听。