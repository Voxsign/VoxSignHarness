# VoxSign Harness 架构设计文档（V0 评审稿）

- 版本：v0.1（评审稿，供评审后冻结）
- 日期：2026-10-01
- 定位：设备端 harness（执行面）的长期增长型架构设计；V0 范围 = 本地闭环、单用户自用
- 阅读：手机端优先，中文为主

---

## 1. 评审决策点（请优先拍板）

| # | 决策点 | 本稿建议 | 备选 |
|---|--------|----------|------|
| D1 | 输入容错层纠错策略 | 本地词典变体替换（确定性）+ 模型二次纠错（词典注入提示词）双层 | 仅本地 / 仅模型 |
| D2 | 低置信度处理 | 默认「回问」（不执行），可配为「交模型澄清」 | — |
| D3 | 意图分类 | V0 用确定性规则分类（8 类），模型不参与本地判定 | 快模型做意图（更准、更慢、费 token） |
| D4 | 记忆注入位置 | 画像/词典块注入「首条 user 消息前缀」（低频变化、命中前缀缓存） | 注入 system prompt |
| D5 | 目录布局 | 顶层平铺包（无 internal/） | internal/ 封装 |
| D6 | 配置文件格式 | JSON（providers / routes / input / memory / global 五节） | TOML/YAML（需引第三方库，与零依赖冲突） |
| D7 | 词典回写 | 预留 `dict_add` 工具（V0 可后补，先手动编辑词典文件） | V0 即实现 |

---

## 2. 背景与目标

### 2.1 产品背景
VoxSign = 语音控制电脑。链路：手机 iOS App（交互/ASR）→ center 中枢（voxsign.net：注册表/ASR/WS 投递/回执/OAuth）→ 各设备上的 **harness**（执行面，Mac/Linux/Windows 对等）。
当前阶段：**V0 本地闭环验证**——不接 center WS 全协议，先跑通「输入 → 模型 → 工具 → 回执」的本地闭环。

### 2.2 定案约束（不可推翻，除非硬性技术障碍）
- 语言：**Go**（单二进制常驻、毫秒启动、goroutine 并发、跨平台交叉编译、go test 内建）。
- 零外部依赖：仅标准库（静态构建，CGO_ENABLED=0）。
- 借鉴 DeepSeek Harness 四点：固定 tool contract（保 prompt cache）、append-only 轨迹日志、Minimal 工具面、模型驱动多步编排。
- 安全：模型输出的命令执行前必须有过滤/确认策略；shell 用 os/exec 并设超时。
- 产出位置：`/Users/zouyongming/DoubaoWork/chats/2026-10-01/new-chat-23/voicesign-harness/`。

### 2.3 架构级目标（本次新增，优先级最高）
1. **长期增长型**：模块可插拔，能力可持续增长（工具可不断加、模型可不断换、路由可不断扩）——**可变化性是第一设计目标**，不是先堆功能。
2. **模型路由层**：多端点可配置。默认模型中心 `aiops.voxsign.ai`（OpenAI 兼容网关，域名统一 VoxSign.ai），同时支持直连 DeepSeek / OpenAI / Gemini 等任意 OpenAI 兼容端点；配置文件 + 环境变量切换；按指令/按场景选模型（快模型做意图、强模型做复杂任务）；路由表可扩展。
3. **输入容错层（ASR 原文处理管线）**：harness 输入是语音转写文本——模糊、口语化、含同音误词与识别错误（真实案例：「Mansour」被误识别为「美墅」、「季总」应为「冀总」）。必须：①轨迹无条件保留 ASR 原文；②规范化/纠错（个人词典+专有名词表做同音/近音纠正与术语映射）；③意图归一化+参数抽取 → 结构化意图 JSON（意图+槽位+置信度），低置信度**回问而非瞎执行**。
4. **个人上下文沉淀层（memory/profile）**：用户信息、偏好、历史决策/任务结果 → 结构化上下文注入模型（分层：长期画像 / 短期会话 / 工具结果回写）；个人词典归本层管理，**输入纠错与模型提示双向复用**，新识别专有名词可回写扩充词典。V0 至少机制+数据结构，简单实现可后补。
5. **单用户自用**：不设计多租户、计费、对外发布；架构预留但不实现。
6. **交付顺序**：先出本架构文档，评审后再按架构实现代码。

