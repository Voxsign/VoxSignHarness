个性化后台运行说明
====================

一、启动
  go build -o p13n .        # 编译，须退出码 0
  ./p13n --data-dir=./data  # 默认监听 127.0.0.1:8080
  可选参数：--addr=127.0.0.1:8080  --data-dir=./data  --token=（占位鉴权）

  监听地址固定只接受回环地址，传入非 127.0.0.1/::1 时启动即失败退出。
  所有 /v1/* 端点须带 Authorization: Bearer <token>，token 为空时放行但仍校验请求头格式。

二、端点
  GET  /v1/health
       返回 {"ok":true,"version":...,"data_dir":...,"dict_size":N}
   POST /v1/process
       请求 {"text":"...","op":"AUTO","feedback":null,"id":"可选"}
       op 取值：AUTO/NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE
       返回 {"intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE",
             "corrected":"纠错后文本","dict_hits":[...],
             "changed":true|false,"trace_id":"..."}
   POST /v1/dict/add   {"term":"...","aliases":["..."]}
   POST /v1/dict/del   {"term":"..."}
   GET  /v1/dict/get?term=...
   POST /v1/feedback   {"id":"...","hit":true|false,"term":"..."}
       落盘 feedback.jsonl，供后续加权/纠错安全阈值调整。

三、行为约束
  纠错只做词典命中与别名替换，命中不确定时不改动原文，正常文本原样返回。
  意图分类基于关键词+规则打分，五类全覆盖，无法判定归 ORCHESTRATE 之外的默认类需显式给出。
  反馈学习仅 append 记录，不重写历史文件。

四、数据文件（均在 --data-dir 下，JSONL/JSON 均为 append-only 或原子替换）
  data/dictionary.json   词典条目（原子替换写入）
  data/feedback.jsonl    反馈回馈，逐行 append
  data/traces.jsonl      每次 /v1/process 调用轨迹，逐行 append
  data/usage.jsonl       端点调用量与耗时，逐行 append
  目录不存在时自动创建；文件以 O_APPEND 打开，进程重启不截断。

五、自检
  go vet ./... && go test ./...    # 须全部通过
  启动后 curl -H "Authorization: Bearer dev" http://127.0.0.1:8080/v1/health 返回 ok:true。