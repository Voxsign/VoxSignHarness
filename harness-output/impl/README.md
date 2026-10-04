# 语音适配层 vhs-voice · 运行说明

## 启动

    go build ./...
    go run ./cmd/vhs-voice

或编译后运行：

    ./vhs-voice

可选环境变量：

- VHS_VOICE_ADDR：监听地址，默认 8950（例：127.0.0.1:8950）
- VHS_UPSTREAM：上游主 harness，默认 http://127.0.0.1:8941
- VHS_DATA_DIR：数据目录，默认 ./data

仅监听 127.0.0.1，非回环地址拒绝启动。仅使用标准库，无第三方依赖。

## 端点

适配层自身：

- GET  /v1/voice/health —— 健康检查，返回 ok/service/upstream/sessions
- POST /v1/voice/parse —— 口语去噪提意，入参 {"text":"…"}，出参 clean/actions/noise_removed
- POST /v1/voice/decompose —— 复合指令拆单任务，入参 {"clean":"…"}，出参 tasks[seq/action/target]
- GET  /v1/health —— 进程健康检查
- POST /v1/process —— 通用处理入口（JSON 请求/响应）

上游主 harness（本层调用，不自实现）：

- POST /v1/tasks —— 逐个投递拆解后的单任务
- GET  /v1/tasks/{id} —— 查询任务状态，用于语音友好汇总

## 能力要点

- 个性化词典：增/删/查条目，含匹配与纠错安全
- 文本纠错：清洗 + 词典纠错，正常文本不被改坏
- 意图分类：NOTE / QUERY / EDIT / COMMIT / ORCHESTRATE
- 反馈学习：✔ / ✘ 回馈落盘，append-only
- 全链路 UTF-8 中文，无占位 TODO

## 数据文件

默认在 VHS_DATA_DIR（默认 ./data）下：

- voice_sessions/<conversation_id>.jsonl —— 会话记忆，可审计，append-only
- feedback.jsonl —— 反馈学习记录，append-only
- traces.jsonl —— 调用轨迹，append-only
- usage.jsonl —— 用量统计，append-only

各会话按 conversation_id 分文件，写入均为追加，不覆盖历史。