# VoxSign Server 后端设计（v1 · 实测锚定版）

- 日期：2026-10-05 · 执行机：Peter Mac · 执行者：Claude Code（Sonnet 5.5，`claude` v2.1.284）
- 依据：任务书 `VoxSign-Server-设计-任务书.md` v2；`~/voicesign-harness`（commit 4186747，GitHub 私有库 smithpeter/voicesign-harness）；`~/.voicesign/harness`；本机实测（证据编号 E1–E17、E19（E18 未使用），见 §2）
- 品牌：VoxSign / voxsign.ai
- 状态：设计稿，**未经评审**；「设计」与「已实现」严格区分，凡写「现状」处均带出处，无出处者标「新建」。

> 阅读约定：`[E#]` = 本机实测证据（§2）；`[代码 path:line]` = 代码出处；`[文档 …]` = docs 出处。

---

## 0. 盘点说明与已知缺口（先说清楚）

| 项 | 实际情况 |
|---|---|
| `~/voicesign-harness/README.md` | **不存在**（根目录无 README*）。以 `ARCHITECTURE.md`（V0 评审稿，2026-10-01）+ `docs/` 代替 |
| `~/voxsign/` | 仅含 `_archive/`（docs_v1、src_v1、openclaw_io、tests_v1…旧归档）及若干 `<MagicMock …>-shm/-wal` 测试残留文件；**无现行代码**，不作为设计依据 |
| `docs/` | 共 100+ 篇。精读：详细设计 v2 定稿（章节 0–16）、产品定位与云架构理念、云端谷歌登录与发布架构（HLD）、AIOps 接口测试报告；`新方案-Claude定稿` 仅读章节结构。其余未逐篇读 |
| `~/.voicesign/harness` | `notes.md`（NOTE 实测堆积，无设计价值）、`trajectory-20261003.jsonl`（525KB，362 条请求，87 条有回执）、`trajectory-20261005.jsonl`、`decisions.jsonl`（92 行） |
| 工作区状态 | harness 仓 `git status` 有 3 处未提交改动（`harness-output/impl/*`，mtime 10-04 23:51，**非本次产生**）；本次实测全部在 `/tmp/vxs` 沙箱（`VHS_LOG_DIR=/tmp/vxs/log`、临时 git 仓），未写 `~/.voicesign/harness` |
| 工具连接 | MCP `voxsign`（CONNECTION_CLOSED）、`memory`（ECONNREFUSED）、`voicemode` 本会话不可用；本设计未依赖它们 |

---

## 1. 现状盘点：已实现能力清单

### 1.1 架构定位（出处 `ARCHITECTURE.md §2`、`docs/详细设计…v2定稿 §11`）
- 现状三段链路：**iOS App（ASR/交互）→ center 中枢（voxsign.net：注册表/ASR/WS 投递/回执/OAuth）→ 设备端 harness（执行面）**。harness 是「设备端执行面」，Go 单二进制、仅标准库、零依赖（`go.mod`: go 1.22，无 require）。
- 原文明确 V0 非目标：「center WS 全协议对接、多租户/计费/对外发布」（`ARCHITECTURE.md §2.4`）。**即：企业端能力在 harness 中本就是「预留不实现」**，Server 必须新建，而非补全。
- 2026-10-04 起提出「公司级云服务」方向：谷歌账户即租户、云端统一网关、免费额度（`docs/产品定位与云架构理念-20261004.md` D1–D4；`docs/云端谷歌登录与发布架构-20261004.md`），并在 harness 内以 `VHS_MODE=cloud` 做了**最小原型**（`server/cloud.go`）。

### 1.2 代码模块盘点（出处：目录与 `main.go`）

| 模块 | 内容 | 成熟度判断（依据） |
|---|---|---|
| `main.go` | 子命令 `run/serve/repl/task/summary/compare/version`，v0.2.0 | 可运行 [E1–E8] |
| `input/` `recog/` `refer/` `memory/` | 输入容错、意图识别（8 类+ASK/REGISTER_TOOL…）、指代消解、个人词典 | 有单测，`go test ./input` 通过 [E17] |
| `space/` `risk/` `pipeline/` | 域（Space）边界、风险/确认三信号、编排管线（`execActions`） | 门禁真生效 [E1–E7]；部分意图占位 [E5] |
| `tools/` | `registry.go` 6 个内置契约 git/file/search/test/run/verify（含 risk 分级、`Register` 需人工确认才落盘）；`executor.go` 475 行：`exec.CommandContext` + 默认 30s 超时、file 写前备份、remote-desktop/weather 执行器 | 真执行 [E3–E6, E15] |
| `provider/` `route/` `router/` `modelcenter/` | 多 provider（OpenAI 兼容）、路由、模型中心配置（`config/model-center.json`：channels default/diagnose/learn/plan/research，tiers fast=deepseek-flash / quality=deepseek-v4-pro，网关 `https://aiops.voxsign.ai/api/model/chat`，`AIOPS_KEY`） | 网关实测可达 [E14] |
| `server/` | 手机 HTTP API：`/v1/tasks /v1/run /v1/voice /v1/task/ /v1/confirm /v1/cancel /v1/health /v1/status /v1/roles /v1/asr /v1/auth/google /v1/me /screenshots/`（`server/server.go:198-217`）；Token 或 127.0.0.1 鉴权；云端模式 JWT+租户+配额 | 本地/云端模式均实测 [E9, E10] |
| `server/cloud.go` | HS256 JWT 手写、Google JWKS(RS256) 验签、PKCE code 换 token、`tenantState{sub,email,tier(free/prime/enterprise),trial_until}`、`quotaState{date,tasks}`、免费档默认 30 任务/天、体验 15 天、JWT TTL 10h | **仅 Google 登录 + 日任务计数**；无组织/角色/计费/审计 |
| `asr/` + `cmd/vhs-asr` | ASR 个性化后台：`/v1/health /correct /dictionary /process /feedback /blacklist /observe /lexicon /task` | 服务可起 [E12]；真实音频未测 |
| `cmd/vhs-voice` | 语音适配层：`/v1/voice/parse /decompose /resolve /run /tasks/{id}`（去噪→拆单→补全→投递→汇总） | 可起，拆单质量有缺口 [E11] |
| `trajectory/` `selfheal/` `verify/` `ground/` `cache/` | append-only 轨迹（input_raw/clean/correct/intent/refer/space_check/risk/confirm/receipts/final/task_metrics）、自愈、独立校验器、认知切片 | 轨迹格式稳定（362 条请求样本） |
| `contract/` `contracts/` `doccontract/` | Go 数据契约（Message/ActionPlan/Receipt/Usage…）、`intent-v1.schema.json` | 公开契约的天然种子 |

