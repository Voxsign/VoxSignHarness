# VoxSign Harness 架构设计文档（v1，2026-10-05 修复后）

> 本文以 **修复后真实代码** 为准重写（替代 V0 评审稿）。凡与代码不符处以代码为准；
> 已知的目标态 vs 现状差异在「§11 执行模型」中标注。仓库零外部 Go 依赖（纯标准库）。

---

## 1. 分层总览

```
┌──────────────────────────────────────────────────────────────┐
│ 客户端：iOS VoiceSign（语音/文本入口、四行回执渲染）           │  ios/
└───────────────┬──────────────────────────────────────────────┘
                │ HTTP（线A入口）
┌───────────────▼──────────────────────────────────────────────┐
│ 线A 入口 server（server/）：HTTP 路由、任务表 byReq 去重、     │
│   串行闸、确认桥、SSE 事件、设备注册、云租户隔离               │
└───────────────┬──────────────────────────────────────────────┘
                │ pipeline.Run(ctx, o, text)
┌───────────────▼──────────────────────────────────────────────┐
│ 交互核心 pipeline（pipeline/）：13 阶段编排循环（见 §3）       │
└──────┬───────────────┬──────────────┬───────────────┬────────┘
       │               │              │               │
┌──────▼─────┐  ┌──────▼─────┐  ┌─────▼──────┐  ┌──────▼─────┐
│ tools 工具族 │  │ provider   │  │ memory/词典 │  │ space 空间 │
│ exec/search │  │ 多模型路由  │  │ 个人上下文  │  │ 域/门禁     │
└────────────┘  └────────────┘  └────────────┘  └────────────┘
   另有 refer(指代) risk(风险分级) verify(独立校验) search selfheal(自愈三环)
   plan(规划) route/router(意图路由) ground(认知切片) hotcache zhiji(在途)

┌──────────────────────────────────────────────────────────────┐
│ 线B 独立 ASR 个性化服务：cmd/vhs-asr（默认监听 127.0.0.1:8787）│
│   暴露 /v1/process /v1/dictionary /v1/feedback /v1/lexicon   │
│   云端 model-center（modelcenter/）：千问 ASR + chat 通道      │
└──────────────────────────────────────────────────────────────┘
```

**线A / 线B 边界（架构契约，archstub D2/D3）**：server（线A）**只通过 HTTP** 调线B；
`asr` 包不得反向 import 核心（pipeline/plan/space/tools）。
（注：`e2e/asr_arch_contract_test.go` D3 当前预期红——asr 仍反向 import 核心包，属已知改造项。）

---

## 2. 目录结构（对照实际 `ls`，2026-10-05）

| 目录 | 角色 |
|---|---|
| `main.go` / `machine.go` | CLI 入口（serve/repl/task 命令）；machine.go 为在途配置 |
| `cmd/vhs-asr` | **线B** ASR 个性化服务二进制（默认 `127.0.0.1:8787`） |
| `cmd/vhs-voice` | 语音适配层（:8950 → 上游主 harness :8941），非线B |
| `cmd/vhs-ledger` | 账本独立二进制 |
| `server/` | **线A** HTTP 入口、任务表、串行闸、确认桥、SSE、云租户、健壮性四件套（robust.go） |
| `pipeline/` | 13 阶段编排循环核心（Run/Options/outcome） |
| `trajectory/` | append-only JSONL 轨迹（kind 单一枚举 + Validate） |
| `input/` | 输入清洗/纠错五级管线、个人词典接入 |
| `refer/` | 指代消解（"这个/那个"→具体对象） |
| `space/` | 空间/域注册与门禁（space.Check） |
| `risk/` | 风险分级 + 不可逆清单 + Guard 疲劳降级 |
| `verify/` | 独立校验器（读现实，不自报） |
| `tools/` | 工具族注册表 + Executor（file/search/run/note…） |
| `provider/` | 多模型 provider 接口与 Registry |
| `router/` `route/` | 模型路由表 / 意图路由 |
| `memory/` | 个人词典/记忆持久化 |
| `hotcache/` `cache/` | L2 热词缓存 / 四元确认缓存 |
| `modelcenter/` | 云端 model-center 通道（ASR + chat） |
| `selfheal/` | 自愈三环（知识库→诊断模型→只读安全重放） |
| `plan/` | 任务规划与校验计划生成 |
| `ground/` | ground 认知切片（项目图/回问上下文） |
| `contract/` `contracts/` | 意图/动作/回执契约；`contracts/intent-v1.schema.json` 为意图权威 |
| `doccontract/` | 工具契约自举装载 |
| `zhiji/` | 在途自模型架构（向量索引/STM/边规则），独立演进 |
| `bench/` | `startup_bench.py` 启动基准（见 §7） |
| `e2e/` | 端到端冒烟 + archstub 架构契约测试 |
| `eval/cases/` | 用例集 + 签署（ratification） |
| `docs/` | 设计/评审/校准文档（历史材料，不在本重写范围） |
| `ios/` | iOS 客户端 |
| `harness-output/` `Result/` `_archive/` | 产物/归档（只读，不入源码契约） |

