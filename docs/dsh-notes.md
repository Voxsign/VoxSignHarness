# DeepSeek Harness (dsh) 只读调研笔记

> 用途：为 VoxSign 设备端 harness（Go 重写，V0 本地闭环）架构文档提供参考。
> 调研方式：`git clone --depth 1 https://github.com/deepseek-ai/deepseek-harness`（浅克隆到 `/tmp/dsh`，**未执行仓库内任何代码、未运行构建**）+ 阅读关键源文件/文档 + GitHub REST API 取仓库元信息。
> 调研时间：2026-10-01。当前 checkout HEAD：`639ed015`（2026-09-29，release-dsh-0.2.0-rc.2 merge）。
> 来源纪律：每条结论标注来源路径/URL；查不到的标"未查证"。

---

## 0. 先纠正一个前提

- **dsh 不是 Go 写的，而是 TypeScript/Node.js**。全仓 `.ts` 约 4648 个、`.tsx` 590 个，`.py` 仅 31 个（Python 是 SDK 客户端 + 一个 runtime wheel，不是 harness 本体）。
  来源：`find` 统计；`https://api.github.com/repos/deepseek-ai/deepseek-harness` 返回 `"language": "TypeScript"`。
- 它是 **everything-is-a-plugin** 架构，构建在 [Cordis](https://github.com/cordiverse/cordiverse/cordis) 框架上（插件向共享 context 注入 service / typed event / 可逆 effect）。
  来源：`README.md`、`docs/architecture.md`（"Cordis" 节）。
- 因此下文的"语言/并发模型"等结论是 **TS/Node 实现**，Go 重写时只能借鉴其**设计与协议**，不能照搬运行时。

---

## 1. 固定 tool contract：系统提示词与工具 schema 的构造、字节稳定性、prompt cache

### 1.1 构造与组织
- 系统提示词由 `packages/core/system-prompt`（`ctx.systemPrompt`）负责，机制是**分节注册 + 一次装配**：
  - 插件注册有序 `PromptSection`（`{name, order, text|fn, interpolate?, complete?}`），按 `order` 升序、再按 code-unit 名排序拼接；`text` 可含 `{{variable}}`，由 `renderPrompt` 插值。
  - 动态上下文用 `PromptContext`（`{name, order, text|fn}`），与静态 section 分离，作为"cache-safe 对应物"。
  - 工具 schema 由 `ctx.systemPrompt.tools(provider)` 注册，每次装配产出 `ToolProviderResult.schemas`。
  来源：`docs/subsystems/system-prompt.md`、`packages/core/system-prompt/src/index.ts`。
- **提示词不是 request 的 `system` 字段，而是作为 `system/message` 会话事件落日志**，经 `deriveMessages()` 投影进模型历史：第一步即使为空也占住 surface 节点 0；渲染文本变化时就地替换。
  来源：`docs/architecture.md`（"Turn flow"/"Session log"）、`packages/core/agent-loop/src/runtime-context.ts`（`SystemPromptProjection`，"The first prompt, even empty, reserves surface node 0"）。

### 1.2 是否字节级稳定 —— 是（明确设计目标）
- PTC 模式下渲染给模型的工具 SDK 段 `renderToolsSdk()` **确定性排序**："tools are emitted in lexicographic name order, so an unchanged tool set produces **byte-identical text across assemblies**"。
  来源：`packages/core/tools/src/ts-types.ts`（`renderToolsSdk` 注释，约 L296-300）。
- 系统提示词同样走"先冻结再发"：每个 attempt 复用同一份渲染产物，重试不重新装配；运行时有不变量"**model-visible means logged**"（模型可见输入必须可从日志重建）。
  来源：`docs/architecture.md`（"Turn flow"、"Session log" 中 "Model-visible means logged"）。

### 1.3 如何利用 prompt cache
- 路由能力分档（provider 自适应）：
  - `SystemPromptUpdate = 'in-history'`：提示词变化时**追加到已缓存历史之后**，而不是改写 message 0，从而保住前缀缓存。
  - `ToolUpdate = 'in-history' | 'addition-only'`：新增工具追加到缓存历史之后、不改写声明；不支持的路由才把文本合并到首个系统节点。
  来源：`packages/llm/llm/src/types.ts` L392-418（`SystemPromptUpdate`/`ToolUpdate`）；`docs/subsystems/system-prompt.md` L44。
- 计费/对账侧：`TokenUsage` 把缓存量单列 `cacheReadTokens` / `cacheWriteTokens`，`inputTokens` 仅指未命中部分（"billed input = sum of the three"）；DeepSeek 的 `prompt_cache_hit_tokens → cacheReadTokens`。
  来源：`packages/llm/llm/src/types.ts` L168-187；`packages/core/agent-loop/tests/request-cache.e2e.ts` L93。
- 附带：DeepSeek 官方适配器的扩展头/扩展体（`dsh_plugin_packages`、`dsh_session_log`）**刻意放在 messages/system/tools 之外**，不污染模型可见前缀。
  来源：`docs/deepseek-llm-api-wire-extensions.md`（开篇"remain outside messages, system prompts, and tool schemas"）。

---

## 2. Append-only 轨迹日志（Session log）

### 2.1 文件组织
- 会话日志是 JSONL append-only 文件，按代次（generation）命名：
  - v0：`session.jsonl[.zstd]`；v1 起：`session.vN.jsonl[.zstd]`（小写代次号）；可 zstd 压缩。
  - 已提交代次路径**永不改名/替换/删除**；写结束时"编码→校验→独占发布"到最终版本名的新文件，源文件保持不动。
  - 当前 writer 版本：`SESSION_FORMAT_VERSION = 4`（`packages/core/session/src/types.ts` L89）；已发布基线 `latestReleasedVersion: 3`（evidenceTag `dsh-v0.1.5-alpha.1`）。
  来源：`docs/architecture.md`（"Session log"）、`docs/session-format-status.md`。
- 后端可替换：`SessionPersistence` 接口（`create/open/stat/list/export`）。

### 2.2 确切字段
- 每个事件 envelope：`{ type, seq, time, data, ignorable?, surfaceOp?, sourceEventSeqs? }`。
  - `seq`：会话内单调递增序号；`time`：Unix 毫秒；`ignorable: true` 表示读者不认识也可安全跳过。
  - `surfaceOp` / `sourceEventSeqs` 仅出现在 surface 事件（`system/message`、`user/message`、`assistant/message`、`tool/result`），由编译器在 `Session.append()` 处强制。
  来源：`packages/core/session/src/types.ts`（`SessionEvent` 联合类型，约 L497-516）。
- 主要事件类型（`SessionEventMap`）：`turn/start{turn}`、`turn/end{turn,reason}`、`step/start{turn,step}`、`step/end`、`system/message{turn,step,message}`、`user/message`、`assistant/message{...}`、`assistant/attempt{turn,step,stream}`（保留失败/重试/取消/流错误的尝试，不计入模型历史）、`tool/call{turn,step,callId,name,arguments}`、`tool/result{...}`、`request/header{...}` 等。
  来源：`packages/core/session/src/types.ts` L281-430。

### 2.3 回放 / 调试
- `deriveMessages()` 从日志投影出模型历史；fork、resume、transcripts、telemetry 全部由这些持久化事件派生。
- 迁移链：`open` 时选最高 canonical 代次、拒绝未来版本、相邻 `vN→vN+1` 各由一个迁移包负责；只读 open 直接用内存结果不发布后继。
- 还有 `ctx.sessionProjections`：注册的 projection 单元对已提交事件做增量 fold，宿主用 `stateOf()` 读单一类型状态、`snapshot()` 批量裁剪客户端视图。
  来源：`docs/architecture.md`（"Session log"/"Projection seam"）、`docs/subsystems/session.md`。

### 2.4 一行真实示例（取自仓库 fixture）
- 头行（会话元数据，逻辑格式 v3）：
  `{"type":"session","version":3,"id":"preview-showcase","createdAt":1787472000000,"cwd":"/dsh/workspace","isSeeded":false,"delegationDepth":0,"agentPreset":"standard"}`
- 首个事件：
  `{"type":"turn/start","data":{"turn":1},"seq":0,"time":1787472000000}`
  来源：`packages/experimental/webworker-runtime/tests/fixtures/vfs-example/home/sessions/--dsh-workspace--/preview-showcase/session.v3.jsonl`。

---

## 3. Code / PTC（模型驱动编排）模式

> PTC = Programmatic Tool Calling。是**可选**能力缝（`ctx.ptcRuntime`），不在 agent-loop 主干里。

### 3.1 模型输出协议
- 模型只面向一个保留工具 **`run_code`**（`RUN_CODE_NAME = 'run_code'`）。它的 schema：
  - 入参：`code`（string，**必填**）= 一个 async 函数的**函数体**（顶层 `await`/`return` 可用；TypeScript erasable syntax，类型注解会被 type-strip）；`description`（string，必填，一句话说明程序做什么）；可选 `timeoutMs`（毫秒，受部署上限封顶）、`sandbox_permissions`（枚举，提权需 `justification` 并走审批）。
  - 另有 Python flavor（`code` 为 async Python 函数体）。
  来源：`packages/core/tools/src/ptc.ts`（L30/L54-111/L323-363）、`packages/core/tools/src/ts-types.ts`（`SDK_INSTRUCTIONS` L250-266）。
- 模型在程序里这样调工具（不是一个个独立 tool_call，而是在一段代码里编排）：
  ```text
  run_code({
    code: "return await tools.bash({ command: 'pwd', description: 'Show current directory' })",
    description: "Show current directory"
  })
  ```
  程序内约定（`SDK_PROGRAM_INSTRUCTIONS`）：
  - 工具作为全局 `tools`，`await tools.name(args)` 调用；返回值是该工具的 typed canonical JSON。
  - **失败的工具调用 reject 为 `ToolCallError`（带 `toolName`/`message`）**，用 try/catch 接住可继续。
  - **无依赖的只读调用可 `Promise.all` 并发；有副作用的写调用按提交顺序串行。**
  - 输出靠 `return` 和/或 `console.log`；工具结果里的图片在该 run 之后挂载，供下一步查看；中间结果不入对话历史。
  来源：`packages/core/tools/src/ts-types.ts` L258-283（`SDK_PROGRAM_INSTRUCTIONS` + `renderBashExample`）。

### 3.2 harness 如何执行并聚合回执
- `run_code` 本身走标准工具管线（pre-execute → guards → approval → execute → post-execute → result）；程序内部对子工具的调用是**子调用**：携带父 token、落 `tool/ptc-dispatch` 事件、拒绝作为 binding rejection 抛回程序、且不注入 `additionalContexts` 以保持 call/result 相邻。
  来源：`docs/tool-execution-pipeline.md`（末段）。
- 运行时 seam（`PtcRuntime`）：`resolve(request)` 校验并补齐 cwd/deadline/authority → `run(spec)` 执行；结果为 `PtcRunResult{ sandbox?, value?, logs[], error? }`。**失败是结果上的一个字段，不是 `run()` 的 reject**（报告失败是调用方职责）。
  - `value`：程序顶层 `return` 的值，必须跨过 lossless-JSON 边界；`logs`：捕获的文本，按通道保序。
  - 失败分类（正交、独立上报）：`exception | timeout | abort | worker-exit(OOM) | invalid-output | output-limit | protocol | sandbox-unavailable`。
  来源：`docs/subsystems/ptc-runtime.md`、`packages/ptc-runtime/ptc-runtime/src/types.ts`。
- 已交付执行后端：沙箱化 Node（sandboxed Node runtime，`packages/ptc-runtime/ptc-runtime-node`）；设计决策见 `.agents/notes/implemented/architecture/2026-09-11-sandboxed-node-ptc-runtime.md`。

### 3.3 一次会话内多少轮 LLM↔harness 往返
- 概念：一个 **step** = 一次模型请求 + 它发起的工具调用；一个 **turn** = 零到多个 step。循环持续到"工具不再要求新请求、且没有新的下一步输入"为止（`tools owe another request, or next-step input arrived -> claim -> next step`）。
- **未在 `packages/core/agent-loop`/`agent` 源码中查到硬编码的最大 step/迭代上限**（grep `maxSteps/maxIterations/maxTurns/stepBudget` 均无生产代码命中）——即往返次数由任务自行收敛，上限策略"未查证"（可能在 profile/配置层，本次未深挖）。
  来源：`docs/architecture.md`（"Turn flow"）；grep 结果（见上）。

### 3.4 失败如何处理
- step 失败时 loop **记录缺失的工具结果**（"Failed steps record missing tool results"）；`assistant/attempt` 保留失败/重试/取消/流错误的尝试但不加进模型历史。
- 取消：请求带 live cancellation，`prepareCall`/reconcile 阶段取消则既不提交 system 也不提交 user 消息；PTC 的 `AbortSignal` 会硬停程序（含循环中），返回 `kind:'abort'`，在途 binding 调用由调用方自行 settle。
  来源：`docs/architecture.md`（"Turn flow"、"Session log"）、`docs/subsystems/ptc-runtime.md`。

---

## 4. 整体架构与性能做法

- **主要语言**：TypeScript（Node.js 服务端 + 浏览器 Web 客户端 + Electron desktop）；Cordis 插件框架；pnpm monorepo（`packages/` ~60 个包，`apps/{cli,web,desktop,desktop-host}`）；构建用 tsdown/tsx，测试用 vitest。
  来源：`README.md`、`package.json`、`docs/architecture.md`。
- **组件划分**：以 profile（`web`/`headless`/`sdk`/`sdk-minimal`/`acp`）+ bundle 分层组合插件树；`dsh-base` 是共享第一层（模型适配、工具、持久化、沙箱与审批、设置、凭证、遥测）。核心包：`core/session`（append-only 日志）、`core/system-prompt`、`core/tools`（scoped 工具注册表 + 守卫执行管线）、`core/agent`/`core/agent-loop`、`llm/llm`、`ptc-runtime`。
  来源：`docs/architecture.md`（"Profiles and bundles"/"Core packages" 表）。
- **并发模型**：Node 单线程 event-loop + async/await；工具执行是三段 **waterfall**（`tools/pre-execute` → 单调 guards/approval → `tools/execute`（around: 超时/重试/metrics）→ 工具体 → `tools/post-execute`）。PTC 程序内只读子调用可 `Promise.all` 并发、写调用串行。事件分三类：durable session event、live `agent/*`、capability `fs|tools|telemetry/*`。
  来源：`docs/tool-execution-pipeline.md`、`docs/architecture.md`（"Events"/"Turn flow"）、`packages/core/tools/src/ts-types.ts`。
- **超时与工具执行**：`tools/execute` 外层包装超时/重试；PTC 程序 `timeoutMs` 可由调用方给、被部署默认值/最大值封顶，`null` 表示不设死线；子进程/PTY/LSP 都通过同一个执行世界（`ctx.subprocess` / `ctx.shell` / `ctx.terminals`），可用 `ctx.sandbox` 后端把整组执行搬到远端沙箱。
  来源：`docs/subsystems/ptc-runtime.md`（`PtcRunRequest.timeoutMs`）、`docs/tool-execution-pipeline.md`、`docs/architecture.md`（"Capability seams"）。

---

## 5. 仓库事实

| 项 | 值 | 来源 |
|---|---|---|
| URL | https://github.com/deepseek-ai/deepseek-harness | `README.md`、clone 来源 |
| 许可证 | MIT（Copyright (c) 2026 DeepSeek） | `LICENSE`、GitHub API `"license":{"spdx_id":"MIT"}` |
| 主要语言 | TypeScript（GitHub API `"language":"TypeScript"`；.py 仅 SDK 客户端） | GitHub API、本地 `find` 统计 |
| 创建时间 | 2026-08-13 | GitHub API `created_at` |
| 最近 push / 提交 | 2026-09-29（HEAD `639ed015`，2026-09-29 17:21 +0800） | GitHub API `pushed_at`；`git log -1` |
| star 量级 | **约 24.0 万**（`stargazers_count: 240022`）；fork 约 2.88 万（`28832`）；commits 20,470 | GitHub API / 仓库页（2026-10-01 实测，动态值） |
| 是否可本地构建 | 是（文档化路径）：装 Node.js + pnpm → `pnpm install && pnpm run build && pnpm dsh web`；或直接 `npx @deepseek-ai/dsh web`。**本次未实际构建**（遵只读约束） | `README.md`、根 `package.json`（`build`/`dsh`/`dev:web` 脚本） |
| 状态 | developer preview，官方明示会有 breaking change | `README.md`、`SAFETY.md` |

---

## 附：对 VoxSign（Go 重写）最有借鉴价值的 3 点

1. **字节级稳定前缀 = prompt cache 友好**：工具声明按字典序确定性渲染、系统提示词作为稳定首节点、变化时"追加在缓存历史之后"而非改写首部。Go 实现时应把"工具列表序列化顺序固定 + 系统提示词稳定在前"作为硬约束。
2. **append-only JSONL 事件日志即唯一事实源**：`{type, seq, time, data, ignorable?}` envelope，模型历史由日志投影（`deriveMessages`）派生，fork/resume/调试全部重放日志。Go 侧可直接套用 envelope 设计与代次化文件名（`session.vN.jsonl[.zst]`）。
3. **PTC：一个 `run_code` 工具 + 程序内 `tools.*` 绑定**，而非把每个能力都做成独立 tool_call；失败走"结果字段 + 8 类正交错误"而非异常；只读并发/写串行。V0 单用户本地闭环可先不上 PTC，但事件日志与稳定前缀这两条建议从第一天就遵守。
