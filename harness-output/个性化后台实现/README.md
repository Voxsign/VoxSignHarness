个性化后台 运行说明

一、环境要求
Go 1.21+，无需外部依赖（标准库实现）。

二、启动
默认启动（监听 127.0.0.1:8787，数据目录 ./data）：
go run .

指定端口与数据目录：
go run . -addr 127.0.0.1:9000 -data-dir ./data

编译后运行：
go build -o pbd . && ./pbd -addr 127.0.0.1:8787 -data-dir ./data

安全约束：仅接受回环地址；-addr 绑定非 127.0.0.1 时进程直接退出。
鉴权：请求头 Authorization: Bearer <token>，token 由 -token 指定（默认 dev-token，占位实现，可通过 -no-auth 关闭）。

三、HTTP 端点
GET  /v1/health
     返回 {"status":"ok","time":...,"data_dir":...,"version":...}

POST /v1/process
     请求 JSON：
     {
       "action": "note|query|edit|commit|orchestrate|dict.add|dict.del|dict.list|feedback",
       "text":   "原始文本",
       "id":     "词典条目 id（dict.del 用）",
       "word":   "词条（dict.add 用）",
       "hit":    true/false（feedback 用）
     }
     响应 JSON：
     {
       "ok": true,
       "intent": "NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE",
       "corrected": "纠错后文本",
       "changes": [{"from":"...","to":"...","reason":"dict|clean"}],
       "dict": [...],        // dict.list 时返回
       "trace_id": "...",
       "error": ""           // 失败时填写
     }

四、P0 能力对应
①个性化词典：dict.add / dict.del / dict.list，词条持久化，匹配按最长优先，纠错只做安全替换（长度相近、非数字/URL/代码块内不替换）。
②文本纠错：先清洗（空白归一、全角半角、连续标点），再词典纠错；无命中时原文返回，保证正常文本不被改坏。
③意图分类：NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE 五类，规则+词典关键词打分，返回置信度最高者，兜底 NOTE。
④反馈学习：feedback 写入 feedback.jsonl，append-only，命中词条权重微调，权重落盘 dictionary.json。
⑤数据落盘：全部 JSONL append-only，分别写入独立数据目录。
⑥端点：/v1/health、/v1/process，JSON 进 JSON 出。
⑦仅监听 127.0.0.1，非回环拒绝；Authorization 占位校验。

五、数据文件（-data-dir 下）
dictionary.json   个性化词典（唯一可覆盖写的文件，原子替换）
traces.jsonl      每次 /v1/process 的请求/响应轨迹，一行一条，append-only
usage.jsonl       端点调用计数与耗时，append-only
feedback.jsonl    ✔/✘ 反馈记录，append-only

启动后目录不存在会自动创建。JSONL 只追加，不重写、不轮转。

六、快速自检
curl -s http://127.0.0.1:8787/v1/health
curl -s -X POST http://127.0.0.1:8787/v1/process \
  -H 'Authorization: Bearer dev-token' -H 'Content-Type: application/json' \
  -d '{"action":"dict.add","word":"个性化后台"}'
curl -s -X POST http://127.0.0.1:8787/v1/process \
  -H 'Content-Type: application/json' -d '{"action":"note","text":"帮我记录一下个性化后台 的实现"}'
tail -n 3 ./data/traces.jsonl

七、测试
go test ./...
go vet ./...