---

## 3. pipeline 13 阶段（Run 的控制流）

`pipeline.Run(ctx, o, text)` 串行闸（`o.mu`）内依次：

1. `input_raw` 落轨迹（先于一切处理）
2. clean（input.Cleaner）
3. correct（个人词典纠错）
4. intent（意图分类）
5. refer（指代消解，写 `refer` 轨迹）
6. space_check（域/门禁，写 `space_check` 轨迹，拒绝即止）
7. risk（风险分级，写 `risk` 轨迹）
8. confirm（按 level 分派：auto/light/strong/human，写 `confirm` 轨迹；human 永不入四元缓存）
9. exec（tools 执行，写 `receipts` 轨迹）
9-bis. selfheal（有失败回执 → 诊断 + 只读安全重放，限 2 轮）
10. verify（独立校验，写 `verify` 轨迹；fail → 进诊断层）
11. attribution（归因回写，写 `attribution` 轨迹）
12. 四元缓存（非不可逆放行才 Set）
13. final（四行回执，写 `final` + `task_metrics`）

---

## 4. 意图分类（唯一权威）

**`contracts/intent-v1.schema.json` 是意图 JSON 的唯一权威 schema。**
pipeline `input` 的确定性分类器产出的 `contract.Intent` 必须满足该 schema；
任何意图枚举/字段以该 schema 为准，不在本文档或代码注释里另立副本。

---

## 5. 可观测性（trajectory，append-only JSONL）

轨迹是「原文→理解→动作→结果」的可重放证据链，0600 逐条落盘。

**kind 单一枚举**（`trajectory.Kinds`，P0-1 修复后已覆盖 pipeline 全部写入点）：

- 基础事件：`input_raw / input_clean / input_correct / intent / intent_source / start / model / actions / receipts / final / error / task_metrics`
- 13 阶段中间判定事件（P0-1 登记，此前写入点引用但未登记 ⇒ 被静默丢弃）：
  `refer / space_check / risk / confirm / verify / attribution`

**写入口径（P0-1b）**：未登记 kind 不再被 `_ =` 静默吞错——`pipeline.(*Options).write` 对
`Trace.Write` 返回的 error 打 `[trajectory] write 被丢弃` warning（仍不阻断只读任务，#44 语义保留）。

**request_id 贯通现状（P0-4a/4b 修复后）**：
- 入口（`/v1/tasks` 的 body `request_id` / HTTP 头 `X-Request-Id`）→ `Options.RequestID` →
  pipeline 轨迹 Entry 的 `request_id` → selfheal 诊断 trace 的 `RequestID`。
- 三类日志带 rid：server 访问日志、ASR 平台/校准日志、selfheal 诊断日志。
- 2026-10-05 断链点已焊死：
  - `Run` 生成 rid 后回写 `o.RequestID`（每任务克隆内统一读到规范 rid）；
  - `repairFailed` 的 SafeRetry trace 经 `Attempt.RequestID` 带主链 rid；
  - `llmSummarize` 诊断日志带 rid；
  - 旧入口 `/v1/run`(handleRun)、`/v1/voice`(handleVoice) 从 `X-Request-Id` 头注入 `o.RequestID`（无则空→自生成）。
- **后续待办（只记录建议，不碰 ios/ 代码）**：
  - **iOS 发版清单（合并为一次发版，C4 阶段落地，不单独发）**：
    - ① 出站请求加 `X-Request-Id` 头（App 生成/透传）；
    - ② 请求体加 `session_id` / `speaker_id` 字段。
    以上三项**合并进同一次 iOS 发版**（与 C4 的 APIClient.swift 改动同批），不单独发版；
    当前服务端已就绪（读头/读体透传），缺的是客户端发头/发字段。
  - iOS 应同时透传**真实 `X-Session-Id`**（按住说话的会话 id），服务端已桥接 `callASRProcess`
    （缺省仍 "voice"，不破坏既有 schema）。

