# VoiceSign-Harness-全景开发文档

> 本文件由 VoiceSign Harness 多步编排（ORCHESTRATE：读→汇总→写→提交）自动生成。

## 概览

本文件由多步编排自动汇总，纳入以下 5 份源文档：

- SPEC-v2-可执行规格书.md
- 详细设计-语音驱动开发-v2定稿-20261002.md
- 异常自愈架构-问题定位模型.md
- M7配置指南.md
- 全会话记录.md

## 各文档

> 注：本次未调用 LLM 润色（模型不可用），内容为源文档结构化拼接。

## 全文摘录

```markdown
# 源文档: SPEC-v2-可执行规格书.md
# VoxSign Harness · SPEC v2（可执行规格书）

- **版本：v2.0 · 2026-10-02 · 分片 A 产出（定义层，不写生产代码）**
- 取代：`docs/SPEC-v1-可执行规格书.md`（v1 底稿保留作历史；本文件为权威）
- 上游契约：`docs/INTERFACE-FREEZE-M2.md`（API 形状冻结，本文件只定义语义，不改签名）
- 共识依据：详细设计 v2 / VSL-v2 判断语义层 / 五原则 / BOUNDARY-AUDIT 方法论
- 北极星：**认知闭环周期** record → discuss → control → 归因，转一圈的速度（#45/#46 见下）

## 0. 失效机制（五原则④活的规范）

```json
{
  "policy": {
    "policy_version": 2,
    "spec": "SPEC-v2-可执行规格书.md",
    "spec_version": "v2.0",
    "invalidates": {
      "on_policy_change": true,
      "on_space_registry_bump": true,
      "on_contract_bump": true
    }
  }
}
```
人话外壳：**策略版本号一变，四元组缓存全失效、space 注册表与工具契约版本联动 bump；任何策略/域/契约改动即 `cache.BumpPolicyVersion()`，不再信旧缓存。**

裁决记录：
- **YAML→JSON（零依赖）**：manifest/contract/policy 一律 `.space.json` / `.contract.json`，不用 YAML——标准库无 YAML 解析器，引第三方依赖违背「单二进制、零第三方依赖」冻结条款（freeze §2）。v1 里的 YAML 示例仅为示意，机器形态以此处 JSON 为准。

---

## 1. 54 条缺口逐条回应表（research-inputs/16）

裁定取值：**采纳补定义** / **已在冻结/M1 实现** / **降级 P2** / **UNKNOWN**。
验证器列：指向具体 `go test` 函数或可复现命令；无验证器标「未完成」（五原则③）。

| # | 级 | 缺口（摘要） | 裁定 | 约束式定义（禁止/必须验证/完成） | 可执行验证器 | 落地位置 |
|---|---|---|---|---|---|---|
| 1 | P0 | M2 交付边界（12卡/六工具/闭环哪些必交） | 已在冻结/M1 实现 | 分片边界见 freeze §1；A=docs+data+input测试，B=space/refer/risk，C=verify/search/cache/tools，D=pipeline/server。禁止越界改他人目录 | `go test ./...` 全绿即边界自检 | freeze §1 |
| 2 | P0 | M1 仓库位置/构建/测试基线 | 已在冻结/M1 实现 | 模块 `voicesign-harness`（无域前缀）；`go test ./...` 必须全绿；分类器基准 `BenchmarkPipelineProcess < 9.9µs` | `go test ./... && go test ./bench -run xxx -bench BenchmarkPipelineProcess` | go.mod / bench |
| 3 | P0 | M1 各包接口与数据结构兼容 | 已在冻结/M1 实现 | contract 为唯一共享契约；M2 新增字段全部 `omitempty`，M1 类别（TIME/FILE_*）与 M2 类别（EDIT…）并存，M2 管线只用 TaskClassifier 产出 | `go test ./contract ./input` | contract/contract.go |
| 4 | P0 | record→discuss→control 状态机/discuss IO/存储 | 采纳补定义 | 三通道=轨迹 JSONL append-only 的三个 stage：record(input_raw/纠错/意图)、discuss(归因/决策理由/回写)、control(执行/校验/回执)。discuss 输出=一条 `kind=attribution` 轨迹 + 一条 `discuss-log`；**禁止** discuss 自动改策略/词典（须人确认） | `pipeline.TestPipelineWritesAttribution`（实测 PASS） | trajectory/ + pipeline |
| 5 | P0 | 执行链先后关系 | 采纳补定义 | 必须按 freeze §3 pipeline.Run 十三步序：input_raw→clean→dict纠错→TaskClassify→refer消解→space select+Check→risk分级→确认→tools执行→verify→归因+轨迹→cache四元组→四行回执。**禁止** 执行在 space_check/risk 之前 | `pipeline.TestPipelineOrdering`（实测 PASS，串行闸） | pipeline |
| 6 | P0 | 执行主体（EDIT/DEBUG 谁做） | 已在冻结/M1 实现 | 用户拍板：模型工具循环 + 外部编码代理均可；harness 只做 gate（space_check+risk）与独立校验，不自己生成代码 | `tools.Executor.Exec` 按 caps 执行（C） | tools/ |
| 7 | P0 | 正式意图 Schema（类型/必填/枚举/默认/非法） | 采纳补定义 | 见 §1.1 �…(截断)

# 源文档: 详细设计-语音驱动开发-v2定稿-20261002.md
# VoxSign Harness 详细设计 v2（定稿）——语音驱动开发（Voice-Driven Development）

- 日期：2026-10-02 · v1 草案经 Claude Code（claude-sonnet-5）尖锐评审后修订
- 评审要点：Claude 最大反对意见（影响面不能靠模型自评）已吸收为架构性修订
- 定位：以写代码为核心场景的生产底座闭环；本稿是 M2 的施工图

---

## 0. 一句话
**用嘴说变更意图，Harness 完成 容错→意图→规划→风险分级→执行→独立校验→回执→沉淀，下次更懂你。**

## 1. 与传统开发方式的本质差异

| 维度 | 传统开发（键盘+IDE） | 语音驱动开发（VDD） |
|---|---|---|
| 输入 | 精确的键鼠/命令 | 模糊语音（ASR 错误、口语、省略） |
| 输出 | 代码文件 | 意图 JSON + 执行 + 回执（可审计） |
| 迭代 | 手写→运行→看错→改 | 对话式 REPL：说→做→回执→再说 |
| 验证 | 人看/人跑 | **独立校验器**（读实际状态，不读自报） |
| 沉淀 | 注释/文档（人维护） | 轨迹 JSONL + 归因六格 + 认知层（自动） |
| 记忆 | 人的脑子 | 三元组缓存 + project-map + decisions-log |
| 代码量 | 人写全部 | **人描述意图，模型生成，人验收** |

**核心变化**：开发 = 意图描述 + 模型生成 + 契约执行 + 独立验证 + 自动沉淀。

## 2. 开发意图分类（8 类，映射契约）

| 意图 | 触发示例 | 风险基线 | 契约 |
|---|---|---|---|
| EDIT 改代码 | "把报价模块错误提示改中文" | 可逆/影响中 | file+git+test+verify |
| DEBUG 查bug | "修一下这个崩溃" | 可逆/影响高 | git+test+run+verify |
| QUERY 查代码 | "这个函数在哪定义" | 只读 | search+file+git |
| TEST 跑测试 | "跑一下测试" | 只读 | test |
| COMMIT 提交 | "提交这批改动" | **不可逆** | git |
| DE…
```

