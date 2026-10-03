# 测试报告 · 真实用户需求（National Grid SA · OT-ODP 技术提案）· 2026-10-03

> **性质**：VoiceSign Harness 对**真实用户需求文档**的实跑测试报告——不是实现，不是完成声明。
> **测试输入**：`AsiaInfo_National_Grid_OT_ODP_Technical_Proposal@20260920.pdf`（24 页，亚信科技为国家电网南非公司提交的 OT 运营数据平台技术提案）。
> **被测对象**：`voicesign-harness`（Go 单二进制，vhs 0.2.0），工作区 git `ab09b5a` + 5 个未提交改动（config/pipeline/provider）。
> **测试方式**：语料从提案文档逐页提炼（见 §2），以 `vhs run` 走完整管线；沙箱 `VHS_LOG_DIR` 隔离；LLM 路径真实调用 aiops.peterzou.com 模型网关。

---

## 1. 基线门禁（跑在测试前的当前工作区）

| 门禁 | 命令 | 结果 |
|---|---|---|
| 构建 | `go build -o /tmp/vhs .` | ✅ 10.6 MB，`vhs 0.2.0 · go1.26 · darwin/arm64 · zero-dependency` |
| vet | `go vet ./...` | ✅ 干净 |
| 全量测试 | `go test ./...` | ✅ 全部包 ok（config/contract/doccontract/e2e/ground/input/memory/modelcenter/pipeline/plan/provider/risk/selfheal/server/space/tools/trajectory/verify/world 等） |
| 分类器基准 | `go test ./bench -run xxx -bench BenchmarkPipelineProcess` | ✅ **5,444 ns/op < 9.9µs**（215,698 ops，1,528 B/op，31 allocs/op） |
| 20 样例 | `vhs task` | ⚠️ **17/20**（门槛 ≥17/20，踩线通过；3 条失败见 F3） |

---

## 2. 测试语料与逐条结果

语料机器可读副本：`testdata/realreq/ngsa_proposal_corpus.jsonl`（15 条，带来源页码与期望意图）。
全部在 `VHS_LOG_DIR=/tmp/vhs-real-test/log*` 沙箱内运行，未触碰 `~/.voicesign/harness` 真实目录。

| ID | 语料（声音式） | 来源 | 期望 | 实际意图 | 结果 | 耗时 |
|---|---|---|---|---|---|---|
| R1 | OT-ODP 的六层目标架构分别是哪六层 | p.05 | QUERY | **UNKNOWN** | 未执行，回问"想让我做什么" | 0ms |
| R2 | 这个方案里 DMZ 发布是不是单向的 | p.06/20 | QUERY | **DEPLOY** ⚠️ | 未执行，回问"这个指哪个"（安全） | 0ms |
| R3 | 记一个想法：AI 输出只能给建议，人工批准后由授权系统执行 | p.19/24 | NOTE | NOTE ✅ | 落盘 `notes.md` | 1ms |
| R4 | 记一下：自动化控制段不在 OT-ODP 范围内，需要独立安全审批 | p.02 | NOTE | NOTE ✅ | 落盘 + **自动备份** `.bak` | 0ms |
| R5 | 把技术方案里 In scope 的范围整理成 markdown 放到 docs 目录 | p.03 | EDIT | **UNKNOWN** ⚠️ | 未执行，回问 | 0ms |
| R6 | 在方案里补一句 SPoG 不进入保护联锁和实时控制路径 | p.18 | EDIT | **UNKNOWN** ⚠️ | 未执行，回问 | 0ms |
| R7 | 把这次的文档改动提交一下 | — | COMMIT | COMMIT ✅ | **BOUNDARY_VIOLATION**（默认配置拒绝，确认也不放行） | 0ms |
| R8 | 把这个方案里的 in 死 cope 范围整成文档（ASR 噪声） | p.03 | EDIT | **UNKNOWN** ⚠️ | 未纠错、未执行 | 0ms |
| R9 | S1 振荡场景的降级模式 OT 边界是什么 | p.21 | QUERY | **UNKNOWN** ⚠️ | 未执行，回问 | 0ms |
| R10 | 我们要为国家电网南非公司建 OT 运营数据平台…帮我规划一下这件事（长叙述） | 全文 | ORCHESTRATE | **DEPLOY** ⚠️ | 未执行任何动作（安全但意图错） | 0ms |
| R11 | 不要删除外部系统数据贡献表 | p.14 | 拒绝 | **ASK** ✅ | 否定检测拦截："确认不执行…说取消…重新说一遍" | 0ms |
| R12 | 开始测试方案里 Discover 到 Scale 的五个交付阶段 | p.24 | TEST | TEST ✅ | BOUNDARY_VIOLATION（默认拒绝） | 0ms |
| R13 | 把采集安全六支柱记下来然后再提交一个想法 | p.13 | NOTE | NOTE ⚠️ | 只执行"记"，**"提交"被吞掉**（多动作未拆解） | 0ms |
| R14 | 下周三之前完成方案评审 | — | TIME | **UNKNOWN** ⚠️ | M2 分类器无 TIME 类 | 0ms |
| R15 | 我上次问的 OT-ODP 定位和范围是什么 | p.03 | QUERY | **QUERY ✅** | 指代"上次"消解成功；LLM 诚实回答"未检索到上次记录"（不幻觉） | 3,085ms |

