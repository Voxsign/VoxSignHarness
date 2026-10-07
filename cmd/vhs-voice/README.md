# vhs-voice — VHS 语音适配层（需求规格 v1）

让「手机端语音 → 后台长程任务」真正跑通。手机端（iOS/豆包模拟器）把语音识别成
文本后，不再直接丢给主 harness（会触发 need_ask 三态拒绝），而是先进本适配层：

```
去噪提意 → 复合指令拆单任务 → 域/对象补全 → 逐个投递主 harness → 语音友好汇总
```

仅标准库、零第三方依赖。

## 启动

```bash
go build -o vhs-voice ./cmd/vhs-voice
VHS_UPSTREAM=http://127.0.0.1:8941 VHS_VOICE_DATA=./voice-data ./vhs-voice
# 默认：端口 8950（VHS_VOICE_ADDR 覆盖）
```

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `VHS_VOICE_ADDR` | `8950` | 监听端口 |
| `VHS_UPSTREAM` | `http://127.0.0.1:8941` | 主 harness 地址（只调其 `POST /v1/tasks` 与 `GET /v1/tasks/{id}`） |
| `VHS_VOICE_DATA` | `./voice-data` | 数据目录（会话审计 JSONL 在 `<dataDir>/voice_sessions/`） |

## 端点

| 端点 | 方法 | 说明 |
|---|---|---|
| `/v1/voice/health` | GET | 健康检查 `{ok,service,upstream,sessions}` |
| `/v1/voice/parse` | POST | 口语去噪提意：`{text}` → `{clean,actions[],noise_removed[]}` |
| `/v1/voice/decompose` | POST | 复合指令拆单任务：`{clean}` → `{tasks[],count}`（无动作→`reason:"无动作"`） |
| `/v1/voice/resolve` | POST | 域/对象补全：explicit > session（最近 3 条）> default（unresolved） |
| `/v1/voice/run` | POST | 编排执行：parse→decompose→resolve→逐个投递上游→轮询→语音友好汇总 |
| `/v1/voice/tasks/{conversation_id}` | GET | 回读会话历史任务（「上次做到哪」） |

## 验收

`docs/VHS-语音适配层-需求规格v1.md` 12 判据全绿（2026-10-04 真跑）：

```
PASS=14 FAIL=0（12 判据 + 2 补充子断言）
```

## 示例

```bash
curl -X POST localhost:8950/v1/voice/run -d '{
  "text": "拉取 GitHub 仓库 VoxSign/voxsign 最新版，跑测试用例",
  "conversation_id": "c2"
}'
```

```json
{"summary":{"done":2,"failed":0,"need_ask":0,"total":2},
 "tasks":[{"seq":1,"action":"拉取","target":"GitHub 仓库 VoxSign/voxsign",
          "task_id":"t-2","status":"done","result":"任务完成"},
          {"seq":2,"action":"跑","target":"测试用例","task_id":"t-3","status":"done",
          "result":"任务完成"}],"next":[]}
```
