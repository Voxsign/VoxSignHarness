# VHS 语音适配层 · 需求规格 v1（2026-10-04）

> 目标：让「手机端语音 → 后台长程任务」真正跑通。手机端（iOS/豆包模拟器）把语音识别成文本后，不再直接丢给主 harness（会触发 need_ask 三态拒绝），而是先进本适配层：**去噪提意 → 复合指令拆单任务 → 域/对象补全 → 逐个投递主 harness → 语音友好汇总**。

## 1. 服务形态（技术约束）

- 独立 Go 服务：`cmd/vhs-voice`（仅标准库，零第三方依赖，可 `go build ./...`）
- 监听端口默认 `8950`（环境变量 `VHS_VOICE_ADDR` 可覆盖）
- 上游主 harness：`http://127.0.0.1:8941`（环境变量 `VHS_UPSTREAM` 可覆盖），适配层只调其 `POST /v1/tasks` 与 `GET /v1/tasks/{id}`
- 会话记忆：本地 JSONL 文件 `<dataDir>/voice_sessions/<conversation_id>.jsonl`，可审计
- 无第三方依赖、无 TODO 占位、UTF-8 中文全链路不乱码

## 2. 端点契约（验收判据，逐条可测）

### E1. GET /v1/voice/health
返回 `{"ok":true,"service":"vhs-voice","upstream":"<上游地址>","sessions":<会话数>}`

### E2. POST /v1/voice/parse —— 口语去噪提意
入参：`{"text":"<口语原文>"}`
出参：`{"clean":"<去噪后文本>","actions":[{"action":"<动作>","target":"<对象>"}],"noise_removed":["<被删的填充词>"]}`
规则：
- 删除填充词/语气词：就是、那个、对吧、好不好、好吧、怎么样、然后、现在、开始、准备、假设、其实、比如、我觉得、你知道、大概、应该、可以（出现即删，不改变剩余语义）
- 提取动作：执行/跑/测试/拉取/编译/启动/实现/提交/报告/检查/对比/分析/部署/安装/更新（动作优先匹配，取首词）
- 提取对象：`GitHub 仓库 <仓库>`、`服务 <名>`、`需求 <名>`、`产物 <名>`、`测试 <名>`（无对象则 target 为空，交给 resolve）

### E3. POST /v1/voice/decompose —— 复合指令拆单任务
入参：`{"clean":"<去噪后文本>"}`
出参：`{"tasks":[{"seq":1,"action":"拉取","target":"GitHub 仓库 smithpeter/voicesign-harness"},...],"count":N}`
规则：
- 按动作词切分：一个动作词（或"动作+对象"短语）＝一个任务；连续动作词相邻合并为「动作序列」时逐个拆开
- 示例：「拉取最新版，编译并启动服务，跑长程任务验收测试，输出报告」→ 4 个任务
- 无动作词时：视为 ASK（返回 `{"tasks":[],"reason":"无动作"}`）

### E4. POST /v1/voice/resolve —— 域/对象补全
入参：`{"text":"<clean>","conversation_id":"<会话>","actions":[{"action":"...","target":""}]}`
出参：`{"resolved":[{"action":"...","target":"<补全后的对象>","source":"explicit|session|default"}],"context":{}}`
规则（优先级由高到低）：
1. explicit：入参已带对象
2. session：读 `<dataDir>/voice_sessions/<conversation_id>.jsonl` 最近 3 条，命中"仓库/服务/需求/产物/测试"类名词则补全（例如历史说过 `smithpeter/voicesign-harness`，本次说「拉最新版」→ 补全为「GitHub 仓库 smithpeter/voicesign-harness」）
3. default：仍空 → target 标注 `"unresolved"`，任务标记 `pending_resolve`

### E5. POST /v1/voice/run —— 编排执行（核心）
入参：`{"text":"<口语原文>","conversation_id":"<会话>","document":"<需求文档全文，可空>"}`
流程（必须可观测、可审计）：
1. `parse` → 去噪 + 动作清单
2. `decompose` → 单任务序列
3. `resolve` → 逐个补全对象
4. **逐个投递**主 harness：`POST /v1/tasks`（text=`<动作> <对象>`、conversation_id=同会话、document=入参 document），然后轮询 `GET /v1/tasks/{id}` 至 done/need_ask/failed
5. 每个任务的轨迹追加写 `<dataDir>/voice_sessions/<conversation_id>.jsonl`
出参：`{"summary":{"total":N,"done":x,"need_ask":y,"failed":z},"tasks":[{"seq":1,"action":"...","target":"...","task_id":"...","status":"done|need_ask|failed|pending_resolve","result":"<简短结果>"}],"next":["<未完成任务的下一步建议>"]}`
- 出参必须**语音友好**：每个任务一行短句（动作+对象+状态），不做长篇。

### E6. GET /v1/voice/tasks/{conversation_id}
返回该会话的历史任务序列：`{"conversation_id":"...","tasks":[...],"count":N}`（供语音端回读「上次做到哪」）

## 3. 判定与验收

| 判据 | 端点 | 可测标准 |
|---|---|---|
| 1 | E1 health | 200 且 ok=true |
| 2 | E2 parse | 「就是那个现在开始跑一下测试对吧」→ actions=[跑,测试]、noise_removed 含「就是/那个/现在/开始/对吧」 |
| 3 | E2 parse 语义保全 | 去噪后不含被删填充词、剩余文本语义可读 |
| 4 | E3 decompose | 「拉取最新版，编译并启动服务，跑长程任务验收测试，输出报告」→ 4 个任务 |
| 5 | E3 无动作 | 「你好呀」→ tasks=[] 且 reason=无动作 |
| 6 | E4 resolve session | 会话历史有仓库→「拉最新版」补全为 smithpeter/voicesign-harness（source=session） |
| 7 | E4 resolve 空 | 无历史且无对象 → target=unresolved、任务标记 pending_resolve |
| 8 | E5 run 编排 | 2 个任务的输入 → summary.total=2、逐个投递上游、每个任务 status 与上游一致 |
| 9 | E5 run 可审计 | 会话 JSONL 有全部任务轨迹（动作/对象/task_id/status/ts） |
| 10 | E5 语音友好 | 出参 tasks 每项一行短句（≤120 字），无长段落 |
| 11 | E6 回读 | GET /v1/voice/tasks/{cid} 返回 count=已执行任务数 |
| 12 | 技术约束 | go build 通过、仅标准库、中文不乱码 |

## 4. 交付形态

- 源码：`cmd/vhs-voice/`（main.go 单文件或同目录小包）
- 文档：README 说明启动方式（VHS_VOICE_ADDR/VHS_UPSTREAM/dataDir 环境变量）
- 验收：评审按上表 12 判据逐条真跑取证