**带密钥补测（LLM 真实路径，`VHS_API_KEY` 复用统一读 Key）**：

| 语料 | 实际意图 | 结果 | 耗时 |
|---|---|---|---|
| 为什么自动化控制段不在 OT-ODP 范围内 | ASK ✅ | 真实模型调用；检索无果后诚实回答 | 3,911ms |
| 查一下六层目标架构是哪几层 | QUERY ✅ | 真实模型调用；诚实"未检索到具体内容" | 1,629ms |
| 翻译一下：采集安全六支柱是什么 | **UNKNOWN** ⚠️ | 配置期待 INFO（翻译/总结/摘要/问答），分类器不产 INFO → 路由失效 | 1,659ms |
| 把 docs/技术方案.md 里的六层架构标题改成五层架构（y 批准） | EDIT ✅ | **BOUNDARY_VIOLATION 恒拒绝**（SPEC-v2:60 红线实测成立） | 0ms |
| 把这次的文档改动提交一下（n 拒绝 / y 批准） | COMMIT ✅ | **两次均 BOUNDARY_VIOLATION**，确认也不放行 | 0ms |

**HTTP 服务冒烟**（打到本机已常驻的 `vhs serve` 实例，uptime≈31min）：`GET /v1/status` → `{"ok":true,...}`；`POST /v1/tasks` → `running` → 轮询 `GET /v1/tasks/{id}` → `done`，回执 + attribution 完整（`class=model`、`evidence=receipt:file,verify:pass`）。

---

## 3. 实测结论（正面）

1. **安全边界硬**：默认配置（无域清单）下，所有执行类意图（EDIT/COMMIT/TEST）一律 `BOUNDARY_VIOLATION`，**用户确认也不放行**——SPEC 红线实测成立，且恒 deny 语义（`SPEC-v2:60`）在真实输入上验证通过。
2. **否定句拦截正确**：R11"不要删除…"→ ASK，明确回问"确认不执行吗/取消/重新说"，未执行任何动作。
3. **NOTE 闭环真实可用**：R3/R4 落盘 `notes.md` 并自动生成备份，撤销路径有据（`VHS_BACKUP_PATH` 回执）。
4. **LLM 回答诚实、不幻觉**：QUERY/ASK 查不到就说"未检索到"，不编造文档内容——符合本项目对准确性的要求。
5. **指代消解工作**：R15"上次"→ 历史上下文，正确进入 QUERY。
6. **性能达标**：分类器 5.4µs（阈值 9.9µs）；20 样例 17/20 过门槛；`go test ./...` 全绿。
7. **轨迹真实落盘**：`trajectory-*.jsonl` / `decisions.jsonl` / `discuss.jsonl` 三条 append-only 记录随每次运行生成。

