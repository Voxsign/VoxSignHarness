个性化后台实现 · 运行说明

一、启动
  go build -o pbackend main.go
  ./pbackend --addr 127.0.0.1:8080 --data-dir ./data
  或直接运行：go run main.go --addr 127.0.0.1:8080 --data-dir ./data
  服务仅监听 127.0.0.1，非回环地址将被拒绝启动；--data-dir 可配置，缺省 ./data，不存在时自动创建。

二、端点
  GET  /v1/health
       返回 {"status":"ok","uptime_ms":...,"data_dir":"..."}，用于存活探测。
  POST /v1/process
       请求 JSON：{"text":"...","action":"process|dict_add|dict_del|dict_get|feedback","user":"...","key":"...","value":"..."}
       响应 JSON：{"ok":true,"intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE","corrected":"...","changed":false,"matches":[...],"trace_id":"..."}
       action=process 走清洗 + 词典纠错 + 意图分类；dict_add/dict_del/dict_get 管理个性化词典；
       action=feedback 携带 {"trace_id":"...","vote":"up|down"} 落盘反馈。
  鉴权：请求头 Authorization: Bearer <token>；当前为占位实现，token 为空时放行，但头部缺失会被记录告警。

三、数据文件（全部 append-only JSONL，位于 --data-dir 下）
  dictionary.jsonl   个性化词典条目：{"op":"add|del","key":...,"value":...,"ts":...}
  feedback.jsonl     反馈：{"trace_id":...,"vote":"up|down","ts":...}
  traces.jsonl       每次 /v1/process 的输入、纠错结果、意图与命中词：{"trace_id":...,"text":...,"corrected":...,"intent":...,"ts":...}
  usage.jsonl        调用计量：{"endpoint":...,"status":...,"latency_ms":...,"ts":...}

四、行为约定
  纠错只做安全替换：命中词典才替换，未命中的原文一字不动，正常文本不会被改坏。
  意图分类固定五类 NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE，无法判定时回落到 NOTE。
  所有写盘均为追加，不覆盖、不重写历史文件；读取时按时间顺序回放得到当前状态。