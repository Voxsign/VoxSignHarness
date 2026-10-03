# VoiceSign个性化ASR后台需求说明书v2-实现计划

> 本文件由 VoiceSign Harness 多步编排（ORCHESTRATE kind=implement：读附件→生成实现计划→写→提交）自动生成。

## 实现目标

- 产品：VoiceSign个性化ASR后台需求说明书v2
- 依据：用户提供的需求文档（见文末全文摘录）

## 需求要点（确定性提取）

- **请求示例（/v1/process）**
- **响应示例**

## 建议模块划分（骨架，待实现时细化）

| 模块 | 职责 | 关键接口 |
|---|---|---|
| cmd/ 入口 | 服务装配与启动 | main() |
| server/ 路由 | HTTP 端点与鉴权 | /v1/* |
| 领域逻辑 | 需求核心机制（意图/词典/反馈） | 领域服务方法 |
| 数据层 | 持久化（文件/DB） | 读写接口 |

## 验收映射（以需求文档验收标准为准）

> 由实现阶段逐条对照需求文档验收标准展开，每达成一条回填证据。

## 实施步骤（骨架）

1. 解析需求文档，抽取模块与接口契约
2. 搭服务骨架（路由/配置/数据目录）
3. 实现核心机制，逐模块真跑验证
4. 对照验收标准逐条复核，补证据
5. 交付（含自测与验收报告）

## 需求文档全文摘录

```markdown
VoiceSign 个性化 ASR 后台 · 需求说明书 v2

  @page { size: A4; margin: 16mm 15mm 18mm 15mm; }
  * { box-sizing: border-box; }
  body {
    font-family: "PingFang SC", "Hiragino Sans GB", "Heiti SC", "Microsoft YaHei", sans-serif;
    font-size: 11pt; line-height: 1.62; color: #1f2937; margin: 0; padding: 0;
  }
  .cover { text-align: left; padding: 10mm 0 6mm 0; border-bottom: 3px solid #1f3a5f; margin-bottom: 8mm; }
  .cover .doc-title { font-size: 24pt; font-weight: 700; color: #1f3a5f; margin: 0 0 4mm 0; line-height: 1.3; }
  .cover .doc-sub { font-size: 12pt; color: #475569; margin: 0 0 6mm 0; }
  .cover table.meta { border-collapse: collapse; width: 100%; font-size: 10pt; }
  .cover table.meta td { padding: 1.2mm 2mm; border: none; }
  .cover table.meta td.k { width: 22mm; color: #64748b; font-weight: 600; }
  h1 { font-size: 16pt; color: #1f3a5f; border-left: 4px solid #1f3a5f; padding-left: 3mm; margin: 8mm 0 3.5mm 0; page-break-after: avoid; }
  h2 { font-size: 12.5pt; color: #1f3a5f; margin: 5.5mm 0 2.5mm 0; page-break-after: avoid; }
  h3 { font-size: 11pt; color: #334155; margin: 4mm 0 2mm 0; page-break-after: avoid; }
  p { margin: 1.5mm 0; }
  ul, ol { margin: 1.5mm 0 1.5mm 0; padding-left: 6mm; }
  li { margin: 0.8mm 0; }
  table { border-collapse: collapse; width: 100%; margin: 2.5mm 0; font-size: 9.5pt; page-break-inside: avoid; }
  th { background: #eef2f7; color: #1f3a5f; font-weight: 600; }
  th, td { border: 1px solid #cbd5e1; padding: 1.6mm 2.2mm; text-align: left; vertical-align: top; }
  code { font-family: "SF Mono", Menlo, Consolas, monospace; font-size: 9pt; background: #f1f5f9; padding: 0 1mm; border-radius: 2px; color: #0f4c81; }
  pre { background: #0f172a; color: #e2e8f0; padding: 4mm 5mm; border-radius: 3px; font-size: 9pt; line-height: 1.5; overflow-wrap: break-word; white-space: pre-wrap; page-break-inside: avoid; }
  pre code { background: none; color: inherit; padding: 0; }
  .callout { background: #f0f7ff; border: 1px solid #bcd6f5; border-left: 4px solid #1f6feb; padding: 3mm 4mm; margin: 3mm 0; border-radius: 2px; }
  .warn { background: #fff8e6; border: 1px solid #f0d9a8; border-left: 4px solid #e8a13a; padding: 3mm 4mm; margin: 3mm 0; border-radius: 2px; }
  .tag { display: inline-block; font-size: 8.5pt; padding: 0.3mm 2mm; border-radius: 2px; margin-right: 1.5mm; font-weight: 600; }
  .tag.p0 { background: #fde2e2; color: #b91c1c; }
  .tag.p1 { background: #fff0d9; color: #b45309; }
  .tag.p2 { background: #e0f2fe; color: #0369a1; }
  .tag.p3 { background: #e6e6fa; color: #4338ca; }
  .footer { margin-top: 10mm; padding-top: 3mm; border-top: 1px solid #cbd5e1; font-size: 9pt; color: #94a3b8; }
  .no-break { page-break-inside: avoid; }
  .center { text-align: center; }

  VoiceSign 个性化 ASR 后台需求说明书 v2（Go 独立重构）
  做一套「越来越懂你」的语音输入理解后台 —— 独立服务、Go 实现、持续个性化、面向未来全双工
  
    文档版本v2.0（2026-10-03）文档状态定稿待评审（可再改）
    阅读对象需求方（本人）、开发实施人员配套材料《VoiceSign Harness 全景审阅稿 v1》（飞书文档）
    技术栈Go 单二进制 / 独立服务 / JSON 配置 / 零第三方依赖边界声明本后台与 VoiceSign Harness 解耦，是独立的语音输入理解服务
  

1. 项目定位与核心目标

1.1 一句话定义
VoiceSign 个性化 ASR 后台：一套用 Go 编写的独立后台服务，接收手机端传来的语音转写文本（及原始音频），完成「纠错 → 理解 → 个性化增强 → 输出标准化意图」，并且在使用过程中持续学习用户的口语、专名、习惯与偏好，越用越懂用户。

1.2 与 VoiceSign Harness 的关系（重要修正）

本模块是纯粹独立的后台，不并入 VoiceSign Harness 二进制，也不与其耦合。

ASR 后台 = 语音输入理解层（把用户说的变成机器能执行的意图）；Harness = 任务执行层（把意图变成真实结果）。两层职责不同、迭代节奏不同，必须解耦。
ASR 后台通过标准接口对外输出意图 JSON，Harness 只是它的一个消费方；未来任何语音入口（手机 App、桌面端、其他设备）都可以复用同一个 ASR 后台。
两套代码独立仓库、独立部署、独立数据目录；仅共享一套「意图 JSON 契约」，契约版本化、只增不改。

1.3 北极星目标（不可动摇）

越来越懂你：这是本后台存在的唯一理由。系统在使用中持续沉淀你的口语说法、专有名词、指代习惯、项目背景与偏好，个性化能力逐日增强，不依赖一次性配置。
语音能力很强：识别结果的纠错、标点、意图理解、控制语义（打断/暂停/撤销）、语音文字双通道都要做到可用、可靠，体验对标豆包语音体系。
响应极快：本地规则优先，模型兜底，处理链路微秒到毫秒级，语音说完即出结果，不出现长等待。
面向未来全双工：架构从第一天就为「全双工实时语音交互」留好接口与数据模型（见第 9 章），未来实现边说边理解、随时打断、实时反馈。

2. 核心设计思想：个性化的三层递进
「越来越懂你」不是一句口号，而是三层能力逐层递进、每一层都在沉淀数据、数据反过来增强下一层的闭环：

层级要解决的问题沉淀的个性化数据

  L1 听清（文本纠错层）
  语音转写常有错字、同音字、口语碎片、专名错误。例：「季总」实际是「冀总」；「那个 module」实际是某个项目名。
  自定义词典（口语→标准写法）、纠错经验库、语音置信度标记。

  L2 听懂（语义理解层）
  一句话的意图是什么？「它」指什么？「那个文档」是哪个？结合上下文与历史才能正确理解。
  指代消解缓存（意图/域/权限/指代 四元组）、对话上下文记忆、意�…
```