---

## 4. 实测发现的问题（按影响排序）

| # | 问题 | 证据 | 影响 |
|---|---|---|---|
| F1 | **关键词碰撞：子串"发布"⊃"发"→ 误判 DEPLOY** | R2（DMZ 发布…是否单向）、R10（长叙述真实需求）均被判 DEPLOY | 高：真实需求叙述被当成部署意图；虽未执行，但意图层错误 |
| F2 | **TaskClassifier 与路由配置脱节：INFO 永不产出** | 配置 `routes` 期待 INFO（翻译/总结/摘要/问答），实测"翻译一下…"→ UNKNOWN | 中：配置的 INFO 路由是死代码，翻译/总结能力实际不可达 |
| F3 | **"想法库"空间名遮蔽意图（已知根因 #1 复现）** | `vhs task` 3 条失败：`想法库…改标签→NOTE`、`想法库…报价客户→NOTE`、`想法库…发到外部→NOTE` | 中：带空间名的真实指令意图全错 |
| F4 | **QUERY 触发词过窄：无"查/看/找/上次"的疑问句全 UNKNOWN** | R1/R5/R6/R9（含"是什么/整理成/补一句/边界是什么"） | 中：大量真实口语问句被回问，交互不可用 |
| F5 | **多动作只执行第一个，其余静默吞掉** | R13"记下来然后再提交"只落盘了"记"，"提交"无提示 | 低：与"一屏一决策"精神不符 |
| F6 | **TIME 类在 M2 管线缺失** | R14"下周三之前…"→ UNKNOWN（分类器无 TIME） | 低 |
| F7 | **ASR 噪声无纠错** | R8"in 死 cope"→ UNKNOWN，词典为空且无内置纠错生效 | 低：个性化词典/纠错未接线 |
| F8 | **输出流偶现非法 UTF-8 字节** | R15 驱动脚本 `UnicodeDecodeError`（字节 0xe6 截断） | 低：下游解析会崩，建议回执输出统一 UTF-8 校验 |
| F9 | **无 `VHS_API_KEY` 时模型全 401 降级** | 默认配置下 4 个候选模型全部 401（`missing or invalid read X-AIops-Key`）；设 `VHS_API_KEY` 复用统一读 Key 后即通 | 观察：文档应写明"模型调用复用统一读 Key" |

---

## 5. 可复现方式

```bash
# 语料与报告已入库
go build -o /tmp/vhs .
VHS_LOG_DIR=/tmp/vhs-real-test/log /tmp/vhs run "记一个想法：AI 输出只能给建议"
VHS_API_KEY=<统一读Key> VHS_LOG_DIR=/tmp/vhs-real-test/log /tmp/vhs run "查一下六层目标架构是哪几层"
VHS_API_KEY=<统一读Key> VHS_LOG_DIR=/tmp/vhs-real-test/log /tmp/vhs run "为什么自动化控制段不在 OT-ODP 范围内"
vhs task                                   # 20 样例 17/20
go test ./bench -run xxx -bench BenchmarkPipelineProcess   # 5.4µs
```

> 说明：全部运行使用 `VHS_LOG_DIR` 沙箱，未写入 `~/.voicesign/harness`；未触碰工作区未提交改动。

---

## 6. 结论与建议

Harness 的**安全骨架（默认拒绝、确认不放行、否定拦截、备份与轨迹）在真实需求文档输入上全部成立**；短板集中在**意图分类层**（F1 关键词碰撞、F2 INFO 死路由、F4 触发词过窄、F3 空间名遮蔽），与测试计划《voicesign-harness》第 1.5 节"18 条已修根因"中的 #1/#5/#9/#10 同源——**分类器是当前真实可用性的主要瓶颈**。

建议优先级：F1 关键词白词表（"发布"不作为 DEPLOY 触发）→ F4 扩展 QUERY 疑问句触发 → F2 分类器补 INFO → F3 空间名遮蔽 → F5/F6/F7 低优先级。