### 1.3 现状关键事实（设计锚点）
1. **执行面在设备，身份面只有雏形**：现有租户 = Google `sub`，隔离靠文件系统目录 `<log_dir>/tenants/<sub>/`（HLD §5；实测 cloud 模式启动即创建 `tenants/`，[E10]）。
2. **云端模式下 harness 在云主机上执行**：这对多租户是风险点（见 §6.3）。
3. **Center WS 没有任何实现**：全仓 `*.go` 无 websocket/wss/gorilla 引用 [E13]；`ARCHITECTURE.md` 仅「预留：center WS 客户端」。
4. **模型/ASR 出口**：`aiops.voxsign.ai`（HLD 中又写作 `aiops.peterzou.com`，两个域名并存，见 §9 备注）→ 阿里 DashScope（千问）（HLD §3）。
5. **品牌**：对外统一 VoxSign；代码内 module 已改 `github.com/Voxsign/voxsign-harness`，但 `ios/` 下仍有 `VoiceSign` 目录/ipa 名、harness 仓名仍是 `voicesign-harness`——**对外禁用 VoiceSign 的要求目前未完全满足**（Server 对外面（域名、API 前缀、错误消息、MCP 名）须清扫）。

---

## 2. 能力基线实测（Capability Model）

### 2.1 方法
- 沙箱：`go build -o /tmp/vxs/vhs .`（0.5s 构建）；`VHS_LOG_DIR=/tmp/vxs/log`；临时 git 仓 `/tmp/vxs/proj`；手工注册 project 域 `scope=/tmp/vxs/proj/**`。
- 入口：`vhs run "<口语文本>"`（一次性跑完整管线，stdin 重定向为 /dev/null，需要确认时管道喂 `y`）。
- 没有用 `go run . repl`：REPL 是交互式，非交互执行器不适用；`vhs run` 与其走同一管线。

### 2.2 Capability Model 表

状态定义：**真执行**=产生了可核对的外部副作用/真实输出；**占位**=管线返回固定占位回执，不执行动作；**缺失**=无实现；**拦截**=门禁拒绝（属正确行为）。

| # | 能力 | 输入（口语） | 状态 | 耗时 | 证据 | 备注（质量） |
|---|---|---|---|---|---|---|
| E1 | QUERY | 「查看当前目录」 | **真执行（语义质量差）** | 2.87s | `search`+LLM 回答；回执声称「检索结果里没有目录内容，只命中本次请求的运行日志」，并建议用户自己去终端执行 | 走的是**文本检索**而非 `ls`；对「查目录」类问题答非所问。QUERY≠只读文件系统查询 |
| E2 | NOTE | 「记一下 VoxSign Server 设计实测」 | **真执行** | 17ms | 回执 `appendd: /tmp/vxs/log/notes.md`（回执动词拼写 `appendd/writed` 是 bug，源 `pipeline.go` NOTE/EDIT 分支） | 写入 `notes.md` 成功；`~/.voicesign/harness/notes.md` 有 50+ 条重复，说明**无幂等/去重** |
| E3 | EDIT（单行/整文件写） | 「把 hello-vxs 写到 /tmp/vxs/proj/b.txt」 | **真执行** | 15ms | `b.txt` 内容 `hello-vxs`（无尾换行） | 仅支持「写到<路径>」句式（`extractWriteTarget`）；不支持「改第N行」类真编辑 |
| E4 | COMMIT | 「提交一下代码」 | **真执行（强制人工确认）** | 24ms（未放行）/ ~43ms（放行） | 无 stdin：回执「待确认（human，未放行）」；喂 `y`：新增提交 `097ec1d 提交一下代码`，`git log` 可核对 | 不可逆动作永远人工确认——门禁行为正确；提交信息=口语原文（质量弱） |
| E5 | ORCHESTRATE | 「整理文档并提交，保存为《VX设计汇总》」 | **真执行（LLM）** | 12s | 读 1 份文档→生成 `docs/VX设计汇总.md`（6557B）→`git commit a98a4b7`；内容为中文汇总 | 仅支持固定文档白名单 `defaultOrchestrateSources`（`pipeline.go:776`），且要求命中「整理/文档/保存或提交」词表；复合口语（「先读 a.txt 再写总结再提交」）→ 被判 **ASK 回问**，未执行 |
| E6 | TEST | 「跑测试」 | **占位** | 16ms | 回执 `M2 已过 space_check+risk+confirm，动作待模型工具循环落地（intent=TEST）`；2026-10-05 21:01 harness 轨迹（早于本次沙箱）同样如此 | 代码 `pipeline.go:657-661` default 分支；`tools` 层 `test` 执行器是真实的，**只是意图→动作没接** |
| E7 | DEBUG | 「调试一下 main 报错」 | **占位** | 16ms | 同上；且槽位抽取错误（`DEBUG debug`，文件 `debug`） | 同 TEST |
| E8 | DEPLOY | 「部署到生产」 | **缺失（被门禁拦截）** | 17ms | `BOUNDARY_VIOLATION`；project 域工具表无 deploy；`planCaps` 声明 `deploy/http` 但 `Exec` 对未知工具返回「执行器未实现工具」（`executor.go` default 分支） | 既无执行器也无域授权。Server 的 DEPLOY 必须新建 |
| E9 | HTTP serve（本地模式） | `vhs serve` → `/v1/health`、`POST /v1/run` | **真执行** | 启动 <2s | health `{"ok":true,"version":"0.2.0"}`；`/v1/run` 返回 `task_id`（异步） | 无 token 时仅回环 |
| E10 | HTTP serve（云端模式 `VHS_MODE=cloud`） | 无 JWT 访问 `/v1/run`、`/v1/me` | **真执行（鉴权门）** | — | 401 `{"code":"unauthorized"}`；`/v1/health` 公开；启动即建 `tenants/` | **未测**：Google 真登录（缺 client id/secret）、配额 429、租户隔离（仅有设计 R15–R17，无测试） |
| E11 | 语音链路·适配层 `vhs-voice` | 「呃那个帮我跑一下测试然后提交代码」 | **真执行（拆单有缺口）** | <100ms | `/v1/voice/health` ok；`parse` 去噪成功（移除「然后/那个」），但 `decompose` 只产出 **1 个任务**（action=跑, target=代码），**丢失「提交」** | 复合指令拆分漏项；这是 Server 语音/编排的已知质量风险 |
| E12 | 语音链路·ASR 后台 `vhs-asr` | `/v1/health` | **服务可起；音频未测** | 2s | `{"contract_version":"1","status":"ok"}`；日志「L2 未装配（模型中心配置读取失败）: open config/model-center.json」——**依赖工作目录**，非仓库根启动会降级 | 未做真实音频→文本；未验证 DashScope ASR 计费/时延 |
| E13 | Center / WS | 全仓检索 websocket | **缺失** | — | 无 websocket 实现；`https://voxsign.net/` HTTP 200、`https://voxsign.ai/` 200（站点在，**harness 无 WS 客户端**） | 「center 注册表/WS 投递/回执」是文档概念，无代码 |
| E14 | 模型网关（对照） | `POST aiops.voxsign.ai/api/model/chat` | **可达** | 0.31s | 无 key→401；带 `X-AIops-Key`→400（鉴权通过，空体被拒）；key 长度 42，未打印 | 仅验证鉴权链路，未调用模型 |
| E15 | Executor `run` 工具直接调用 `claude -p`（方案 B 底层） | Go 程序 `tools.Executor{}.Exec("run", {command:[claude,-p,…]})` | **真执行** | 7.4s | ok=true，输出中文一句话答复；Timeout 默认 30s / 设 120s 均成功 | 见 §2.4 |
| E16 | 方案 A：Claude Code 本机互调 | `claude -p "reply exactly: CLAUDE-OK"` | **真执行** | 10s（含 stdin 等待 3s 警告，`</dev/null` 可省） | 输出 `CLAUDE-OK` | 本次设计整体即以方案 A 完成 |
| E17 | 单元测试 | `go test ./tools ./pipeline ./server ./input` | **通过** | 5.4s | 4 个包全 ok | 未跑全仓 `go test ./...`（含 live 测试，需网络/key） |
| E19 | 轨迹统计（2026-10-03） | 362 请求：87 有回执、15 refer、13 attribution、9 space_check、7 risk、7 confirm、5 verify | **真实使用数据** | — | `trajectory-20261003.jsonl` | 说明**大部分输入止步于意图/ASK 阶段**，真实执行占比 ≈24% |

