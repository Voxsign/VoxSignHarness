# 参照系：DeepSeek Harness（dsh） vs VoiceSign Harness（vhs）

> 建立日期：2026-10-03（Peter 指示：读 DeepSeek harness 开源源码，与 vhs 实现逻辑做差异化对照，形成参照系，避免从头手写）
> 证据纪律：每条对照结论必须带**双方源码证据（文件:行）**；dsh 侧引用路径相对 `/Users/zouyongming/deepseek-harness/`，vhs 侧相对 `/Users/zouyongming/voicesign-harness/`。

## 1. 研究对象与证据源

| 侧 | 仓库 | 读取的源码/文档 | 覆盖能力 |
|---|---|---|---|
| **dsh（DeepSeek）** | github.com/deepseek-ai/deepseek-harness（本地 HEAD 99f6f02 = rc.7） | docs/architecture.zh.md；docs/subsystems/{goal,approval,invariants,session}.zh.md；packages/goal/goal-round-driver/src/{index.ts,prompt.ts,invariant.ts}；packages/plan/plan-mode/README.zh.md；packages/workflow/README.zh.md | 长程任务引擎（goal-round）、审批、运行时不变量、会话日志、规划模式、动态工作流 |
| **vhs（VoiceSign）** | github.com/smithpeter/voicesign-harness（本地 main ee924f4 + 4 处未提交修复） | server/server.go:517 spawnTask / runPipeline；pipeline/pipeline.go（execOrchestrate / llmGenerateImplement）；input/taskintent.go；config/config.go；data/20-tasks.jsonl | 任务状态机、意图识别、确认桥、确定性骨架 + LLM 语义实现、JSONL 落盘 |

## 2. 全景对照表