### 2.4 非目标（V0 明确不做）
center WS 全协议对接、多租户/计费/对外发布、流式输出（预留）、插件外部加载（预留）、自动学习型画像更新（预留）。

---

## 3. 设计原则（按优先级）

| 优先级 | 原则 | 落地方式 |
|--------|------|----------|
| P0 | 可变化性（第一目标） | 各层接口化：Provider 接口、Tool 注册表、Route 表、记忆类型表；配置驱动新增，不改核心代码 |
| P1 | 高性能 | Go 单二进制；固定 tool contract 保 prompt cache；模型一次返回多步动作减少往返；本地意图直通（TIME 类零 LLM 调用） |
| P2 | 极简 | V0 工具面 5 个、意图 8 类，不堆功能 |
| P3 | 安全 | 高危命令默认拦截 + confirm 策略 + 超时钳制 + 全量轨迹可审计 |
| P4 | 移动端优先交付 | 文档/README 手机可读、中文为主 |

---

## 4. 总体架构

### 4.1 分层模块图

```
┌─────────────────────────── 设备端 harness（Go 单二进制） ───────────────────────────┐
│                                                                                    │
│  入口层 Entry      stdin 文本 │ 本地 HTTP :8765 │ REPL │（预留：center WS 客户端）     │
│        │ 请求(ASR 原文或文本)                                                       │
│        ▼                                                                           │
│  输入容错层 input   ①原文保留→②清洗→③词典纠错→④意图JSON(置信度)→⑤低置信回问           │
│        │ 意图JSON + 纠错文本                                                       │
│        ▼                                                                           │
│  路由层 router      按 意图+关键词 → (provider, model, max_turns)                   │
│        │                                                                           │
│  编排层 agent       固定提示词(角色+工具契约) + 画像/词典注入 + ActionPlan 循环       │
│        │                                                       ▲                    │
│        ▼                                                       │                    │
│  模型接入层 provider OpenAI兼容客户端(多端点) / mock 离线 ──────┘                    │
│        │ ActionPlan(JSON 多步)                                                     │
│        ▼                                                                           │
│  工具层 tools       Registry(可插拔)：shell / read_file / write_file / list_dir /   │
│                    get_time /（预留：dict_add / memory_update）                    │
│        │ Receipt[]                                                                 │
│        ▼                                                                           │
│  记忆层 memory      画像 profile / 词典 dictionary / 事实 facts / 会话 sessions      │
│        ▼                                                                           │
│  输出: stdout │ HTTP JSON │ REPL ── + 轨迹 JSONL（全量、append-only）               │
│                                                                                    │
│  横切：config(文件+env) · trajectory(append-only JSONL) · safety(风险分级/超时)      │
└────────────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 数据流（一次语音指令的生命周期，六段管线）

```
ASR 原文（语音转写，可能含误识别）
  │ ①轨迹：无条件保留原文（原始证据，先于一切处理落盘）
  ▼
[input] 清洗 → 纠错(个人词典/专有名词表) → 意图 JSON{intent, slots, confidence, corrected_text}
  │ 低置信度/缺必需槽位 → 回问(ask 非空，不执行)
  ▼
[router] 按 意图+关键词 → 选择(端点, 模型, max_turns)
  ▼
[agent] 固定提示词(角色+工具契约+画像+词典) + 请求 → [provider] LLM
  │ 模型返回 ActionPlan（JSON 多步动作）
  ▼
[tools] 按序执行（每步超时、风险分级）→ Receipt[]
  ▼
回执聚合 → 必要再一轮 LLM → final
  ▼
[memory] 会话摘要/事实写回 · [trajectory] 全量 JSONL（含 ASR 原文、意图、路由、动作、回执）
  ▼