---

## 6. 安全（红线）

- 串行闸：同进程同时只允许一个 Run 在飞（保护用户未提交改动，绝不覆盖他人文件）。
- 不可逆（commit/deploy/human）永远人工确认，永不被四元缓存学习掉。
- space.Check 拒绝即绝不执行；verify 读现实不自报；归因必须来自真实错误。
- 回环鉴权：ASR 服务 token 空 ⇒ 只允许回环地址（NF-5，对齐线A）。

---

## 7. 性能基准

**启动基准以 `bench/startup_bench.py` 为准**（替代 V0 稿里的描述）：冷启动到首个可响应的耗时。
任务层硬上限 `taskMaxDeadline = 10min`（外部调用再卡也不拖死进程，见 `server.runPipeline`）。

---

## 8. 验证方法论

1. **单元判据（先红后绿）**：各包 `*_criteria_test.go` 用 go/parser 机械提取符号做对账，
   例如 `trajectory` 包判据⑪：Kinds 单一枚举 + 未登记 kind 必须报错（不手写列表，防"假绿空过"）。
2. **可执行回归门**：
   - `CGO_ENABLED=0 go test ./...`
   - `CGO_ENABLED=0 go test -tags archstub ./e2e -run TestArch -v`
   - （注：本机 macOS 27 上默认 `CGO_ENABLED=1` 的外部链接器产出缺 LC_UUID 的测试二进制，
     dyld 直接 abort；本仓库零 cgo，故统一用 `CGO_ENABLED=0` 跑测试，语义不变。）
3. **架构契约（archstub D1–D7）**：线A/线B 边界、无 ASR 主闭包、可输入校验等结构不变量。
4. **verifylint / 校准报告**：见 `docs/` 下关键文档与 `eval/` 用例集。

---

## 9. 健壮性（robust.go）

外部调用四件套：分级超时（fast/mid/long）、熔断、指数退避重试（仅幂等）、有界 bulkhead。
`safeGo`：所有业务后台 goroutine 受 recover 保护——单任务 panic 被隔离，打日志+栈、写轨迹 error、
任务标 canceled，进程继续存活（P0-2）。

---

## 10. 配置与端口

- ASR 线B 默认端点 `http://127.0.0.1:8787`（`VHS_ASR_ENDPOINT` 覆盖）。
  查证结论：实际承担线B 的是 `cmd/vhs-asr`（监听 8787，暴露 `/v1/process`）；
  旧默认 8123 为配置漂移，已统一（P0-3）。启动日志互提示：vhs-asr 打"线A默认端点=…"，
  server 打"实际ASR端点=…"。
- vhs-voice（:8950）是语音适配层，非线B；主 harness 默认 :8941。

---

## 11. 执行模型（目标态 vs 现状）

**目标态（已定案原则）**：每虚拟用户一个 Runner、中央调度器（按优先级/串行闸分派）、
trace id（request_id）从入口贯穿到日志与轨迹，多租户隔离。

**现状（如实标注）**：
- 临时 goroutine + 串行闸：每个任务一个 `safeGo` goroutine，全局 `o.mu` 串行闸 + server 任务表；
  多租户 Runner / 中央调度器为**预留未实现**。
- trace id 贯通：`Options.RequestID` 已落地，主去重路径端到端一致（见 §5 已知断链点）。
- 多租户：云端模式按 sub 做热词库/数据隔离（`server.cloud`），Runner 级多租户为预留。

> 本节只描述目标与差距，文档层面不改变代码实现。

---

## 12. 与 `~/voxsign` 的关系

本仓库（voicesign-harness）是 VoiceSign 的**编排/可观测/校验骨架（harness）**：
承担 pipeline 编排、轨迹可观测、space/risk/verify 安全门禁、ASR 线B 个性化服务与 e2e 契约。
`~/voxsign` 是上游产品/客户端侧仓库；本 harness 通过 `contracts/intent-v1.schema.json` 与之对齐意图契约，
经 HTTP（线A）接收指令、以四行回执返回结果。两者不互相 import，仅以契约 + HTTP 解耦。

---

## 13. 签署与用例

`eval/cases/cases.jsonl` 为验收用例集（17 条），签署状态见 `eval/cases/RATIFICATION-REQUEST-2026-10-05.md`。