### 2.3 能力模型汇总

| 状态 | 能力 |
|---|---|
| 真执行 | NOTE、EDIT（整文件写）、COMMIT（强确认）、ORCHESTRATE（固定文档白名单）、QUERY（文本检索+LLM）、HTTP serve、云端鉴权门、模型网关可达 |
| 占位 | TEST、DEBUG（意图→动作未接；底层 `test/run` 执行器真实可用） |
| 缺失 | DEPLOY（执行器+域授权）、Center WS、真实音频 ASR 实测、Google 登录实测、租户隔离/配额测试、RBAC/SSO/审计/计费/私有化 |
| 质量缺口 | QUERY 查目录答非所问；voice 拆单丢「提交」；复合口语被 ASK；NOTE 无去重；回执 `appendd/writed` 拼写；ASR 服务依赖 cwd |

### 2.4 方案 A / B 实测记录（验收 #4）

| 项 | 方案 A（Claude Code 指挥，Bash 调 harness） | 方案 B（harness `run` 工具执行 `claude -p`） |
|---|---|---|
| 能否跑通 | **能**：E1–E12 全部通过 Bash 驱动 harness 完成；`claude -p` 自检 CLAUDE-OK | **底层能；端到端口语入口不能**。`Executor.Exec("run", [claude -p …])` 跑通 [E15]；但从口语入口 `vhs run "跑测试 claude -p …"` 只会命中 TEST **占位**（轨迹 2026-10-05 21:01 已有同样记录），`pipeline.go` 没有把「执行命令」意图接到 `run` 工具 |
| 耗时 | harness 本地意图 15–45ms；含 LLM 的 QUERY 2.9s、ORCHESTRATE 12s；`claude -p` 单轮 ≈7–10s | `claude -p` 单轮 7.4s（Executor 默认 30s 超时够单轮，**不够多步子任务**，需显式放大） |
| 输出质量 | 高（可读回执、可核对副作用）；缺点：QUERY/voice 拆单质量缺口 | 输出文本 OK（stdout 截断到 `maxOut=4000` 字符，`executor.go`）；但**无回执写回**：没有把 Claude 结果写入轨迹/回执链的接线 |
| 结论 | 设计期用 A；A 是「开发期指挥」模式 | B 只能作为 Server 内「Agent Runner」的雏形：需补 ①意图→run 接线 ②超时/并发策略 ③回执写回 ④沙箱（§6.3、§7.2） |

### 2.5 实测局限（不得夸大）
- 未用真实麦克风/音频；未调用 `/v1/asr`。
- Google 登录、配额 429、租户隔离未实测（环境无 OAuth client）。
- 网关仅验证鉴权，未做模型调用质量/时延对照。
- 实测样本量 1 次/项，耗时仅作量级参考。
- 实测前 harness 仓已有未提交改动与 2026-10-05 21:01 的并行轨迹条目（非本次产生，取自 `~/.voicesign/harness/trajectory-20261005.jsonl`）。

---

## 3. 设计总纲：边界与闭源策略

### 3.1 VoxSign Server 是什么 / 不是什么

| | |
|---|---|
| **是** | 云端控制面 + 数据面：身份与租户、语音/ASR 服务、任务编排与派发、模型网关、计费配额、审计合规、设备注册与指令投递（即文档里的「center 中枢 + 云端 Harness 网关」，现状均为空白/原型） |
| **不是** | 不是设备端 harness（执行面，留在用户设备/自托管，保持现有定位）；不是 iOS 客户端；不是开源产品 |
| 与 harness 的关系 | harness = 执行面（Agent）；Server = 控制面。Server 通过**公开契约**与 harness 对接，不依赖 harness 内部实现 |

### 3.2 硬约束 #2「闭源：实现闭源，仅 API/MCP 契约公开」落地

| 层 | 公开 | 闭源 |
|---|---|---|
| 仓库 | `voxsign-contracts`（**公开**）：OpenAPI 3.1、MCP tool/resource schema、`intent-v1.schema.json`（现有，`contracts/`）、Receipt/ActionPlan JSON Schema（由 `contract/` 包导出）、错误码表、一致性测试套件（conformance tests）、SDK stub | `voxsign-server`（**私有**）：全部服务实现、迁移脚本、部署清单、运维 runbook |
| 产物 | OpenAPI/MCP 文档、契约版本号（现有 `contract_version:"1"` 即 `model-center.json` 与 ASR 健康接口里的字段） | 编译后的二进制/镜像（私有 registry，签名）；私有化交付仅给二进制/镜像，**不交源码** |
| 许可 | 契约仓：Apache-2.0（建议）；Server：专有许可（EULA） | 依赖清单(SBOM)可向企业客户出示，但不含源码 |
| 防回流 | CI 检查：`voxsign-contracts` 不得 import server 包；server 仓不得出现在公开 remote | harness 仓当前即 **PRIVATE**（`gh repo view`：`visibility: PRIVATE`）——现状与闭源策略一致，但 harness 本身是否开源**未决**（见 §9 说明：不影响本设计） |
| 契约演进 | 契约 `contract_version` 语义化；破坏性变更走 `/v2`；conformance suite 由契约仓发布，第三方客户端/自托管 harness 可自测 | — |

**现状依据**：`contract/` 包「只依赖标准库、不含运行逻辑」（`contract/contract.go` 头注释），`contracts/intent-v1.schema.json` 已是独立 schema，`doccontract/` 已有文档契约 —— 契约/实现分离已有雏形，Server 沿用并抬到仓库级别。

### 3.3 命名与对外面清扫（品牌约束）
对外面（域名、API 路径、错误文案、MCP server 名、SDK 包名、响应头）一律 `voxsign`/`VoxSign`；清理项：`ios/` 下 VoiceSign 命名、`voicesign-harness` 仓名对外不可见（保持私有）、HLD 中 `aiops.peterzou.com` 域名不得出现在对外面。

---

## 4. 总体架构

### 4.1 分层图

