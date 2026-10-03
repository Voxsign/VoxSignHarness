个性化后台实现 —— 运行说明

一、构建与启动

  依赖：Go 1.21+，标准库即可，无外部依赖。

  构建：
    go build -o personald .

  启动（默认参数）：
    ./personald

  常用参数：
    --addr       监听地址，默认 127.0.0.1:8080（非回环地址直接拒绝启动）
    --data-dir   数据目录，默认 ./data（启动时自动创建）
    --token      占位鉴权令牌，默认 dev-token；为空表示不校验

  指定端口与数据目录：
    ./personald --addr 127.0.0.1:9090 --data-dir /var/lib/personald

  开发期直接跑：
    go run . --data-dir ./data

二、鉴权

  除 /v1/health 外，请求需带：
    Authorization: Bearer <token>
  未携带或令牌不符返回 401。当前为占位实现，未接入真实身份体系。

三、HTTP 端点

  GET /v1/health
    健康检查。返回 {"status":"ok","data_dir":"...","uptime_s":N}

  POST /v1/process
    统一业务入口，请求体 JSON，响应体 JSON。

    请求字段：
      text      待处理文本（可选）
      feedback  true/false，对本轮结果点赞或点踩（可选）
      dict_op   词典操作（可选），形如
                {"op":"add","term":"...","replacement":"...","weight":1}
                {"op":"del","term":"..."}
                {"op":"get","term":"..."}

    响应字段：
      intent     NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE 之一
      corrected  清洗与词典纠错后的文本（原文未被改坏时与 text 一致）
      dict       词典查询结果（dict_op 为 get 时返回）
      trace_id   本轮追踪号，用于关联 traces.jsonl
      ok         处理是否成功

    示例：
      curl -s -X POST http://127.0.0.1:8080/v1/process \
        -H 'Authorization: Bearer dev-token' \
        -H 'Content-Type: application/json' \
        -d '{"text":"帮我记一下明天开会"}'

四、数据文件

  全部位于 --data-dir 指向的目录，JSONL 文件均为 append-only，进程只追加不覆写：

    traces.jsonl    每轮处理的入参、纠错结果、意图、耗时、trace_id
    usage.jsonl     端点的调用记录（时间、路径、状态码）
    feedback.jsonl  ✔/✘ 反馈落盘（trace_id、判定、原始文本、时间）
    dictionary.json 个性化词典条目（增删改后整体重写，属唯一非 append 文件）

  查看最近记录：
    tail -n 20 ./data/traces.jsonl

五、行为约束

  1. 纠错只做清洗（空白、全半角、连续标点）与词典命中替换，无匹配时原样返回，保证正常文本不被改坏。
  2. 意图分类断言五类之一，无法判定时归为 QUERY。
  3. 词典删除后立即生效，纠错链路不再命中该条目。
  4. 监听地址非 127.0.0.1 / ::1 时启动报错退出。

六、自检

  go build ./...   编译应无错误、无 todo 占位
  go vet ./...     静态检查
  启动后 curl /v1/health 返回 200 即视为就绪