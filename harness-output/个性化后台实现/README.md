README.md

个性化后台（Go 单二进制）

一、编译
  go build -o personald .
  （要求 Go 1.21+；无第三方依赖，仅标准库）

二、启动
  ./personald -addr 127.0.0.1:8787 -data-dir ./data
  参数说明：
    -addr      监听地址，默认 127.0.0.1:8787；非回环地址（如 0.0.0.0、局域网 IP）启动时直接拒绝并退出。
    -data-dir  数据目录，默认 ./data；首次运行自动创建。
    -token     可选占位鉴权串，默认空；非空时要求请求头 Authorization: Bearer <token>。

  第一次启动会在 data-dir 下生成字典文件 dictionary.json（不存在则写入空词典骨架）。

三、HTTP 端点（全部 JSON 请求/响应）
  1) GET /v1/health
     返回：{"ok":true,"status":"healthy","data_dir":"...","dictionary_size":N}

  2) POST /v1/process
     请求：{"text":"...","intent_hint":"","feedback":""}
     响应：{"intent":"NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE","clean_text":"...","corrected_text":"...","corrections":[...],"dict_hits":[...],"trace_id":"..."}
     说明：
       - text 为空或纯空白返回 400。
       - 正常文本经过清洗与词典纠错后必须保持不变（不误改）。
       - 纠错只采用词典中的高置信条目，并按安全规则（长度、非子串误伤）生效。
       - feedback 字段为可选，取值为 "yes"/"no"（或 ✔/✘），等价于调用一次 /v1/feedback。

  3) POST /v1/dict  （增）
     请求：{"term":"错误词","replacement":"正确词"}
     响应：{"ok":true,"size":N}

  4) GET /v1/dict?term=xxx  （查；term 省略则返回全部条目）
     响应：{"ok":true,"entries":[{"term":"...","replacement":"...","hits":N}]}

  5) DELETE /v1/dict?term=xxx  （删）
     响应：{"ok":true,"removed":true,"size":N}

  6) POST /v1/feedback
     请求：{"trace_id":"...","verdict":"yes|no","note":"可选"}
     响应：{"ok":true}

四、数据文件（全部 append-only JSONL，位于 data-dir）
  data/dictionary.json   个性化词典，整体原子重写（增删改）
  data/feedback.jsonl    反馈学习记录，每行 {"ts":...,"trace_id":...,"verdict":...,"note":...}
  data/traces.jsonl      每次 /v1/process 的调用轨迹，每行一条
  data/usage.jsonl       用量统计，每行 {"ts":...,"endpoint":...,"intent":...,"latency_ms":...}

  落盘约定：所有 .jsonl 仅追加、不重写、不删除；写入失败不影响接口返回，但会写入 stderr 日志。

五、意图分类规则（NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE）
  按关键词与句式打分，取最高分，无命中时默认 NOTE。

六、快速自检
  curl -s http://127.0.0.1:8787/v1/health
  curl -s -X POST http://127.0.0.1:8787/v1/dict -d '{"term":"登陆","replacement":"登录"}'
  curl -s -X POST http://127.0.0.1:8787/v1/process -d '{"text":"我要登陆系统"}'
  curl -s -X POST http://127.0.0.1:8787/v1/feedback -d '{"trace_id":"<上一步返回的 trace_id>","verdict":"yes"}'
  tail -n 5 ./data/traces.jsonl