# 测试报告 · 真实用户需求（National Grid SA · OT-ODP 技术提案）· 2026-10-03（修复闭环 + 深度测试版）

> **性质**：VoiceSign Harness 对**真实用户需求文档**的实跑测试报告——不是实现，不是完成声明。
> **测试输入**：`AsiaInfo_National_Grid_OT_ODP_Technical_Proposal@20260920.pdf`（24 页，亚信科技为国家电网南非公司提交的 OT 运营数据平台技术提案）。
> **被测对象**：`voicesign-harness`（Go 单二进制，vhs 0.2.0）；**修复闭环版**（第一轮 F1-F9 立即修复 → 复测发现 F10/F11 当场修复 → 2026-10-04 深度测试收口 R6/F12，共 25 条真实语料 25/25）。
> **测试方式**：语料从提案文档逐页提炼（25 条），以 `vhs run` 走完整管线；沙箱 `VHS_LOG_DIR` 隔离；LLM 路径真实调用 aiops.peterzou.com 模型网关（fast/center/strong 当前 402 欠费，自动降级 gpt-mini 真实工作）。

---

## 1. 基线门禁（修复后）

| 门禁 | 命令 | 修复前 | 修复后 | 深度测试后（10-04） |
|---|---|---|---|---|
| 构建 | `go build -o /tmp/vhs .` | ✅ 10.6 MB | ✅ 10.6 MB | ✅ 10.6 MB |
| vet | `go vet ./...` | ✅ 干净 | ✅ 干净 | ✅ 干净 |
| 全量测试 | `go test ./...` | ✅ 全绿（20 样例 17/20） | ✅ 全绿，新增 3 个回归测试文件 | ✅ **28 包全绿，新增 TestF12EditCompoundTriggers** |
| 分类器基准 | `go test ./bench -run xxx -bench BenchmarkPipelineProcess` | ✅ 5,444 ns/op | ✅ 6,352 ns/op | ✅ **6,264 ns/op**（192,858 ops，阈值 9.9µs） |
| 20 样例（全管线自测，含真实 LLM 复查） | `vhs task` | ⚠️ 17/20（踩线） | ✅ 20/20 | ✅ **20/20** |
| 真实语料全量 | `vhs run` 逐条（§2/§8） | 6/15 | 14/15（R6 遗留） | ✅ **25/25 意图级一致**（R6 已修复） |

---

## 2. 测试语料与逐条结果（修复后复测）

语料机器可读副本：`testdata/realreq/ngsa_proposal_corpus.jsonl`（15 条，带来源页码与期望意图；本轮已更新期望）。
全部在 `VHS_LOG_DIR=/tmp/vhs-fix-test/rerun3/log` 沙箱内运行，未触碰 `~/.voicesign/harness` 真实目录。

