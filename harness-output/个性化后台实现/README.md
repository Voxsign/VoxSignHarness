README.md — 个性化后台（P0）

概述
个性化文本处理服务：个性化词典 + 文本纠错 + 五类意图分类 + 反馈学习 + JSONL 落盘，仅监听 127.0.0.1。

启动
1) 编译：go build -o pb ./cmd/pb     （或 go build -o pb .）
2) 运行：./pb -addr 127.0.0.1:8080 -data-dir ./data -token dev-token
   参数说明：
   -addr      监听地址，仅允许回环（127.0.0.1 / ::1 / localhost），非回环直接拒绝启动
   -data-dir  数据目录，默认 ./data，可配
   -token     鉴权占位 token，通过请求头 Authorization: Bearer <token> 校验
3) 后台常驻（可选）：nohup ./pb -data-dir /var/lib/pb > pb.log 2>&1 &

HTTP 端点
GET  /v1/health
     健康检查。返回 {"status":"ok","version":"...","data_dir":"..."}
POST /v1/process
     业务入口。请求 JSON：
     {
       "text": "待处理文本",
       "intent": "可选，指定则跳过分类",
       "feedback": "可选，ok | not_ok，用于反馈学习",
       "request_id": "可选"
     }
     响应 JSON：
     {
       "request_id": "...",
       "cleaned": "清洗后文本",
       "corrected": "词典纠错后文本",
       "intent": "NOTE | QUERY | EDIT | COMMIT | ORCHESTRATE",
       "confidence": 0.0,
       "dict_hits": [{"term":"...","action":"replace|keep","reason":"..."}],
       "changed": true
     }
词典管理端点（增/删/查）
GET    /v1/dict            查询全部条目
GET    /v1/dict?q=关键词   按关键词检索
POST   /v1/dict            新增条目 {"term":"...","canonical":"...","note":"..."}
DELETE /v1/dict?term=...   删除条目
纠错安全约定：仅当词典命中且替换不改变实体/否定/数字/时间语义时替换；未命中一律原文返回，正常文本不被改坏。

数据文件（全部 JSONL，append-only，位于 -data-dir 目录）
data/dictionary.json    个性化词典（增删查后即时落盘；JSONL 快照 + 操作追加）
data/dict_ops.jsonl     词典增删操作流水（append-only）
data/traces.jsonl       每次 /v1/process 的输入/输出轨迹（append-only）
data/usage.jsonl        调用用量与耗时统计（append-only）
data/feedback.jsonl     反馈学习记录，✔/✘ 回馈（append-only）

intent 五类
NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE

安全与鉴权
仅监听 127.0.0.1（非回环地址拒绝启动）；除 /v1/health 外，所有端点需 Bearer token（可占位，但必须校验，未通过返回 401）。

快速自检
curl -s http://127.0.0.1:8080/v1/health
curl -s -H "Authorization: Bearer dev-token" -H "Content-Type: application/json" \
  -d '{"text":"记一下，明天交周报"}' http://127.0.0.1:8080/v1/process

目录约定
cmd/pb/main.go     入口与路由
internal/dict      词典（增删查）
internal/correct   清洗与纠错
internal/intent    意图分类
internal/feedback  反馈学习
internal/store     JSONL append-only 落盘
data/              默认数据目录