```
┌──────────────────────────── 接入层 ─────────────────────────────┐
│ iOS App · Web 控制台(企业管理) · 设备 Agent(harness) · 第三方 MCP/API 客户端 │
└───────────┬─────────────────────────────────────────────────────┘
            │ HTTPS(REST/SSE) · MCP · 设备通道(长连/WS，新建)
┌───────────▼───────────────── 网关层（闭源）──────────────────────┐
│ Edge Gateway：TLS、限流、WAF、租户解析、AuthN(JWT/OIDC/API Key)、 │
│ AuthZ(RBAC/策略)、计量打点、请求 ID/trace、契约校验(OpenAPI)        │
└───────────┬─────────────────────────────────────────────────────┘
┌───────────▼───────────────── 核心服务（闭源，Go）────────────────┐
│ ① Identity&Tenant  ② Voice(ASR/个性化)  ③ Task Orchestrator      │
│ ④ Model Gateway    ⑤ Billing&Quota     ⑥ Audit&Compliance        │
│ ⑦ Device Hub(设备注册/指令投递/回执)   ⑧ Admin/Console API        │
└───────────┬─────────────────────────────────────────────────────┘
┌───────────▼───────────────── 数据层 ─────────────────────────────┐
│ PostgreSQL(租户/RBAC/计费/任务元数据，行级隔离 RLS) · 对象存储(音频/产物/备份) │
│ Redis(会话/限流/队列) · 追加式审计存储(哈希链) · 密钥库(KMS/Vault)  │
└───────────┬─────────────────────────────────────────────────────┘
┌───────────▼───────────────── 上游 ───────────────────────────────┐
│ 模型/ASR 提供商（多家，区域可选）· Google/企业 IdP · 支付渠道 · 设备端 harness │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 现状→架构的出处映射
- 「统一网关 + 租户命名空间」：HLD §3、`产品定位 D4`；现有网关前身 `server/server.go` 的 `auth()/public()/cors()` 中间件链（`server.go:398-430`）。
- 「契约先行」：现有 `contract_version:"1"`（`config/model-center.json`、`vhs-asr /v1/health`）。
- 「设备=控制器/电脑=执行器、回执推送」：`docs/详细设计…v2定稿 §11`。
- 「模型网关」：`config/model-center.json` 的 channels/tiers 与 `modelcenter/`、`provider/`、`route/` 包。

### 4.3 技术选型（延续现有约束，不新增未论证选择）
- 语言 Go（定案约束，`ARCHITECTURE.md §2.2`）。**例外**：Server 允许第三方依赖（pgx、OIDC/SAML 库、OpenTelemetry、Redis 客户端），因为 harness 的「零依赖」约束出自「设备端单二进制」，不适用于云端服务——**这是对现有约束的显式放宽，需评审确认**（计入 §9 之外的 ADR-S1）。
- 存储：PostgreSQL（行级安全 RLS）替代现有「JSON 文件 + 目录」（现状：`tenants/<sub>/quota.json`，HLD §5）——文件存储无法做并发计费与审计一致性。
- 对外协议：REST + SSE（任务流）+ MCP（工具面）；设备通道先 **HTTPS 长轮询/SSE，再升级 WS**（因为 WS 现状为零实现 [E13]，先上最低风险通道）。

---

## 5. 核心服务设计

每个服务给出：职责 / 现状依据 / 差异动作（固化·补全·新建）/ 关键接口 / 验收。

### 5.1 Identity & Tenant（身份与租户）
- **现状**：Google OIDC + PKCE、JWKS 验签、HS256 会话 JWT，租户=Google `sub`，档位 `free/prime/enterprise`（`server/cloud.go:290-330`）。无组织、无成员、无角色、无企业 IdP。
- **设计**：
  - 数据模型：`org(tenant)` ← `membership(user,org,role)` → `user(identity)`。**tenant_id 与 Google sub 解耦**（个人=自动创建的单人 org，`org_id = personal:<sub>`，保持「零注册」体验；企业 = 管理员创建 org，成员以邮箱域/邀请加入）。
  - AuthN：保留 Google OIDC（个人）；新增通用 OIDC + SAML 2.0（企业 SSO，§6.2）；设备/自动化用 API Key / 设备证书（短期 JWT，由 Server 签发）。
  - 会话：JWT 改 **EdDSA/RS256 + kid 轮换**（现状 HS256 单密钥，`cloud.go:52`；`jwt-secret` 默认写 `<log_dir>/cloud/jwt-secret`，多实例/轮换不可行）。
- **差异动作**：固化 Google 登录链路与 PKCE；补全 org/成员/角色；新建 IdP 联邦、SCIM、API Key。
- **接口（契约公开）**：`/v1/auth/google`（现有）、`/v1/auth/oidc/{idp}`、`/v1/auth/saml/acs`、`/v1/me`（现有，扩展为含 org/role）、`/v1/orgs`、`/v1/orgs/{id}/members`。
- **验收**：双租户隔离测试（HLD R16）、JWT 过期/篡改拒绝（R15）、SSO 登录回路。

### 5.2 Voice（语音/ASR/个性化）
- **现状**：`/v1/asr`（server）、`vhs-asr` 服务（热词词典、纠错、反馈、黑名单、observe/lexicon）、`vhs-voice` 适配层（去噪→拆单→补全）、输入容错层（原文必留、词典纠错、低置信回问 `ARCHITECTURE.md §2.3`）。ASR 后台以 `/v1/health` 暴露 `contract_version`。
- **已知缺口**：拆单漏项 [E11]；ASR 服务依赖 cwd 读 `config/model-center.json` [E12]；个性化词典文件存储。
- **设计**：
  - Voice 服务把 ASR 后台 + 适配层收拢为一个服务，**词典/热词按租户（且可选按用户）分层**：org 级共享词库（Enterprise 卖点，HLD §5「团队共享热词库」）+ user 级私有词。
  - 音频只在内存/临时对象存储处理，默认不留存；留存须租户策略显式开启（§7）。
  - 复合指令拆分：把 `vhs-voice` 的规则拆单升级为「规则 + 模型校验」，并以 E11 的失败用例作为回归样本。
- **差异动作**：固化 ASR 契约 v1 与词典 API；补全租户分层、配置路径解耦、拆单质量；新建音频生命周期管理。
- **验收**：E11 回归样本通过；词典租户隔离；音频保留策略测试。

### 5.3 Task Orchestrator（任务编排）+ Device Hub（设备中枢）
- **现状**：`pipeline/` 的意图→space_check→risk→confirm→execActions；`/v1/tasks`（异步 `task_id`，[E9]）、`/v1/confirm`、`/v1/cancel`；轨迹 append-only。TEST/DEBUG 占位、DEPLOY 缺失 [E6–E8]；Center WS 缺失 [E13]；云端模式在服务器本机执行（HLD §5 命名空间）。
- **设计**：
  - **执行位置原则（核心）**：Server 本身**不在共享主机上执行租户代码/命令**。执行只发生在 ①租户自有设备上的 harness（经 Device Hub 投递），或 ②Server 管理的**隔离沙箱 Runner**（每任务独立容器/microVM，无共享文件系统，网络出口受控）。这直接修正现状「云端模式 harness 在云主机执行」的多租户风险。
  - 任务状态机：`received → clarify(ASK) → policy_check(域/风险/确认) → dispatched → running → awaiting_confirm → done/failed/cancelled`；与现有 `ASK/BOUNDARY_VIOLATION/待确认` 回执一一对应（实测见 [E1–E8]）。
  - 确认策略：沿用「三信号任一高风险即升级；不可逆永远人工确认」（`详细设计 §4`；[E4] 实测证明生效），Server 把确认请求推送到手机（`/v1/confirm`），企业策略可**加严不可放松**。
  - Device Hub：设备注册（设备证书）、心跳、指令队列（至少一次投递 + 幂等键）、回执回写。首版 HTTPS 长轮询，WS 作为后续。
  - **Agent Runner（方案 B 产品化）**：把 `claude -p`/任意 LLM Agent 作为受控 Runner，补齐 [E15] 暴露的缺口：超时/并发配额、回执写回、沙箱。
  - 意图→动作补全（来自 [E6–E8]）：TEST/DEBUG 接 `test/run` 执行器；DEPLOY 新建部署执行器 + 域授权 + 强确认；ORCHESTRATE 去掉固定文档白名单。
- **差异动作**：固化（意图/风险/确认/回执契约）；补全（TEST/DEBUG/ORCHESTRATE 泛化）；新建（Device Hub、Runner 沙箱、DEPLOY、任务状态机持久化）。
- **验收**：每个意图类的 e2e 真实副作用测试（沿用 [E1–E8] 的方法，转为 CI）；沙箱逃逸测试；设备断线重投幂等。

### 5.4 Model Gateway（模型网关）
- **现状**：`config/model-center.json`：channels（default/diagnose/learn/plan/research，含 timeout/max_concurrency/write_back）+ tiers（fast/quality）；网关 `/api/model/chat`，`X-AIops-Key` 鉴权 [E14]；`_timeout_note`：强模型一次规划实测 ≈59.6s，故超时统一 180s；`AIOPS_KEY` 只在云端持有，绝不下发 iOS（HLD §1）。
- **设计**：
  - 统一出口：所有模型/ASR 调用经 Model Gateway，**租户不接触上游 key**；按租户/档位/渠道限流（沿用 `max_concurrency` 语义）。
  - 多 provider + 区域路由：同一 channel 可配多个上游，按区域/合规标签选路（§8.4）。
  - 计量：每次调用写 `usage`（沿用 `contract.Usage{prompt/completion/total_tokens}`）→ Billing。
  - 租户策略：模型白名单、禁用外发（私有化时仅指向客户内网模型）。
  - 缓存友好：保持固定 tool contract 前缀字节稳定（`contract.ToolSchema` 注释说明的 prompt cache 设计），Server 侧不得改写前缀。
- **差异动作**：固化 channels/tiers 配置语义与超时经验值；补全多 provider/区域/计量；新建租户级策略与 key 托管。
- **验收**：计量与上游账单对账误差；超时策略（≥180s 与 L2PlanTimeout 关系）回归。

### 5.5 Billing & Quota（计费配额）
- **现状**：免费档每日任务数（默认 30，`FreeDailyTasks`）、体验会员 15 天、`checkAndConsume` 超限 429 `quota_exceeded`（`server.go:420-429`、`cloud.go:298-330`）；定价设想 Prime ¥29/月、Enterprise ¥499/月起（HLD §5）。**只有任务计数，没有 ASR 秒数/token 计量、没有账单、没有支付。**
- **设计**：
  - 计量事件（不可变）：`task`、`asr_seconds`、`llm_tokens(model,tier)`、`device_seats`、`storage_gb`，幂等键防重。
  - 配额层级：org 总额度 → 角色/成员/设备子额度；软限（告警）与硬限（429）分离。
  - 计划：Free / Prime / Team / Enterprise（Enterprise 含年度合同、用量包、超量后付费）；价格数字**仅沿用 HLD 设想，未经定价评审**。
  - 账单：月度账单 + 发票导出；支付渠道**待定**（不预设）。
- **差异动作**：固化 429 语义与体验期；补全多维度计量与持久化（现状为文件，需事务）；新建账单/发票/支付/用量包。
- **验收**：并发扣额不超卖；计量-账单-上游成本三方对账。

### 5.6 Audit & Compliance（审计合规）
- **现状**：harness 轨迹 append-only（`trajectory/`，kinds：input_raw/intent/space_check/risk/confirm/receipts/final…，样本 362 请求），`decisions.jsonl`（确认决策），`task_metrics`。ASR 原文无条件保留（`ARCHITECTURE.md §2.3`）。**这是设备本地日志，不具防篡改，也无租户维度统一视图。**
- **设计**：
  - 统一审计事件模型：`who(user/device/api_key) / what(action,intent,risk) / on(resource) / when / result / request_id / tenant`；映射现有轨迹 kinds 与 `decisions.jsonl` 字段，**设备端轨迹上行为审计的一个来源**。
  - 防篡改：追加式 + 哈希链（每批次签名），租户可导出验真。
  - 保留与导出：租户可配保留期；导出 JSONL/CSV；对接 SIEM（syslog/Webhook）。
  - 合规：数据主体请求（导出/删除）、数据处理协议、子处理方清单；音频/原文留存策略可配（§7.4）。
- **差异动作**：固化轨迹事件结构；补全统一事件、防篡改、导出；新建合规工作流。
- **验收**：删除租户后数据清除证明；哈希链校验；审计覆盖率（每个特权操作均有事件）。

---

## 6. 企业端原生设计（硬约束 #3）

> 原则：企业能力**从第一天进入数据模型与网关**，不做后补丁。现状依据：`tenantState.Tier` 已含 `enterprise` 档位（`cloud.go:293`），HLD 已定 Enterprise「团队共享热词库、专属云托管、SLA」——但全部为空壳，需新建。

### 6.1 多租户
- 隔离模型三档：**共享库行级隔离（RLS，默认）** / **独立 schema（Team）** / **独立实例与独立数据库（Enterprise 专属云、私有化）**。
- 每张表带 `org_id`，网关在请求入口解析并注入；跨租户访问由 RLS 兜底（防应用层漏判）。
- 文件/对象存储路径前缀 `orgs/<org_id>/…`，不再用「目录即租户」（现状 `tenants/<sub>/`，仅适合单机）。
- 验收：对应 HLD R16 扩展为自动化双租户渗透用例。

### 6.2 SSO / 身份
- 个人：Google（现状，保留）。企业：OIDC（Okta/Azure AD/Google Workspace）+ SAML 2.0；**强制 SSO** 开关（禁用个人登录加入该 org）；SCIM 2.0 自动开通/停用；JIT 仅限已验证域。
- 与「谷歌账户是唯一身份」决策（产品定位 D1）的关系：D1 理由是「不自建认证栈、不存密码」——企业 SSO 是**联邦而非自建口令体系**，不违背 D1 的动机，但文字上与「唯一」冲突，须评审修订（开放问题 Q1）。

### 6.3 RBAC
- 内置角色：`owner / admin / security_admin / billing_admin / member / viewer / device`；自定义角色 = 权限集合。
- 权限粒度对齐现有域（Space）与风险模型：资源动作 `task:create / task:confirm_irreversible / device:register / dictionary:write / audit:read / billing:manage …`。
- **与 harness 域边界叠加**：Server 侧 RBAC 决定「谁能发起什么」，设备侧 Space（global/project/sandbox/vault-notes/vault-creds/external，`详细设计 §13`）决定「设备上能碰什么」——两者**取交集**；组织策略可下发域模板与「不可逆动作必须由指定角色确认」。
- 现状依据：`/v1/roles` 端点已存在（`server.go:201`，注释 M5-3），**未读其实现，语义未知**，复用或避让须先核对，避免命名冲突。

### 6.4 审计
- 见 §5.6；企业额外：管理员操作审计、登录审计、导出审计、不可关闭的审计级别。

### 6.5 计费
- 见 §5.5；企业额外：合同/年度额度、部门成本分摊（按 org 内 group/标签）、用量上限告警、发票/对公转账。

### 6.6 SLA
- 现状：无任何 SLA 数据；实测基线仅有 aiops 平台端点延迟（中位 ~320–550ms，`AIOps接口全面测试报告`）与强模型规划 ≈59.6s（`model-center.json` `_timeout_note`）。
- 设计：先定义 **SLI**：API 可用性、任务派发延迟（P95）、ASR 端到端延迟、审计写入可靠性；SLO 在 PHASE 2 用压测基线定；**承诺数字（如 99.9%）不在本稿写死**，因无实测依据；SLA 以合同附件形式（含服务积分）。
- 专属云：独立实例 + 独立监控 + 变更窗口。

### 6.7 私有化部署
- 交付形态：**签名镜像 + Helm chart/compose + 离线安装包**，仅二进制不含源码（与 §3.2 闭源一致）；License 文件（租户数/座席/到期/特性位，离线验签）。
- 外部依赖可替换：模型网关可指向客户内网模型；IdP 指向客户 AD；对象存储 S3 兼容；**无强制外联（telemetry 默认关，可选回传）**。
- 升级：版本间数据库迁移前向兼容 + 回滚包；契约版本对齐检查（`contract_version`）。
- 现状依据：HLD §6「B 独立 VPS Docker 单二进制 + systemd + Caddy」「部署单元 Dockerfile + docker-compose」是私有化形态的直系前身，固化后加 Helm 与离线包。

---

## 7. 安全设计

### 7.1 总体
- 零信任：每个请求都带租户与主体；服务间 mTLS；密钥在 KMS/Vault，不落配置文件。
- 现状已有且保持：回环绑定/Token 鉴权（`server.go`）、`AIOPS_KEY` 仅云端持有（HLD §1）、不可逆动作永远人工确认 [E4]、域默认拒绝（`BOUNDARY_VIOLATION` [E8]）、工具契约注册需人工确认才落盘（`registry.go:204-213`）、写文件前备份（`executor.go`）。

### 7.2 威胁与对策（仅列与现状相关者）

| 威胁 | 现状暴露（出处） | 对策 |
|---|---|---|
| 云端模式在共享主机执行命令 | 云端 harness 直接 `exec.CommandContext`（`executor.go:126-135`），`remote-desktop` 可「运行应用/只读命令」 | §5.3：共享主机禁用 `run/remote-desktop`；仅沙箱 Runner/租户设备执行；命令白名单+网络出口策略 |
| JWT 单对称密钥 | HS256，secret 缺省写本地文件（`cloud.go`、`config.go:75`） | 非对称签名、kid 轮换、短 TTL + 刷新、撤销列表 |
| 租户隔离靠目录 | `tenants/<sub>/` | RLS + 对象前缀 + 渗透测试 |
| 提示注入/越权（语音→命令） | `extractReadOnlyCmd` 仅白名单只读命令（`executor.go:405-419`）；Space 边界 | 保持白名单；模型输出的动作一律过 policy_check；Runner 不继承 Server 凭证 |
| 凭证泄露 | 本机 `~/.aiops/keys/` 目录下除 `peter-mac.key` 外还有 azure-email-graph.json、mansour-password.txt、mansour-refresh-token.json 等凭证类文件（仅 `ls` 看到文件名，未读取内容） | Server 不托管用户明文凭证；`vault-creds` 域保持设备本地；Secrets 走 KMS；扫描并清理历史文件 |
| 审计可篡改 | 本地 jsonl | §5.6 哈希链 |
| 配额绕过/滥用 | 文件计数 | 事务扣额 + 限流 + 异常检测 |
| 供应链 | 现 harness 零依赖 | Server 引入依赖后：SBOM、依赖扫描、镜像签名 |
| 数据外泄至上游模型 | 请求发往第三方（DashScope） | 租户级「禁止外发/区域锁定/脱敏」策略（§8.4） |

### 7.3 加密与密钥
传输 TLS1.2+（沿用 Caddy/Let's Encrypt 方案，HLD §6）；静态加密（DB/对象存储）；企业可选 BYOK（私有化/专属云）。

### 7.4 数据保留（默认值待评审）
音频：默认处理后不留存；ASR 原文：现状「无条件保留」属**设备端轨迹原则**，云端按租户策略（默认保留期由合规评审定，本稿不给数字）；备份加密。

---

## 8. 部署与运维

### 8.1 现状
- 单二进制、`VHS_MODE=cloud|local`、`build.sh`、`dist/`、`.githooks`、`.github`（CI）；HLD 三形态：本地模拟云端（P2）、平台同机子域（P3 首选）、独立 VPS Docker。
- 当前无监控/备份实现（仅 `~/.voicesign/harness/backups/` 为文件写前备份，不是系统备份）。

### 8.2 目标部署架构
- 环境：dev / staging / prod；多区域（见 8.4）；SaaS 多租户集群 + 专属云 + 私有化三形态共用同一套镜像。
- 无状态服务（网关/核心服务）水平扩展；PostgreSQL 主从 + PITR；Redis 哨兵；对象存储跨区复制（受数据驻留约束）。
- 设备通道与 API 分离伸缩（长连接数量特性不同）。

### 8.3 AIOps 接入与运维
- 现状平台：AIOps 网关，`X-AIops-Key`（鉴权实测 [E14]），key 文件 `~/.aiops/keys/peter-mac.key`（`export AIOPS_KEY=…` 格式；本次未输出其内容，仅验证长度 42）；平台能力（CMDB hosts/free/expiry/domains、skill 发布需 write-grant）见 `AIOps接口全面测试报告`；该报告发现 configure/panel 页「回退首页（未独立实现）」。
- 设计：
  - Server 的**生产密钥不使用个人 key**：每个环境/服务独立 AIOps 服务账号 key，写入 KMS，90 天轮换；`peter-mac.key` 仅限开发机互调，禁止进入镜像/仓库。
  - 部署流水线经 AIOps `cicd`（route 别名「发布/上线/部署/构建」→ cicd，该报告）；主机/域名/到期通过 CMDB 登记，**到期登记现为「大量待补」**（报告），上线前补齐。
- **监控**：RED 指标（请求/错误/延迟）、任务派发延迟、设备在线数、模型上游延迟/错误率、配额与账单异常；追踪（OpenTelemetry）；日志集中、PII 脱敏。沿用现有 `task_metrics`（loop_ms/net_ms/ok/归因）作为任务级指标原型。
- **告警**：按 SLI（§6.6）分级；值班手册随 `docs/` 管理。
- **备份与恢复**：DB 每日全量 + WAL 连续归档；每季度演练恢复；备份加密，保留与驻留与租户策略一致。
- **自愈**：现有 `selfheal/`（异常自愈、诊断 provider，`main.go: selfheal.PrepareDiagKey`）可作为运维自愈雏形，但仅限诊断建议，不自动执行破坏性变更。

### 8.4 部署区域（「非中国境内」）
- 任务书要求部署考虑非中国境内。现状事实：①模型/ASR 经平台转阿里 DashScope（HLD §3）——上游在中国大陆生态；②voxsign.net / voxsign.ai 均可访问 [E13]；③本机时区 +03:00，harness 内置城市表含利雅得/迪拜/多哈（`executor.go weatherCities`），**暗示用户/部署重心在中东，但这是推断，非明确需求**。
- 设计：Server 部署区域为**可配置的「区域单元(cell)」**，每个 cell 自带 DB/对象存储/模型出口；租户创建时绑定 home cell，数据不跨 cell。首个生产 cell 放非中国境内区域；模型网关按 cell 选 provider。
- **未验证风险**：DashScope 国际站是否满足时延/合规与计费；境外 cell 访问大陆上游的延迟与可用性 —— 需 PHASE 1 实测后定（见 Q2）。

---

## 9. 开放问题（3 条，均附建议）

**Q1：企业 SSO 与「谷歌账户是唯一身份」（产品定位 D1）如何调和？**
- 事实：D1 明确「无注册/密码体系，Google 唯一标准」；企业端需 OIDC/SAML/SCIM，且 tenant 必须是 org 而非个人 sub。
- **建议**：修订 D1 为「**身份来源=联邦 IdP（Google 为默认个人 IdP，企业自带 IdP）；不自建口令体系**」，租户 ID 与 sub 解耦（个人 org = `personal:<sub>`）。D1 的四个收益（零注册、安全外包、跨设备、挂配额）全部保留。需 Peter 拍板。

**Q2：Server 的首个生产区域与模型/ASR 上游如何确定？**
- 事实：现状上游为 DashScope（HLD §3）；任务书要求考虑非中国境内；未验证境外访问与国际站可用性；`aiops.voxsign.ai` 与 `aiops.peterzou.com` 两个域名在文档中并存。
- **建议**：PHASE 0 内做一次 **48 小时多区域探测**（时延/可用性/DashScope 国际站/备选 provider），再定首个 cell；Model Gateway 从第一天就支持多 provider，避免被单上游绑死；统一对外域名为 voxsign.ai，`peterzou` 域名仅内部。

**Q3：云端任务到底在哪执行（云沙箱 vs 仅用户设备）？**
- 事实：现有云端模式在云主机上执行 [E10/HLD §5]，多租户下不安全；设备通道 WS 零实现 [E13]；Runner（方案 B）已验证底层可行 [E15] 但无沙箱。
- **建议**：**首版仅「用户设备执行」（Device Hub 长轮询）+ 云端只做编排与模型调用**；云沙箱 Runner 列为 PHASE 3 付费能力。理由：最小攻击面、与「手机=控制器、电脑=执行器」定位一致（`详细设计 §11`），且延期云沙箱不影响企业核心卖点。

---

## 10. 路线图（PHASE 0–4）

> 时长不估算（无团队规模/速度依据）；以**出口判据**定义阶段完成。

| PHASE | 目标 | 主要交付 | 出口判据（可验证） |
|---|---|---|---|
| **0 契约与地基** | 契约先行、闭源边界、区域探测 | 建 `voxsign-contracts`（公开）与 `voxsign-server`（私有）；OpenAPI v1 覆盖现有端点（`/v1/tasks /run /confirm /cancel /asr /auth/google /me /health`）；错误码表；conformance 套件首版；品牌清扫；Q1/Q2/Q3 拍板；多区域探测报告 | 契约仓不 import 私有包（CI 检查）；现有 harness 通过 conformance 套件；Q1–Q3 有决议 |
| **1 云端 MVP（个人）** | 把现有云端模式固化为可上线服务 | PostgreSQL 数据层（org/user/membership/quota）；Google 登录固化；JWT 非对称+轮换；Edge Gateway；Model Gateway（多 provider）+计量；Voice 服务收拢（ASR+适配层，修 E11/E12）；Device Hub v1（长轮询）；统一审计事件 v1；首个 cell 部署（Docker+Caddy） | HLD R15–R18 全部**自动化**通过；双租户隔离测试通过；超限 429 实测；E1–E8 回归样本入 CI（含 TEST/DEBUG 接线） |
| **2 企业核心** | 企业端原生能力上线 | OIDC/SAML SSO + SCIM；RBAC（含与 Space 取交集）；策略下发；审计哈希链+导出；计费 v1（多维计量、账单、用量包）；SLI 采集与压测基线→定 SLO；管理控制台 API | 企业演示租户完成：SSO 登录→角色限权→不可逆确认→审计导出验真→账单对账；SLO 草案基于实测 |
| **3 专属云与私有化** | 高阶企业形态 | 独立实例部署；Helm+离线安装包+License；BYOK；沙箱 Runner（Q3 建议的付费能力）；DEPLOY 执行器+域授权；WS 设备通道 | 在隔离网络内一键安装并跑通 E1–E8；License 离线验签；沙箱逃逸测试通过；升级/回滚演练 |
| **4 规模化与合规** | 稳定运营 | 多 cell 与数据驻留；SIEM 对接；合规认证准备（范围待定）；成本优化；第三方客户端/MCP 生态（基于公开契约） | 灾备演练达标；多区域租户数据不跨 cell 的审计证明；契约生态内至少 1 个第三方通过 conformance |

### 10.1 差异映射表（现有能力 → 设计落点 → 动作）

动作定义：**固化**=已有且可用，抬为正式契约/服务；**补全**=已有雏形或占位，需补齐；**新建**=现状无。

| 现有能力（出处/实测） | 设计落点 | 动作 | 依据 |
|---|---|---|---|
| 意图 8 类+ASK 契约（`contract/`、`contracts/intent-v1.schema.json`） | 公开契约仓 | **固化** | 契约/实现已分离 |
| 风险/确认三信号、不可逆永远人工确认 | Orchestrator 策略引擎；企业只能加严 | **固化** | [E4] |
| 域（Space）边界，默认拒绝 | 设备侧域 + Server RBAC 交集 | **固化**+补全 | [E8] |
| 轨迹 append-only（kinds 样本 362 请求） | 审计事件设备来源 | **固化**→补全（统一事件/防篡改） | trajectory-20261003 |
| NOTE/EDIT/COMMIT 真执行 | Runner/设备执行器 | **固化**（修拼写、去重、提交信息质量） | [E2–E4] |
| ORCHESTRATE 固定白名单 | Orchestrator 通用多步 | **补全**（去白名单、复合口语不 ASK） | [E5] |
| QUERY（文本检索+LLM） | Orchestrator 查询意图 | **补全**（查目录/文件走真实列表） | [E1] |
| TEST/DEBUG 占位 | Orchestrator 意图→动作接线 | **补全**（`test/run` 执行器已真实） | [E6, E7]、`pipeline.go:657` |
| DEPLOY 缺失 | 部署执行器+域授权+强确认 | **新建** | [E8] |
| `server/server.go` HTTP API（/v1/tasks 等） | Edge Gateway 公开 REST 契约 | **固化**（OpenAPI 化） | [E9] |
| `cloud.go` Google 登录+JWT | Identity 服务 | **固化**；HS256→非对称 | `cloud.go:52` |
| `tenantState`/`quotaState`（文件） | Billing&Quota（PG+事务） | **补全** | `cloud.go:290-330` |
| 租户=`tenants/<sub>/` 目录 | org + RLS + 对象前缀 | **补全**（模型变更） | HLD §5 |
| org/成员/角色/SSO/SCIM/API Key | Identity & RBAC | **新建** | 现状无 |
| `model-center.json` channels/tiers/超时经验 | Model Gateway 配置语义 | **固化**+补全（多 provider/区域/计量） | [E14] |
| `vhs-asr` 词典/纠错/反馈 API | Voice 服务 | **固化**；补租户分层与 cwd 解耦 | [E12] |
| `vhs-voice` 去噪/拆单/补全 | Voice 服务适配层 | **补全**（拆单漏项） | [E11] |
| Center WS / 设备注册 | Device Hub | **新建**（先长轮询） | [E13] |
| 方案 B：`Executor.Exec("run", claude -p)` | Agent Runner | **补全**（意图接线、回执写回、沙箱） | [E15] |
| `selfheal/` 诊断 | 运维自愈（仅建议） | **固化** | `main.go` |
| `task_metrics`（loop_ms/net_ms/归因） | 任务级监控指标 | **固化**→补全 | trajectory |
| Docker/Caddy/systemd 部署设想（HLD §6） | 私有化与 SaaS 同镜像 | **补全**（Helm/离线包/License） | HLD §6 |
| 计费/发票/支付 | Billing | **新建** | 现状只有计数 |
| 审计哈希链/导出/合规工作流 | Audit | **新建** | 现状无 |
| SLA/SLO | 运营 | **新建**（待实测基线） | 无基线 |

---

## 11. 自检：三条硬约束逐条核对

### 11.1 硬约束 #1「独立完整：覆盖背景/边界/架构/服务/企业端/安全/部署/路线图」

| 要求 | 位置 | 判定 |
|---|---|---|
| 背景 | §0、§1、§2（现状与实测） | ✔ |
| 边界 | §3.1–3.3 | ✔ |
| 架构 | §4 | ✔ |
| 服务 | §5（6 项核心服务 + Device Hub） | ✔ |
| 企业端 | §6 | ✔ |
| 安全 | §7 | ✔ |
| 部署（含运维/AIOps/监控/备份/非中国境内） | §8 | ✔（境外可行性标「未验证」） |
| 路线图 | §10 PHASE 0–4 | ✔ |
| 独立性 | 本文可脱离 harness 阅读：Server 与 harness 仅经公开契约耦合（§3） | ✔ |
| 缺口声明 | README 缺失、`~/voxsign` 无现行代码、部分 docs 未精读（§0） | 已披露 |

### 11.2 硬约束 #2「闭源：实现闭源，仅 API/MCP 契约公开」

| 检查点 | 位置 | 判定 |
|---|---|---|
| 仓库拆分（契约公开 / 实现私有） | §3.2 | ✔ |
| 私有化交付不含源码 | §3.2、§6.7 | ✔ |
| CI 防回流 | §3.2、PHASE 0 出口 | ✔（设计） |
| 契约内不含实现细节 | §3.2（OpenAPI/MCP/Schema/错误码/conformance） | ✔ |
| 现状一致性 | harness 仓 PRIVATE（`gh repo view` 实测） | ✔ |
| **风险点** | §4.3：Server 引入第三方依赖，需遵守其许可证（避免 AGPL 等强 copyleft 依赖污染闭源）——**已列为待评审项，设计未逐个核许可证** | ⚠ 需在 PHASE 0 建依赖许可白名单 |
| **未决** | 契约仓若公开，`contract/` 包中是否含内部字段需脱敏审查（未逐字段审） | ⚠ 待做 |

### 11.3 硬约束 #3「企业端原生：多租户/RBAC/SSO/审计合规/计费/SLA/私有化」

| 能力 | 位置 | 原生性判定 |
|---|---|---|
| 多租户 | §5.1、§6.1 | ✔ 数据模型层（org_id+RLS），非后补 |
| RBAC | §6.3 | ✔ 与 Space 取交集 |
| SSO | §6.2 | ✔ 含 SCIM；与 D1 冲突已列 Q1 |
| 审计合规 | §5.6、§6.4 | ✔ 哈希链+导出 |
| 计费 | §5.5、§6.5 | ✔ 计量事件模型 |
| SLA | §6.6 | ⚠ 仅 SLI 与流程；**无实测基线，SLO 数字未定**（如实披露） |
| 私有化部署 | §6.7、PHASE 3 | ✔ 与 SaaS 同镜像 |
| 路线图中企业能力的位置 | PHASE 1 已含 org/RBAC 数据模型与审计 v1；PHASE 2 完整企业能力 | ✔ 非「最后补」 |

### 11.4 「无依据不写」自查
- 已标注推断/未验证：中东部署重心（§8.4，推断）、SLA 数字（未写）、价格（沿用 HLD 设想，未评审）、`/v1/roles` 语义（未读实现，待核对）、境外上游可行性（未验证）、第三方依赖放宽（待评审）。
- 未写入任何未经核实的性能承诺。

### 11.5 验收标准对照（任务书 §四）

| # | 标准 | 状态 |
|---|---|---|
| 1 | 每个服务设计引用代码/接口/实测证据 | ✔（§5 各节「现状」、§10.1 依据列） |
| 2 | 三硬约束贯穿 | ✔（§11.1–11.3；2 条 ⚠ 如实披露） |
| 3 | 开放问题 ≤3 且各有建议 | ✔（Q1–Q3） |
| 4 | 方案 A/B 实测记录 | ✔（§2.4） |
| 5 | 设计 md + 基线报告 + 差异表；HTML+飞书双交付 | **部分**：md 含基线报告（§2）与差异表（§10.1）已完成；**HTML 转换与飞书交付属豆包侧，本执行者未做**；GitHub 发布见下 |

### 11.6 交付状态（GitHub-first 规则）
- 本地文件：`VoxSign-Server-设计.md`（本文件）。
- **GitHub 登记：SYNC_PENDING**。未发布：①harness 仓为私有，但本设计含闭源策略与安全细节，发布到哪个仓/Issue 需 Peter 确认；②未发现已有 "VoxSign Server" 设计 Issue（`gh issue list` 在 smithpeter/voicesign-harness 搜索仅命中 #4 测试派单，不相关）；③治理 Issue metasystem-live#155 存在（OPEN）。待授权后：建任务 Issue（repo/issue/产出位置/验收/回写目标）、提交本文件、回读核验。
- 沙箱产物：`/tmp/vxs/`（日志、临时仓、planb 测试程序），可删除，不影响 `~/.voicesign/harness`。