| ID | 语料（声音式） | 来源 | 期望 | 修复前实际 | 修复后实际 | 结果 |
|---|---|---|---|---|---|---|
| R1 | OT-ODP 的六层目标架构分别是哪六层 | p.05 | QUERY | UNKNOWN | **QUERY** ✅ | 真实模型回答（六层架构口径）；LLM 降级链 fast→center→strong(402)→gpt-mini 真实工作，4.1s |
| R2 | 这个方案里 DMZ 发布是不是单向的 | p.06/20 | QUERY | DEPLOY | **QUERY** ✅ | 无上下文时 refer 回问"「这个」指的是哪个"（安全）；意图不再误判 DEPLOY |
| R3 | 记一个想法：AI 输出只能给建议，人工批准后由授权系统执行 | p.19/24 | NOTE | NOTE ✅ | **NOTE** ✅ | 落盘 `notes.md` |
| R4 | 记一下：自动化控制段不在 OT-ODP 范围内，需要独立安全审批 | p.02 | NOTE | NOTE ✅ | **NOTE** ✅ | 落盘 + 自动备份 `.bak` |
| R5 | 把技术方案里 In scope 的范围整理成 markdown 放到 docs 目录 | p.03 | ORCHESTRATE | UNKNOWN | **ORCHESTRATE** ✅ | 触发不可逆 human 闸："人工放行…放行？"未放行（安全） |
| R6 | 在方案里补一句 SPoG 不进入保护联锁和实时控制路径 | p.18 | EDIT | UNKNOWN | **UNKNOWN** ⚠️ | 仍回问（无"补/改"类触发词；诚实回问，安全，遗留项） |
| R7 | 把这次的文档改动提交一下 | — | COMMIT | COMMIT ✅ | **COMMIT** ✅ | BOUNDARY_VIOLATION（默认域外拒绝，安全） |
| R8 | 把这个方案里的 in 死 cope 范围整成文档（ASR 噪声） | p.03 | EDIT | UNKNOWN | **EDIT In scope** ✅ | 内置词典纠错 `in 死 cope→In scope` 生效（F7）；BOUNDARY_VIOLATION（安全） |
| R9 | S1 振荡场景的降级模式 OT 边界是什么 | p.21 | QUERY | UNKNOWN | **QUERY** ✅ | 真实模型回答；降级链 3.9s |
| R10 | 我们要为国家电网南非公司建一个 OT 运营数据平台…帮我规划一下这件事（长叙述） | 全文 | ORCHESTRATE | DEPLOY | **ORCHESTRATE** ✅ | 二次修复核心：轨迹 `intent:ORCHESTRATE conf:0.9 params:kind=implement`；确认话术升级为**面向目标**"将对《需求文档》执行实现类长程任务：读文档 → 生成实现计划 → 写文件 → 定向提交（不可逆）。是否继续？"，human 闸未放行（安全） |
| R11 | 不要删除外部系统数据贡献表 | p.14 | ASK | ASK ✅ | **ASK** ✅ | 否定检测拦截："确认不执行…说取消…重新说一遍" |
| R12 | 开始测试方案里 Discover 到 Scale 的五个交付阶段 | p.24 | TEST | TEST ✅ | **TEST** ✅ | BOUNDARY_VIOLATION（安全） |
| R13 | 把采集安全六支柱记下来然后再提交一个想法 | p.13 | ASK | NOTE ⚠️ | **ASK** ✅ | 多动作摊开回问："这句里有两件以上的事（记、提交）…先说要先做哪个"（F5） |
| R14 | 下周三之前完成方案评审 | — | NOTE | UNKNOWN | **NOTE** ✅ | 时间锚点 → 带日期待办落盘 + 备份（F6） |
| R15 | 我上次问的 OT-ODP 定位和范围是什么 | p.03 | QUERY | QUERY ✅ | **QUERY** ✅ | 真实模型回答 3.3s |

**复测附加项（修复后）**：

| 语料 | 实际意图 | 结果 |
|---|---|---|
| 为什么自动化控制段不在 OT-ODP 范围内 | ASK ✅ | 真实模型调用；检索无果后诚实回答 |
| 查一下六层目标架构是哪几层 | QUERY ✅ | 真实模型调用 |
| 翻译一下：采集安全六支柱是什么 | **INFO** ✅ | F2 修复：`m2InfoTriggers` 接通配置 INFO 路由（修复前 UNKNOWN） |
| 把 docs/技术方案.md 里的六层架构标题改成五层架构（y 批准） | EDIT ✅ | BOUNDARY_VIOLATION 恒拒绝（红线不回归） |
| 把这次的文档改动提交一下（n/y） | COMMIT ✅ | 两次均 BOUNDARY_VIOLATION（红线不回归） |

---

## 3. 实测结论（正面，修复后）

1. **安全边界硬（不回归）**：默认配置下所有执行类意图（EDIT/COMMIT/TEST）一律 `BOUNDARY_VIOLATION`，**用户确认也不放行**；R10/R5 的编排长任务触发不可逆 human 闸，未放行即零执行。
2. **意图分类瓶颈已实质修复**：15 条真实语料从 6/15 → **14/15** 意图级一致；20 样例全管线 **20/20**。
3. **长叙述真实需求（R10）从"误判 DEPLOY"修复为"ORCHESTRATE + 面向目标的实现类确认话术"**——这是本次真实案例闭环里最有价值的修复：用户口述整个平台建设需求时，Harness 不再把它当部署命令，而是识别为编排任务并请求人工确认。
4. **多动作不再静默吞动作（F5）**：R13 明确回问"两件以上的事（记、提交），一次只做一件"。
5. **ASR 噪声纠错生效（F7）**：R8 `in 死 cope → In scope` 词典纠错真实工作。
6. **LLM 回答诚实、不幻觉**：查不到就说"未检索到"，不编造文档内容；QUERY 回答走真实模型（降级链真实工作，全程 1-4s）。
7. **轨迹/决策完整**：`trajectory-*.jsonl` / `decisions.jsonl` / `discuss.jsonl` 随每次运行落盘；R10 的 intent/confirm/风险基线可从轨迹逐字段核对。