输出：stdout / HTTP JSON / REPL
```

### 4.3 已就位骨架（评审后继续使用）
- `go.mod`：module `voicesign-harness`，go 1.22，零依赖。
- `contract/`：共享数据契约（Message / ActionPlan / Action / Receipt / Usage / ToolSchema / ParseActionPlan）——与本文档兼容，后续新增类型（意图 JSON、词典条目、轨迹条目）随各包落位。
- 其余包（config/provider/router/input/memory/tools/agent/server）在评审通过后按本文档实现。

---

## 5. 输入容错层（input 包）——V0 管线第一段，优先级最高

### 5.1 管线五级

| 级 | 名称 | 职责 | 产物 |
|----|------|------|------|
| ① | raw 保留 | 轨迹无条件记录 ASR 原文（先于一切处理，作为原始证据）；输入对象不可变地携带原文 | `Input{Raw}` |
| ② | clean 清洗 | 去填充词（嗯/那个/请/帮我/麻烦/的话）、全角→半角、压缩空白、去多余标点 | `cleaned` |
| ③ | correct 纠错 | 个人词典+专有名词表变体替换（同音/近音/误写）；内置常见同音字小表；**词典注入提示词由模型做二次纠错**（模型在意图 JSON 中输出 `corrected_text`） | `corrected_text` + `corrections[]` |
| ④ | intent 意图 | 关键词+槽位抽取 → 意图 JSON；确定性规则分类，V0 8 类 | `Intent{intent, slots, confidence}` |
| ⑤ | ask 回问 | 置信度 < 阈值 或 缺必需槽位 → 回问文案，**不执行** | `Intent{ask:"…"}` |

每一级都写入轨迹（kind: `input_raw` / `input_clean` / `input_correct` / `intent`），保证「为什么这么理解」可回放审计。

### 5.2 意图分类法（V0，确定性规则）

| 意图 | 触发词示例 | 必需槽位 | 本地置信度 | 动作/工具 |
|------|-----------|----------|-----------|-----------|
| TIME | 几点 / 时间 / 日期 / 星期 | 无 | 0.95（零 LLM 直通） | get_time |
| FILE_READ | 读 / 打开文件 / 看看内容 | path | 0.8 | read_file |
| FILE_WRITE | 写 / 保存 / 创建 / 追加 | path + text | 0.7 | write_file |
| FILE_LIST | 列表 / 目录 / 文件夹 / 有什么 | path（默认 .） | 0.8 | list_dir |
| SHELL | 运行 / 执行 / 跑一下 / 命令 | cmd | 0.7 | shell |
| APP_LAUNCH | 打开 + 应用名 | app | 0.6（高危动作，需确认） | shell(open, confirm) |
| INFO | 其余查询（翻译/总结/研究…） | query | 0.4 → 交模型 | 模型 |
| UNKNOWN | 无触发词 | — | 0.2 → 回问 | ask |

置信度启发式：基础分 − 缺失必需槽位×0.2；阈值 `VHS_INTENT_CONF`（默认 0.6）；低于阈值且非 INFO → `ask` 回问（模板：「你是想让我…？请再说一遍」）；`VHS_LOW_CONF_ACTION=model` 可改为交快模型澄清。

### 5.3 纠错演示（写入验收，实现阶段必须跑通）

```
ASR 原文："帮我打开美墅的文件夹看看有什么"
→ clean:   "打开美墅的文件夹看看有什么"
→ correct: 词典 {"term":"Mansour","variants":["美墅","曼苏尔","曼苏"]}
           → "打开 Mansour 的文件夹看看有什么"
           corrections:[{"from":"美墅","to":"Mansour","rule":"dict"}]