| 维度 | dsh 实现（证据） | vhs 现状（证据） | 差距判定 |
|---|---|---|---|
| **长程任务载体** | goal：持久目标状态（active/paused/blocked/complete + maxGoalRounds），goal/change 事件 + 严格回放（docs/subsystems/goal.zh.md:8-55） | taskState：ID/RequestID/Text/Document/Status/Role/startedAt/confirmCh + ConvID（server.go:517-524） | vhs 无"目标/轮次/阻塞"语义，任务一跑到底 |
| **"一轮接一轮直到完成"** | goal-round-driver：idle 检查点 → 预留 round → 排队 → 领取 → 准入（claimed/admitted），陈旧预留被拒（index.ts:137-205, 333-347） | runPipeline 单次 goroutine 跑完即 done（server.go:573-600）；无续跑/轮次概念 | **核心差距**：dsh 是"多轮逼近目标"，vhs 是"单发任务" |
| **"没完成不让成功"** | 每 Round 提示词显式要求：完成前收集证据、读当前 goal 并标记 complete，未完成保持 active（prompt.ts:12-26）；round-limit/prompt-rejected → block（index.ts:166-171, 388-398） | done 即成功；产物是骨架也照常 done（L-01 三跑实证：LLM 超时 → 骨架兜底仍 done） | **核心差距**：vhs 缺"证据门"与"未完成则续跑/阻塞" |
| **轮次上限** | maxGoalRounds（目标定义字段）+ 驱动器 round-limit block（goal.zh.md:54；index.ts:166-171） | 无（只有澄清轮次 maxAskRounds，server.go:596-601） | vhs 应引入 maxRounds（用户口径：最多 20 轮/30 分钟一轮） |
| **审批/确认** | approval seam：allowed-once 一次性授权、never 策略、approval/asked+decided 审计对、unavailable 失败闭合（approval.zh.md:21-47, 84-88） | ConfirmFn 确认桥：need_confirm → answer("执行") → 继续（server.go:551-570, 984）；确认粒度=整任务一次 | vhs 粒度粗（任务级一次），dsh 粒度细（工具调用级 allowed-once）；vhs 无审计事件对 |
| **轨迹留痕** | session：类型化 SessionEvent 仅追加日志为**唯一真源**，模型历史从日志派生，"模型可见即已记录"运行时不变量（session.zh.md:5, 34-88） | data/*.jsonl 落盘（20-tasks.jsonl）+ emitEvent（server.go:561）；任务结果落 ts.Outcome | vhs 是"结果落盘"不是"事件溯源"；无法从日志重建模型所见 |
| **运行时不变量** | invariants：包自有不变式注册表，违规抛 INVARIANT 错误（invariants.zh.md:5, 51-54） | scripts/gate.sh 门禁（A21/B0）+ 时序扫描（B 类 21→0） | 概念等价（门禁=不变式），但 vhs 是外部脚本非运行时自检 |
| **真装配** | seam 三角（Service Definition/Provider/Consumer）+ sandbox 后端 + shell 本地/远程后端（architecture.zh.md:104-106） | 本地 shell 执行（runCmd）+ 真编译验证（llmGenerateImplement go build -C） | vhs 已践行"真装配不许桩"（咬过 5 次的洞），与 dsh sandbox 理念一致 |
| **架构哲学** | Cordis 插件树：无特权内核，每部分是插件、注册副作用可逆、可配置替换（architecture.zh.md:11-13） | 单体 Go：server/pipeline/input/config 分层（职责清晰） | 哲学不同但**不是缺陷**——vhs 是轻量单体，面向语音网关场景 |

## 3. 逐维度对照详情（关键差距 × 借鉴建议）

### 3.1 核心差距：dsh 的 Goal Round 引擎 vs vhs 的单发任务

**dsh 的机制（源码实证）**：
1. 目标（objective + maxGoalRounds）持久化为 goal/change 事件，回放重建（goal/src/types.ts 经 docs/subsystems/goal.zh.md:74-100）
2. agent idle 时驱动器检查：active+armed+未超上限 → 预留 roundsStarted+1（index.ts:164-190）
3. 排入 `<goal_round>` 提示词（含 Objective + Round N/max），要求**完成前收集证据、读当前 goal、标记 complete；未完成保持 active**（prompt.ts:15-23）
4. 竞态防护：checkpoint flush 后重查 revision；陈旧预留拒绝且不消耗轮次（index.ts:111-114, 333-347）
5. 上限 → `block('round-limit')`；提示词被拒 → `block('prompt-rejected')`（index.ts:166-171, 388-398）
6. max-tokens 结束 → disarm（不消耗轮次）；aborted → 分情况 cancel/disarm（index.ts:317-327）

**vhs 的现状（源码实证）**：
- spawnTask → runPipeline 单 goroutine → 三种终态：err → canceled；Ask 不收敛 → canceled；否则 done（server.go:573-615）
- **无轮次、无续跑、无目标、无上限**——L-01 三跑实证：LLM 超时 → 骨架兜底 → 仍 done（产物是骨架也"成功"）

**借鉴建议（L-01 直接落地）**：
1. taskState 增 `MaxRounds int`（默认如 20）+ `RoundsUsed int`；runPipeline 末尾检查产物质量判据（可编译+语义证据），**未达标 → 不 done，进入下一轮**（重新生成/修复）
2. 每轮 prompt 显式携带 `Round N/Max` + "完成前收集证据（真编译+需求↔实现逐条对照）、未完成保持 active"
3. 超限 → 明确 blocked（不是 done）——**"没把 harness 造出来之前不让验收成功"**
4. need_confirm 审计：记录 asked/decided 事件对（对齐 dsh approval 审计）

### 3.2 次差距：会话日志 vs 结果落盘

- dsh：SessionEvent 仅追加日志，模型历史/plan 状态/goal 状态全从日志派生，重启/fork/回放一致（session.zh.md:5, 89-120）
- vhs：ts.Outcome + data/*.jsonl（结果级），emitEvent 只发 need_confirm 通知（server.go:561）；**无法从磁盘重建一次任务的完整执行轨迹**

**借鉴**：L-01 验收要求的"轨迹留痕"（需求文档 §8 traces/usage JSONL）已由产品侧覆盖；harness 侧可把任务生命周期事件（submit/confirm/round N/evidence/build/complete/blocked）追加进任务日志，作为"怎么证明达到目标"的证据源。

### 3.3 已对齐项（确认 vhs 方向正确，不必照抄 dsh）

| 项 | 结论 |
|---|---|
| 真装配不许桩 | vhs 已践行（runCmd 真实执行 + go build 真编译），与 dsh sandbox/shell seam 理念一致；**保持** |
| 确认桥（need_confirm） | 语义等价 dsh approval 的 ask 策略；**保持**，补审计对 |
| 门禁 | gate.sh 等价 dsh invariants 理念；**保持** |
| 意图识别 | dsh 无对应物（它是通用 harness，vhs 有语音场景意图分类）——**vhs 独有优势，保持** |

## 4. L-01 直接可用的 5 条改造（按优先级）

1. **【P0】多轮逼近目标**：taskState 增 MaxRounds/RoundsUsed；pipeline 执行后产物判据未达标 → 下一轮（LLM 修复/重新生成），不 done（对应 dsh round-driver 核心）
2. **【P0】"完成前收集证据"提示词**：goal round 式系统提示（Objective + Round N/Max + 证据要求 + 未完成保持 active）——对应 dsh prompt.ts 逐字可借鉴
3. **【P1】上限 → blocked 语义**：超 maxRounds → status=blocked（reason 携带 round-limit），杜绝"骨架也 done"（对应 dsh index.ts:166-171）
4. **【P1】确认审计对**：need_confirm 记录 asked/decided 事件（对齐 dsh approval.zh.md 审计）
5. **【P2】任务事件日志**：submit/round/evidence/build/complete 追加为任务级事件流（对齐 dsh session 日志理念，不引入全套事件溯源）

## 5. 下一步

- 按 §4 改造 vhs（P0 两条优先）→ 重跑 L-01（8766 端口）→ 门禁 → 对照表更新
- 待议：vhs 是否引入轻量"目标"概念（objective 持久化）还是保持"任务=单次目标"；若引则对齐 dsh GoalPhase（active/paused/blocked/complete）

## 6. 读码边界（诚实声明）

- 本参照系**未读**：dsh 的 agent-loop/agent 核心源码（packages/core/agent-loop）、llm-streaming、sandbox 实现细节、compaction/persistence——如需对照"轮次循环内部"或"持久化机制"需再读
- dsh 侧证据来自**源码 + 生成式子系统文档**（docs/subsystems/*.zh.md 是源码生成的 cordis-surface，可信）；vhs 侧证据来自**真读源码**（server.go/pipeline.go/config.go/taskintent.go）