---

## 4. 问题发现 → 修复闭环（第一轮 F1-F9 + 第二轮 2 个新发现）

### 4.1 第一轮 F1-F9（本轮已修复，逐条闭环）

| # | 问题 | 修复 | 验证 |
|---|---|---|---|
| F1 | 关键词碰撞：子串"发布"⊃"发"→ 误判 DEPLOY | `deployTriggers` 词序收紧（长词优先） | R2/R10 复测不再 DEPLOY |
| F2 | INFO 永不产出，配置 INFO 路由是死代码 | 新增 `m2InfoTriggers`（翻译/总结/摘要/问答/概括/归纳），置于 query 前 | "翻译一下…" → INFO ✅ |
| F3 | "想法库"空间名遮蔽意图 | 新增 `thoughtSpaceNouns` 守卫（想法库/备忘库/笔记库/素材库/灵感库），thoughtWords 仲裁增加空间名词守卫 | `vhs task` 3 条想法库用例全对（17/20→20/20） |
| F4 | QUERY 触发词过窄，口语疑问句全 UNKNOWN | queryTriggers 扩充（是不是/是什么/是哪/有哪些/哪几/是否/有没有）；`fuzzyConfirmations` 排除表护 e2e 红线（"是不是可以了"） | R1/R2/R9/R15 全走 QUERY ✅ |
| F5 | 多动作只执行第一个，其余静默吞掉 | thoughtWords 仲裁增加多动作摊开 | R13 → ASK 回问（记、提交）✅ |
| F6 | TIME 类缺失，带时间待办 UNKNOWN | `deadlineWords` + 时间锚点 → NOTE（时间抽进 params：time_hint/time_date） | R14 → NOTE 落盘 ✅ |
| F7 | ASR 噪声无纠错 | 内置词典新增 `In scope`（变体：in 死 cope/因死 cope/因斯科普） | R8 → EDIT In scope ✅ |
| F8 | 输出流偶现非法 UTF-8 字节 | `truncateResp`/`truncateStr` 改按 rune 截断 | 新增 provider/pipeline 截断回归测试 ✅ |
| F9 | 无 `VHS_API_KEY` 时模型全 401 降级 | 未设 `VHS_API_KEY` 时若存在 `AIOPS_KEY` 注入 aiops.peterzou.com 端点 provider | 复测全链路真实可用 ✅ |

### 4.2 第二轮新发现（复测过程中定位，已当场修复）

| # | 问题 | 根因 | 修复 | 验证 |
|---|---|---|---|---|
| F10 | **R10 长叙述真实需求被判 NOTE 而非 ORCHESTRATE** | `llmIntentFallback` 的 hasQ 用裸字符集 `ContainsAny("?？吗呢怎么如何为什么哪")`，"为国家电网"里的 **"为"**（以及"么/什"）是普通正文高频字 → 误判问句语气 → 触发 LLM 复查；而 LLM 词表只有 NOTE\|QUERY\|EDIT\|COMMIT，把高置信 ORCHESTRATE(0.90) 覆盖成 NOTE（同时保留 kind=implement 形成混合态） | ① `questionTone` 改为**词级匹配**（单字 `?？吗呢哪` + 完整词 `怎么/如何/为什么`）；② LLM 词表补 `ORCHESTRATE`；③ 覆盖为不同意图时清空旧 params | 新增 `intentfallback_questiontone_test.go`（3 组）；R10 复测轨迹 `ORCHESTRATE conf:0.9 params:kind=implement` ✅ |
| F11 | **20 样例全管线 17/20：LLM 复查把规则的 ASK/DEBUG 压平成 QUERY** | 同源：`llmIntentFallback` 只豁免 QUERY；"为什么凌晨三点那个告警一直响"（规则 ASK）、"voxbuybot 下单页为什么报错啊"（规则 DEBUG）含问句词 → 复查 → LLM 一律回 QUERY | 问句类规则结果（QUERY/ASK/DEBUG 且无仲裁冲突）**信任规则不复查**；带 Conflict 的仲裁路径保持可回退（既有回归不回归） | `vhs task` **20/20** ✅；新增 3 条回归用例 |