→ intent:  FILE_LIST, slots{path:"~/Documents/Mansour"}, conf 0.8
→ router:  file 场景 → provider(center) → LLM ActionPlan [list_dir]
→ 执行 → 回执 → final（含纠错说明）
```

另例：「季总」→ 词典变体「季总」→「冀总」。

### 5.4 个人词典（归记忆层管理，双向复用）

```json
// memory/dictionary.json
{
  "version": 1,
  "terms": [
    {"term": "Mansour", "variants": ["美墅","曼苏尔","曼苏"], "category": "人名", "source": "manual", "last_used": "2026-10-01"},
    {"term": "冀总",   "variants": ["季总"], "category": "称呼", "source": "manual"},
    {"term": "model.peterzou.com", "variants": ["彼得周点com","model彼得周"], "category": "域名", "source": "manual"},
    {"term": "VoxSign", "variants": ["voxsign","沃克斯赛因"], "category": "产品名", "source": "manual"},
    {"term": "center", "variants": ["中枢","森特"], "category": "架构名", "source": "manual"}
  ]
}
```

**双向复用**：
- 输入侧：input.correct 读取词典做变体替换；
- 模型侧：词典块注入提示词（模型输出用正确术语、理解语境）；
- 回写：模型纠正出的新专有名词 → 追加词典条目（`source:"model"`）；V0 先手动编辑，`dict_add` 工具预留后补。

### 5.5 意图 JSON schema

```json
{
  "intent": "FILE_LIST",
  "slots": {"path": "~/Documents/Mansour"},
  "confidence": 0.8,
  "corrected_text": "打开 Mansour 的文件夹看看有什么",
  "corrections": [{"from": "美墅", "to": "Mansour", "rule": "dict"}],
  "ask": ""
}
```

`ask` 非空 ⇒ 不得执行任何动作，直接回问。

---

## 6. 模型路由层（router 包）——多端点可配、按场景选模型

### 6.1 路由表（配置驱动，可扩展）

```json
// 配置节 routes：按 意图 + 关键词 命中，先命中先得，default 兜底
[
  {"name": "time",    "intent": ["TIME"],                    "provider": "local",      "max_turns": 0},
  {"name": "file",    "intent": ["FILE_READ","FILE_WRITE","FILE_LIST"], "provider": "center", "max_turns": 1},
  {"name": "intent",  "intent": ["INFO"], "match": ["翻译","总结","摘要","问答"], "provider": "fast", "max_turns": 1},
  {"name": "complex", "match": ["代码","脚本","debug","部署","分析","研究","架构"], "provider": "strong", "max_turns": 2},
  {"name": "default", "provider": "center", "max_turns": 2}
]
```

- 命中输出：`(provider 名, model, max_turns)`；provider 名在 providers 表中解析。
- 显式覆盖：`VHS_ROUTE=<route名>` 强制路由；`VHS_PROVIDER=<provider名>` 强制端点。
- 路由结果（含实际端点/模型）写入轨迹与回执——「这次用了哪个模型」可审计。

### 6.2 providers 表（多端点，默认模型中心）

```json
// 配置节 providers：name / kind(openai|mock) / endpoint / model / api_key / response_format / timeout_ms
// 模型名为 2026-10 调研时点（见 §14.2）；center 实测默认 gpt-4o-mini，模型表以带 key 拉取 /v1/models 为准
[
  {"name": "center", "kind": "openai", "endpoint": "https://model.peterzou.com", "model": "gpt-4o-mini", "api_key": ""},
  {"name": "fast",   "kind": "openai", "endpoint": "https://model.peterzou.com", "model": "gpt-4o-mini", "api_key": ""},
  {"name": "strong", "kind": "openai", "endpoint": "https://model.peterzou.com", "model": "deepseek-v4-pro", "api_key": ""},
  {"name": "deepseek", "kind": "openai", "endpoint": "https://api.deepseek.com", "model": "deepseek-flash", "api_key": ""},
  {"name": "openai", "kind": "openai", "endpoint": "https://api.openai.com/v1", "model": "gpt-5.4-mini", "api_key": ""},
  {"name": "gemini", "kind": "openai", "endpoint": "https://generativelanguage.googleapis.com/v1beta/openai", "model": "gemini-3.8-flash", "api_key": ""},
  {"name": "mock",   "kind": "mock"}
]
```

- 同一网关可声明多个 provider（不同 model）→ 实现「按场景选模型」。
- API key 解析顺序：env `VHS_API_KEY` > 该 provider 配置的 api_key；配置文件建议 chmod 600。
- provider 可带按端点专属参数（透传，兼容层差异见 §14.2）：如 DeepSeek 确定性输出建议 `"reasoning_effort": "none"`（thinking 默认开启且会吞掉 temperature）。
- 端点实测要点（2026-10-01，详见 §14.2 与 `docs/endpoints-research.md`）：
  - `model.peterzou.com` 实测存活：`POST /v1/chat/completions` + Bearer 鉴权，根路径声明 `default_model: gpt-4o-mini`；另有 `/v1/decisions`、`/v1/custom/:name` 私有路由——**勿依赖**；拿到有效 key 后先拉 `/v1/models` 校准模型表。
  - DeepSeek：json 模式**必须**在提示词中含 "json" 字样并给 JSON 样例（本架构固定提示词天然满足）；前缀缓存默认自动、磁盘 KV，命中约 $0.003–0.044/1M。
  - OpenAI：前缀缓存须 ≥1024 tokens 且逐字节一致（见 §13 的诚实说明）。
  - Gemini 兼容层：无自动前缀缓存（需显式 cached_content），未列出参数被静默忽略。

### 6.3 Provider 接口（可插拔核心）

```go
// provider 包
type Provider interface {
    Name() string
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) // OpenAI 兼容
}
// 注册表：NewRegistry(cfg) → Get(name)；新增端点 = 配置加一行；新增 kind（如 anthropic）= 实现接口注册
```

---

## 7. 个人上下文沉淀层（memory 包）

### 7.1 分层与存储

| 层 | 内容 | 存储 | 注入方式 | V0 状态 |
|----|------|------|----------|---------|
| L0 长期画像 | 用户信息/偏好/历史决策 | `memory/profile.json` | 首条 user 消息前缀（低频变化，命中前缀缓存） | 实现 |
| L0′ 词典 | 个人词典/专有名词表 | `memory/dictionary.json` | 同上（与画像同块注入） | 实现 |
| L1 短期会话 | 会话上下文 | `memory/sessions-YYYYMMDD.jsonl` | 预留：按需摘要注入 | 机制+数据结构，注入后补 |
| L2 工具结果写回 | 每次 Run 的执行摘要 | `memory/sessions-*.jsonl` + `memory/facts.jsonl` | 事实块（≤20 条）随画像注入 | 实现（被动写回）；facts 自动提炼预留 |

### 7.2 注入策略（缓存友好）

```
系统提示词（恒定）：角色 + 工具契约           ← 每轮命中缓存
首条 user 消息前缀（低频变化）：画像块 + 词典块  ← 变更少，前缀缓存命中
动态内容：当前请求（纠错后文本） + 回执消息
```

### 7.3 隐私边界（明示）
画像/词典/事实会随请求发送到所选模型端点（这正是其用途）；敏感凭据不进画像（密钥只存配置文件）；本地文件权限 0600。

---

## 8. 编排层（agent 包）

- 固定 system prompt（角色 + 工具契约 + 输出 JSON 规则）——启动时渲染一次，字节稳定，`prompt_hash` 落轨迹用于诊断缓存命中。**提示词必须含 "json" 字样与 JSON 样例**（DeepSeek json_object 模式的硬性要求，见 §14.2）。
- 工具契约**只增不改（addition-only）**：未来新增工具一律追加到契约块末尾，永不改写首部——借鉴 dsh 的 `toolUpdate:'addition-only'` / `systemPromptUpdate:'in-history'`，工具增长不破坏前缀缓存。
- 模型驱动多步编排（借鉴 dsh Code/PTC 模式）：模型一次返回 `ActionPlan{actions[], final}`，harness 一次会话按序执行并聚合回执；`max_turns` 由路由决定（默认 2）；达上限后不再调模型，用回执合成 final。
- 动作执行：独立只读动作可并行（Go goroutine，对标 dsh 的 `Promise.all` 只读并发）；写动作串行。V0 默认顺序执行，并行作为扩展点。
- 失败处理：执行失败记录进轨迹与回执，但**不入模型历史**（对标 dsh `assistant/attempt`：失败尝试只留存不入 history）——避免把错误噪音喂回模型；回执按「user 角色 + 标记块」回传（不依赖 OpenAI function-calling 协议，兼容任何 OpenAI 兼容端点）。
- 编排策略可插拔：V0 用「单轮多步 + 至多 max_turns 轮」；流式输出、增量编排等作为新策略预留。

---

## 9. 工具层（tools 包）

- `Registry`：`Schemas() []contract.ToolSchema`（固定顺序、字节稳定）+ `Execute(ctx, Action) Receipt`。
- V0 五个工具（冻结 schema，实现阶段原样落地）：`shell` / `read_file` / `write_file` / `list_dir` / `get_time`；预留 `dict_add` / `memory_update`。
- 扩展一个工具 = 4 步：①写 schema 常量 ②实现 Execute 分支 ③注册进 Registry ④加测试——不触碰其他模块。
- 安全分级：低危自动执行；高危（rm/mv/sudo/chmod/kill/shutdown/curl/wget/重定向 `>`/`open` 等）默认拦截，`confirm:true` 或 `allow_high_risk` 放行；交互式命令（vim/ssh/top）一律拦截；超时钳制（默认 15s，上限 120s）；输出截断（默认 4000 字符）。

---

## 10. 轨迹与可观测（trajectory，append-only JSONL）

- `memory/../trajectory-YYYYMMDD.jsonl`（0600，O_APPEND，逐条落盘）。
- 条目类型：`input_raw`（ASR 原文，无条件保留）/ `input_clean` / `input_correct` / `intent` / `start` / `model` / `actions` / `receipts` / `final` / `error`。
- 每条含：ts、request_id、turn、kind、内容；model 条含 prompt_hash、usage、latency_ms。
- 回放：按 request_id 过滤可完整还原一次指令的「原文→理解→动作→结果」。

---

## 11. 配置（config 包）

- 文件：`~/.voicesign/harness.json`（默认；`VHS_CONFIG` 指定），五节：`global` / `providers` / `routes` / `input` / `memory`。
- 环境变量覆盖（优先级最高）：`VHS_CONFIG` `VHS_API_KEY` `VHS_PROVIDER` `VHS_ROUTE` `VHS_LOG_DIR` `VHS_ADDR` `VHS_ALLOW_HIGH_RISK` `VHS_INTENT_CONF` `VHS_LOW_CONF_ACTION` `VHS_ACTION_TIMEOUT_MS` `VHS_MAX_OUTPUT_CHARS` 等。
- 特殊端点：`kind:"mock"` = 内置离线 mock（无 key 可跑通闭环，开发/演示用）。

---

## 12. 安全

1. 模型命令执行前强制走风险分级（见 §9）；默认「低危自动、高危待确认、交互拦截」写进 README 与代码注释。
2. 每步动作独立超时，shell 用 `os/exec` + `CommandContext`。
3. 密钥不落轨迹/日志；配置文件 0600。
4. 轨迹全量可审计——「模型让我执行过什么」永久留痕。

---

## 13. 性能目标与基准

| 指标 | 目标 | 验证 |
|------|------|------|
| 进程冷启动（version 子命令） | 中位数 < 50ms | `bench/startup_bench.sh` 30 次采样 |
| mock 闭环 E2E harness 开销（不含真实 LLM） | < 20ms/请求 | `bench` go test -bench |
| 单二进制体积（-s -w） | < 15MB | build.sh 输出记录 |
| TIME 意图本地直通 | 零 LLM 调用，仅本地执行 | 冒烟用例 |
| 固定前缀缓存 | prompt_hash 稳定；支持缓存的端点命中 | 轨迹诊断 |

真实延迟主要由 LLM 网络/推理决定；固定前缀 + 低变画像块使支持缓存的端点降首 token 延迟与成本。
**缓存门槛的诚实说明**（调研实测，见 §14.2）：OpenAI 前缀缓存须 ≥1024 tokens 且逐字节一致——V0 的固定前缀（系统提示词+画像块）可能不足 1024 tokens，OpenAI 端点在 V0 下大概率不命中；DeepSeek 磁盘缓存无明确 token 门槛，但要求完整命中"已持久化的前缀单元"。因此 V0 把缓存收益定位为"DeepSeek/模型中心上的固定前缀一致性"（prompt_hash 监控），前缀随工具/画像增长后缓存收益自动放大。

---

## 14. 借鉴 dsh 与端点兼容性（调研结论，2026-10-01 实测）

### 14.1 DeepSeek Harness（dsh）设计点映射 → 详见 `docs/dsh-notes.md`

**重要事实**：dsh 是 **TypeScript/Node**（Cordis 插件框架 + pnpm monorepo），非 Go——Go 重写只能借鉴其设计与协议，不能照搬运行时。

| dsh 做法（来源：真实仓库） | 本架构落地 |
|---------------------------|-----------|
| 固定 tool contract：`PromptSection{order,name,text}` 分节注册、一次装配；工具声明按字典序确定性渲染（"byte-identical text"）；`systemPromptUpdate:'in-history'` / `toolUpdate:'addition-only'`——变化追加在缓存历史之后，不改写首部 | agent 启动渲染一次固定前缀；工具契约**只增不改**（§8）；prompt_hash 落轨迹对账（对标其 cacheReadTokens/cacheWriteTokens） |
| append-only 轨迹：`session.jsonl`，事件 envelope `{type,seq,time,data}`；`assistant/attempt` 失败尝试只留存不入历史；`deriveMessages()` 从日志投影模型历史，fork/resume/调试全靠重放 | trajectory JSONL：`input_raw→…→final` 全事件；失败执行留痕不入模型历史（§8）；按 request_id 重放还原（§10） |
| Code/PTC 模式：保留工具 `run_code{code,description,timeoutMs}`，代码内 `await tools.name(args)` 编排；只读调用并发（Promise.all）、写调用串行；失败走 8 类正交错误而非异常；一轮 turn = 多 step | 模型返回 JSON `ActionPlan`（多步动作），harness 一次会话按序执行聚合回执；只读并行为扩展点（§8）；V0 用 JSON 动作序列而非任意代码（安全优先） |
| Minimal 思想：默认只留少量必要工具，能力按需注册 | tools 注册表 + V0 五工具（§9） |
| 仓库事实：github.com/deepseek-ai/deepseek-harness · MIT · TS · 建仓 2026-08-13、最近 push 2026-09-29 · star 约 24 万 |

### 14.2 端点兼容性压缩表 → 详见 `docs/endpoints-research.md`

| 特性 | model.peterzou.com（实测） | api.deepseek.com | api.openai.com | Gemini 兼容层 |
|------|---------------------------|------------------|----------------|---------------|
| 路径/鉴权 | `/v1/chat/completions` + Bearer（实测） | `…/chat/completions` + Bearer | `/v1/chat/completions` + Bearer | `…/v1beta/openai/…` + Bearer |
| json_object | 未查证（取决于上游） | ✅ 须提示词含 "json"+样例 | ✅ json_object/json_schema | ✅（json_schema 非完全递归） |
| 前缀缓存 | 未查证 | 默认自动磁盘缓存，命中约 $0.003–0.044/1M | 自动；**≥1024 tokens 且逐字节一致**；省至高 90% | 无自动缓存，需显式 cached_content |
| 2026-10 模型名 | 默认 gpt-4o-mini（实测） | deepseek-flash / deepseek-v4-pro | gpt-5.6 系 / gpt-5.5 / gpt-5.4-mini | gemini-3.8-flash 等 3.x |
| 主要坑 | 私有路由 `/v1/decisions`、`/v1/custom/:name` 勿依赖 | thinking 默认开、temperature 被吞（需 reasoning_effort:"none"） | 前缀不足 1024 tokens 永不命中；>15 req/min 溢出分流 | 未列参数被静默忽略；2.5Pro/3 系关不掉思考；仍 beta |

**对路由层/编排层的落地要求**（已并入 §6.2 / §8）：DeepSeek 提示词含 "json"+样例；确定性输出用 `reasoning_effort:"none"`；center 需有效 key 后拉 `/v1/models` 校准；Gemini 端点在 V0 不作为默认（缓存/参数兼容成本高）。

---

## 15. 目录结构与扩展点（growth 版）

```
voicesign-harness/
├── go.mod / ARCHITECTURE.md / README.md(实现阶段)
├── main.go          # 入口：run / serve / repl / version / config
├── server/          # 本地 HTTP：healthz、/v1/run（预留：center WS 客户端挂载点）
├── input/           # 输入容错层：clean/correct/intent/ask（扩展：新意图、新纠错规则）
├── router/          # 路由：意图+关键词→(provider,model,turns)（扩展：routes 表加行）
├── provider/        # 模型接入：OpenAI 兼容客户端 + mock（扩展：新 kind 实现接口）
├── agent/           # 编排：固定提示词 + ActionPlan 循环 + 轨迹（扩展：新编排策略）
├── memory/          # 画像/词典/事实/会话（扩展：新记忆类型、dict_add/memory_update 工具）
├── tools/           # 工具注册表 + shell/文件 + 风险分级（扩展：注册新工具）
├── config/          # 配置加载：文件+env，providers/routes/input/memory/global（扩展：新配置节）
├── contract/        # 共享契约（已就位）
├── bench/           # 性能基准
└── docs/            # 调研/设计文档
```

**扩展点总表**（评审时对照检查"能不能长"）：

| 想加什么 | 动哪里 | 不动哪里 |
|----------|--------|----------|
| 新工具 | tools/ 注册 + schema + 测试 | 其他包 |
| 新模型端点/换模型 | config providers 表加一行/改 model | 代码 |
| 新场景路由 | config routes 表加行 | 代码 |
| 新意图 | input/ 分类表加行 + 槽位规则 | 其他包 |
| 新记忆类型 | memory/ 加类型 + 注入渲染 | 其他包 |
| 新入口（如 center WS） | server/ 挂新 handler | 核心管线 |
| 多租户/计费（预留） | 架构层预留接口，V0 不实现 | — |

---

## 16. V0 验收清单（评审通过后实现）

1. Go 单二进制、零外部依赖；macOS 本机 `go build` 跑通；build.sh 一条命令交叉编译 darwin/linux/windows。
2. 本地闭环：stdin / HTTP / REPL 触发 → 路由 → LLM（配置可指 model.peterzou.com / DeepSeek / 任意 OpenAI 兼容 / mock）→ ActionPlan → 执行 → 回执。
3. **输入容错端到端（含纠错演示）**：`"帮我打开美墅的文件夹看看有什么"` → 纠错（美墅→Mansour）→ 意图 JSON(FILE_LIST) → list_dir 执行 → 回执；轨迹中可查到 ASR 原文、纠错记录、意图 JSON。
4. 固定 tool contract + append-only 轨迹 JSONL（含 ASR 原文无条件保留）。
5. 路由层：默认 center + deepseek/openai/gemini/mock 可切；按场景选模型生效（如 TIME 本地直通、复杂任务 strong）。
6. 记忆层：profile/dictionary 注入生效；词典双向复用（纠错 + 提示词）；会话写回落盘。
7. 安全：高危命令默认拦截演示；超时生效。
8. `go test ./...` 全绿 + bench 数字写入 README。
9. README（中文、手机可读）：安装/配置/运行/扩展工具/安全/性能。

---

## 17. 里程碑（评审后执行）

| 里程碑 | 内容 | 门禁 |
|--------|------|------|
| M0（本次） | 架构文档评审 | 本文档定稿冻结 |
| M1 | config(五节) + provider(mock/openai) + input(clean/correct/intent/ask) + memory(画像/词典/会话) + router | go test 全绿 |
| M2 | agent(固定提示词/循环/轨迹) + tools(五工具/风险分级) | go test 全绿 |
| M3 | main/server/build.sh/bench/README + 端到端纠错演示 + 交叉编译验证 | 验收清单全过 |

---

## 18. 预留不实现（架构留位）

多租户/计费、center WS 全协议、流式输出、外部插件加载、自动画像学习、多语言 harness、`dict_add`/`memory_update` 工具（V0 后补）。
