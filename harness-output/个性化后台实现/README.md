# 个性化后台实现 · 运行说明

## 一、环境与构建
需要 Go 1.21 及以上。

编译：
go build -o pbackend .

启动（默认监听 127.0.0.1:8080，数据目录 ./data）：
./pbackend

自定义地址与数据目录：
./pbackend -addr 127.0.0.1:9000 -data-dir /var/lib/pbackend

说明：服务只接受回环地址。若 -addr 填写的 IP 非 127.0.0.0/8 或 ::1，启动时会直接拒绝并退出。鉴权为占位实现，可通过 -token 设置；请求头带 `Authorization: Bearer <token>` 时校验，未设置时不校验。

## 二、HTTP 端点

1) 健康检查
GET /v1/health
返回：{"status":"ok","version":"...","data_dir":"..."}

2) 业务处理
POST /v1/process
请求体（JSON）：
{
  "text": "原始文本",
  "session_id": "可选",
  "op": "可选，见下",
  "feedback": {"trace_id": "可选", "label": "up|down"}
}

op 取值：
- 空或 "process"：文本纠错 + 意图分类（默认）
- "dict.add"：新增词条，配合 "term"、"replacement"、"note"
- "dict.del"：删除词条，配合 "term"
- "dict.list"：列出词条

响应体（JSON）：
{
  "trace_id": "本次请求 ID",
  "intent": "NOTE|QUERY|EDIT|COMMIT|ORCHESTRATE",
  "text": "清洗与词典纠错后的文本",
  "original": "原始文本",
  "corrections": [{"from": "...", "to": "...", "term": "..."}],
  "dict": [ ... ] // 仅 dict.list 返回
}

文本纠错遵循安全原则：仅按词典精确/近似命中替换，命中不确定或会破坏语义时保持原文不变，corrections 为空即表示未做任何修改。

意图分类五类：NOTE（记录）、QUERY（查询）、EDIT（修改）、COMMIT（提交/确认）、ORCHESTRATE（编排/多步调度）；无法判定时归为 NOTE。

## 三、数据文件（均在 data-dir 下，JSONL 一律 append-only）
- data/dictionary.json：个性化词典条目，新增/删除通过临时文件+rename 原子替换
- data/traces/traces.jsonl：每次 /v1/process 的输入、纠错结果、意图、耗时
- data/usage/usage.jsonl：调用计数与端点用量
- data/feedback.jsonl：✔/✘ 反馈记录，只追加不覆盖

JSONL 每行一个独立 JSON 对象，写入为 O_APPEND 追加，进程重启后继续追加，不做重写。

## 四、反馈学习
在 /v1/process 请求中携带 feedback 字段即可落盘 feedback.jsonl；已落盘的反馈会在后续纠错与意图判定中被读取，用于调整词条权重。反馈文件损坏的行会被跳过，不影响启动。

## 五、快速自测
curl http://127.0.0.1:8080/v1/health
curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -d '{"text":"明天要开会","session_id":"s1"}'
curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -d '{"op":"dict.add","term":"开会","replacement":"评审会"}'
curl -s -X POST http://127.0.0.1:8080/v1/process -H 'Content-Type: application/json' -d '{"text":"明天要开会","feedback":{"trace_id":"上一步返回的 trace_id","label":"up"}}'