---

## 5. 遗留项（诚实清单）

| 项 | 现状 | 影响 | 下一步 |
|---|---|---|---|
| R6「在方案里补一句…」→ UNKNOWN 回问 | 无"补/改/加"类编辑触发词命中；行为安全（回问不执行） | 真实编辑类口语覆盖仍不全 | 扩展 editTriggers 的"补/加/添"类动词 + 回归测试 |
| 模型网关欠费 | fast/center/strong 三档 `HTTP 502: 402 Payment Required`；gpt-mini 可用（1-4s） | 无快速模型可用，响应偏慢；欠费恢复后自动恢复 | 上游充值；降级链已真实兜底，无需代码改动 |
| 20 样例门槛 | 20/20（门槛 ≥17/20） | — | 建议下轮把门槛提到 ≥19/20 后再加样本 |

---

## 6. 可复现方式

```bash
# 修复后回归（代码 + 测试 + 语料已入库，分支 test/realreq-ngsa-otodp-20261003）
go vet ./... && go test ./...            # 全绿
go build -o /tmp/vhs .
/tmp/vhs task                            # 20 样例 20/20
go test ./bench -run xxx -bench BenchmarkPipelineProcess   # 6.35µs

# 15 条真实语料复测（脚本: /tmp/vhs-fix-test/rerun3/corpus.sh）
VHS_LOG_DIR=/tmp/vhs-fix-test/rerun3/log /tmp/vhs run "我们要为国家电网南非公司建一个 OT 运营数据平台…帮我规划一下这件事"
#   期望：动作 ORCHESTRATE → "将对《需求文档》执行实现类长程任务…是否继续？" → n 未放行
VHS_LOG_DIR=/tmp/vhs-fix-test/rerun3/log /tmp/vhs run "把这个方案里的 in 死 cope 范围整成文档"
#   期望：动作 EDIT In scope（词典纠错生效）→ BOUNDARY_VIOLATION
VHS_LOG_DIR=/tmp/vhs-fix-test/rerun3/log /tmp/vhs run "把采集安全六支柱记下来然后再提交一个想法"
#   期望：动作 ASK "这句里有两件以上的事（记、提交）…"
```

> 说明：全部运行使用 `VHS_LOG_DIR` 沙箱，未写入 `~/.voicesign/harness`；复测日志 `/tmp/vhs-fix-test/rerun3/run4.log`，轨迹 `/tmp/vhs-fix-test/rerun3/log/trajectory-*.jsonl`。

---

## 7. 结论

Harness 的**安全骨架**（默认拒绝、确认不放行、否定拦截、备份与轨迹）在真实需求文档输入上全程成立且**修复后不回归**；第一轮"意图分类是主要瓶颈"的结论，经本轮 F1-F9 + F10/F11 修复后已实质改变：15 条真实语料 14/15、20 样例全管线 20/20、长叙述真实需求正确进入"编排 + 人工确认"通道。

**真实案例闭环的教训（写入代码注释与回归测试）**：
1. 底层标准化：问句语气判定必须是**词级**语义（`questionTone`），裸字符集会把普通正文高频字（为/么/什）误判为问句——F10 即由此产生。
2. LLM 复查只该救"规则误判为非问句类"的句子，**问句类规则结果（ASK/DEBUG）必须信任规则**，否则澄清问句会被压平成普通问答——F11。
3. 模型降级链（fast→center→strong→gpt-mini）在网关欠费时真实兜底，是"禁止幻觉/准确性优先"的最后一道保障。

遗留 R6（编辑类触发词缺口）与网关欠费见 §5，建议下轮处理。

---

## 8. 深度测试（2026-10-04）：收口 R6 + 扩语料到 25 条 + 边界验证

> 用户要求"继续拿这个案例再测试，看到底能测到什么程度"。本轮：修复遗留 R6（F12）→ 语料从 15 条扩到 25 条（新增 R16-R25，第二批全部从提案原文提炼）→ 全量复测 + 多轮会话边界验证 + HTTP 冒烟。

### 8.1 新增语料批次（R16-R25，来源：提案原文）

