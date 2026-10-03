个性化后台实现 — 运行说明

一、启动

  默认（监听 127.0.0.1:8080，数据目录 ./data）
    go run main.go

  编译后运行
    go build -o pback . && ./pback

  常用参数 / 环境变量（二者等价，参数优先）
    --addr      监听地址，默认 127.0.0.1:8080
    --data-dir  数据目录，默认 ./data
    --token     占位鉴权令牌，默认空（空则不校验）
    对应环境变量：ADDR / DATA_DIR / TOKEN

  安全约束
    仅接受回环地址；--addr 若为非 127.0.0.1 / ::1 的地址，启动即报错退出。
    设置 --token 后，请求须带 Authorization: Bearer <token>，否则 401。

二、端点

  GET  /v1/health
    返回 200，{"status":"ok","data_dir":"...","dict_size":N}

  POST /v1/process
    请求与响应均为 application/json，统一结构：
    {"action":"...", ...} -> {"ok":true, ...} / {"ok":false, "error":"..."}

    1) 文本处理（清洗 + 词典纠错 + 意图分类）
       {"action":"process","text":"明天三点开会"}
       响应：{"ok":true,"cleaned":"...","corrected":"...","intent":"NOTE",
              "edits":[{"from":"...","to":"..."}],"trace_id":"..."}

    2) 词典增/删/查
       {"action":"dict.add","term":"后台","aliases":["后台系统"]}
       {"action":"dict.del","term":"后台"}
       {"action":"dict.get","term":"后台"}      （查单条）
       {"action":"dict.list","query":"后台"}    （模糊列表，可省 query）

    3) 反馈学习（✔/✘ 落盘，仅追加）
       {"action":"feedback","trace_id":"...","verdict":"up"}    // up=✔ / down=✘
       响应：{"ok":true,"recorded":true}

三、数据文件（全部 append-only JSONL，除词典为整表覆盖）

  data/dictionary.json    个性化词典，增删改后整表覆盖写入
  data/feedback.jsonl     反馈记录，每行一条，只追加
  data/traces.jsonl       每次 /v1/process 的输入输出轨迹，只追加
  data/usage.jsonl        调用计数与耗时，只追加

  说明
    目录不存在时启动自动创建；data-dir 可配，多个实例需使用不同目录。
    JSONL 文件按行独立解析，损坏行跳过并计入 usage.jsonl。
    词典纠错仅在命中词条或高置信模糊匹配时改写，未命中保持原文，
    避免正常文本被改坏。

四、意图类别

  NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
  分类结果在 process 响应的 intent 字段返回；无法判定时返回 NOTE 并标记 low_confidence。