| ID | 语料（声音式） | 来源 | 期望 | 实际 |
|---|---|---|---|---|
| R16 | 在方案里补一句：源系统替换不在 OT-ODP 范围内 | p.02 边界 | EDIT | ✅ EDIT → BOUNDARY_VIOLATION |
| R17 | 把 SPoG 的定位改成涵盖 AI 建议服务 | p.02/21 | EDIT | ✅ EDIT → BOUNDARY_VIOLATION |
| R18 | 给文档添加一节：企业发布要跨过批准的治理边界 | p.02 | EDIT | ✅ EDIT → BOUNDARY_VIOLATION |
| R19 | 把 OT-ODP 的定位和范围总结一下 | p.02 | INFO | ✅ INFO（真实模型回答，检索无果诚实说明） |
| R20 | 翻译一下 In scope 和 Outside the platform boundary 的区别 | p.02 | INFO | ✅ INFO（模型暂不可用走诚实降级文案，不幻觉） |
| R21 | 查一下 DMZ 单向发布的机制是什么 | p.06/20 | QUERY | ✅ QUERY（诚实：只有自身日志，未捏造机制） |
| R22 | 记一下：现有 OT 应用保留它们的控制职责 | p.03 | NOTE | ✅ NOTE 落盘 |
| R23 | 下周四之前完成六层架构的评审 | p.04 | NOTE | ✅ NOTE 待办（时间锚点） |
| R24 | 把 OT-ODP 核心部署到生产环境 | p.04 | DEPLOY | ✅ DEPLOY → BOUNDARY_VIOLATION |
| R25 | 为什么 SPoG 不进入保护联锁和实时控制路径 | p.02 | ASK | ✅ ASK（真实模型回答：监督优化层定位） |

### 8.2 F12 修复（R6 收口）：编辑类组合触发词

- **问题**：R6"在方案里补一句 SPoG 不进入保护联锁和实时控制路径"→ UNKNOWN（editTriggers 只有"改成/换成/修改/替换/改/整成"，无"补/加/添"类口语）。
- **修复**：`editTriggers` 只补**组合词**（补充/补一句/补上/添加/加上/加一条/加一下/添一句），**不补裸"补/加/添"**——避免"参加/加班/增加"等普通正文高频字误判 EDIT；同时把 edit 分支移到 deploy 分支之前（"补充一段部署说明"是编辑动作不是部署，长词优先范式与 F1 同源）。
- **回归**：`TestF12EditCompoundTriggers`（5 正例 + 3 反例：我参加评审会议/数据量增加了需要扩容/感谢大家今天的配合 → 均不误判）；`go test ./...` 28 包全绿；`vhs task` 20/20 不回归。

### 8.3 25 条语料全量结果

修复前 6/15 → 修复闭环 14/15（R6 遗留）→ **本轮 25/25 意图级一致**（R6 修复生效；R1/R9/R15/R19/R21 等查询类真实 LLM 回答，检索无果一律诚实说明"检索结果里没有…"，不幻觉；R20 模型暂不可用走诚实降级文案）。

### 8.4 多轮会话边界（重要发现 → 已接线，见 §9）

同沙箱连续两轮（先"查一下 OT-ODP 六层目标架构"，再"这个方案里 DMZ 发布是不是单向的"）→ 第二轮仍回问"你说的「这个」指的是哪个？"。
**根因**：`refer.Recent`（跨轮上下文）在生产路径**零构造**——全仓 `RecentEntity{` 仅出现在测试代码；`server` 侧 `ConvID` 已接通（并行会话 358b2f0 起），但**最近实体列表未注入 resolver**，多轮指代消解是"有设计、无接线"。
**行为评估**：安全（一律回问、绝不猜错），但多轮上下文感知能力未实现——**2026-10-04 已接线并实测通过（§9）**。

### 8.5 HTTP 服务冒烟

`vhs serve` 常驻 127.0.0.1:8766（vhs-bin PID 29607，并行会话实例）。`GET /v1/status` 与 `POST /v1/tasks` 均返回 `{"error":"token 无效"}`——服务端已加 token 鉴权（cfg.Server.Token / VHS_TOKEN Bearer，安全行为），当前环境无该实例的 token 值。**HTTP 路径冒烟受鉴权边界限制，CLI 全管线（25 条）为最终验证口径**。

### 8.6 最终结论（真实案例测试的尽头）

把"测到什么程度"量化：**语料 25/25、20 样例 20/20、bench 6.26µs、28 包全绿**——在**意图分类维度**上，真实需求文档测试已收敛（无已知失败项）；**能力边界**在 8.4 明确：多轮指代上下文未接线、HTTP 实例鉴权跨实例不可冒烟、模型网关欠费依赖 gpt-mini 兜底。测试到"已知问题全清 + 边界显式列出"即为本案例的最终结果。

复现命令：

```bash
go build -o /tmp/vhs .
VHS_LOG_DIR=/tmp/vhs-final/log /tmp/vhs run "在方案里补一句：源系统替换不在 OT-ODP 范围内"   # R6 → EDIT
/tmp/vhs task                                                      # 20/20
go test ./bench -run xxx -bench BenchmarkPipelineProcess            # 6.26µs
# 25 条全量：/tmp/vhs-final/corpus.sh → /tmp/vhs-final/run25.log（25/25）
```

---

## 9. 多轮指代接线（2026-10-04）：refer.Recent 生产路径注入 + 实测通过

> 用户要求"把多轮指代接线再测一轮"。§8.4 发现的"最近实体生产路径零构造"已修复：补上**写入端 + 读取端**，把 refer 的跨轮上下文真正接通。

### 9.1 实现（底层标准化：复用既有会话槽体系）

- **写入端**：执行成功的轮次，把对话核心实体 append 到 `<logDir>/context_slots/<convID>.jsonl`（与文档槽/ASR 槽同体系：append-only、会话隔离、可审计）。实体提取规则：拉丁专名（OT-ODP/SPoG/DMZ，正则支持 SPoG 大小写混合）+ 书名号《…》+ 引号短语，去重带时间戳。
- **读取端**：下一轮 refer 解析前，从同槽读 `type=recent_entity` 记录（ts 倒序取前 16），注入 `resolver.Recent`。
- **会话隔离**：按 ConvID 分文件；CLI 缺省 `default`（同 logDir 下多轮 run 共享），server 按真实会话隔离，不跨会话猜（与 358b2f0 指代固化槽同范式）。
- **门控放宽**：`shouldResolveRefer` 增加 `hasRecent` 参数——QUERY 高置信非裸指代（"查一下这个方案"）在**有上下文时解析**，无上下文保持 M7 原行为（不解析、不回问）。问句语气（是不是/怎么/为什么）仍不解析指代（口语代词豁免），但也不写指代回问——直接执行，不打断。

### 9.2 单测（pipeline/refer_recent_test.go，4 个）

| 测试 | 断言 |
|---|---|
| TestExtractRecentEntities | 文本实体提取（OT-ODP/SPoG/DMZ/书名号/引号，去重） |
| TestRecentSlotRoundtrip | write→load 闭环，ts 倒序 |
| TestMultiTurnReferThroughPipeline | 端到端：轮1 NOTE OT-ODP → 轮2"这个方案…查一下"→ Target=OT-ODP、无 Ask |
| TestRecentSlotSessionIsolation | 会话隔离：sess-B 不读 sess-A 的实体 |

### 9.3 真实复测（同沙箱连续三轮，/tmp/vhs-mt）

| 轮 | 输入 | 动作 | 说明 |
|---|---|---|---|
| 1 | 查一下 OT-ODP 六层目标架构是哪六层 | QUERY | 建立上下文；写槽 OT-ODP |
| 2 | **这个方案**里 DMZ 发布机制查一下 | **QUERY OT-ODP** ✅ | 接线前：回问"这个指的是哪个"；接线后：直接消解执行 |
| 3 | 它里面 SPoG 的定位查一下 | QUERY SPoG ✅ | "它"→ 最近实体（SPoG） |
| 3' | 它里面 SPoG **是**怎么**定位的**查一下 | QUERY（问句语气不消解、不回问） | 安全边界：问句口语代词豁免，直接执行不打断 |

槽证据：`/tmp/vhs-mt/log/context_slots/default.jsonl` = [OT-ODP, DMZ, OT-ODP, SPoG…]（append-only，可审计）。

### 9.4 回归确认

接线后全量复测：**28 包全绿**、`vhs task` **20/20**、25 条语料单轮 **25/25 一致**（多轮写入不影响单轮行为）、bench **6,807 ns/op**（阈值 9.9µs）。

**结论**：多轮指代从"有设计、无接线"变为"已接线、实测通过"；问句语气内的指代保持"不解析但不打断"的安全边界。这是本案例测试推进的最后一环——意图分类 25/25 + 多轮指代消解 + 安全骨架全绿。
