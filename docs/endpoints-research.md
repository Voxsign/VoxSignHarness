# VoxSign 模型路由层 — OpenAI 兼容端点兼容性调研笔记

- 调研时点：2026-10-01（沙特时间 UTC+3）
- 方法：官方文档优先（web_fetch 精读 + general_search 交叉验证）；model.peterzou.com 为本地 curl 实测。
- 纪律：每条结论附来源 URL；查不到的标"未查证"；未编造任何参数/模型名。

---

## 0. 压缩结论表（端点 × 特性）

| 特性 | model.peterzou.com（私有网关） | api.deepseek.com | api.openai.com | Gemini OpenAI 兼容层 |
|---|---|---|---|---|
| ① 请求路径 | `POST /v1/chat/completions`（实测确认存在） | `POST https://api.deepseek.com/chat/completions` | `POST https://api.openai.com/v1/chat/completions`（标准路径） | `POST https://generativelanguage.googleapis.com/v1beta/openai/chat/completions` |
| ① 鉴权 | `Authorization: Bearer <key>`（实测返回 `invalid_api_key` auth_error，证实 Bearer） | `Authorization: Bearer <TOKEN>` | `Authorization: Bearer <KEY>`（标准） | `Authorization: Bearer $GEMINI_API_KEY` |
| ② response_format=json_object | 未查证（取决于上游路由；无 key 无法实测） | ✅ 支持，**且 system/user prompt 必须含 "json" 字样并给 JSON 样例** | ✅ 支持（json_object / json_schema） | ✅ 支持（json_object 映射为 `responseMimeType=application/json`；json_schema 不接受完全递归 schema） |
| ③ temperature | 未查证 | ✅ 0–2，默认 1；**thinking 模式下无效** | ✅ 标准支持（推理模型参数约束另见官方） | ✅ 透传；Gemini 3 范围 0.0–2.0，默认 1.0 |
| ④ 前缀缓存 | 未查证 | **默认自动开启的磁盘 KV 缓存**；须完整命中已持久化"缓存前缀单元"；不用后数小时~数天自动清除；命中价约 $0.003–0.044/1M | **自动缓存**（无代码改动、无写缓存费用）；≥1024 tokens 才可能命中；内存缓存静默 5–10 分钟（最长 1h），扩展 retention 最长 24h；命中输入折扣最高 90% | **OpenAI 兼容层无自动前缀缓存**；须显式 `extra_body.google.cached_content` 传 cachedContents ID |
| ⑤ 2026-10 推荐模型名 | 根路径声明 `default_model: "gpt-4o-mini"`（实测） | `deepseek-flash`（=DeepSeek-V4.1-Flash）、`deepseek-v4-pro`；旧名 deepseek-chat/deepseek-v4-flash 的现状未在现行文档出现 | GPT-5.6 家族（gpt-5.6-sol / -terra / -luna，2026-07-09 GA）、gpt-5.5、gpt-5.4-mini；GPT-6 家族 2026-09-22 官宣改进缓存（确切 API ID 未查证，建议实拉 /v1/models） | 文档当前示例 `gemini-3.8-flash`；另有 gemini-3.5/3.6-flash、gemini-3.1-pro-preview 等 3.x 系列 |
| ⑥ 已知坑 | 额外暴露 `/v1/decisions`、`/v1/custom/:name` 私有路由；其余行为未查证 | thinking 默认开启、temperature 被忽略；JSON 模式必须在提示词写 "json"；v4-pro 不支持 vision；峰时（UTC 01–04、06–10 工作日）价格翻倍 | 缓存须 ≥1024 tokens 且前缀逐字节一致；同前缀 >15 req/min 会溢出分流；GPT-5.6 起写缓存可能计费 | 兼容层仍 beta；**未列出的参数会被静默忽略**；Gemini 2.5 Pro / 3 系无法关闭思考；Batch 模式下 json_schema 曾报错；cached_content 需自行预建 |

---

## 1. model.peterzou.com（私人 OpenAI 兼容网关）

> 说明：该网关无公开文档；以下为 2026-10-01 本地 curl 实测（无有效 key，仅探测公开元信息）。

- 根路径 `GET https://model.peterzou.com/` 返回（HTTP 200, application/json）：
  ```json
  {"service":"modelcenter-gateway","status":"ok",
   "endpoints":["/v1/models","/v1/chat/completions","/v1/embeddings","/v1/decisions","/v1/custom/:name"],
   "default_model":"gpt-4o-mini"}
  ```
  来源：本地实测 curl（2026-10-01）。
- ① 路径/鉴权：`/v1/chat/completions` 存在（根端点列表）；`GET /v1/models` 带 `Authorization: Bearer test` 返回 `{"error":{"code":"invalid_api_key","type":"auth_error"}}` → 证实 **Bearer API Key 鉴权**。来源：本地实测。
- ② response_format 支持：**未查证**（取决于网关路由到的上游，需有效 key 实测）。
- ③ temperature：**未查证**。
- ④ 缓存行为：**未查证**（网关层是否透传/二次缓存未知）。
- ⑤ 默认模型：网关自报 `default_model: "gpt-4o-mini"`。来源：本地实测根路径 JSON。
- ⑥ 已知点：除标准 `/v1/models`、`/v1/chat/completions`、`/v1/embeddings` 外，网关还私有暴露 `/v1/decisions`、`/v1/custom/:name`；路由层 schema 不应依赖这两个私有路由。
- 建议：接入后第一件事是 `GET /v1/models`（带真实 key）拉模型清单，并对 chat/completions 做一次 json_object + temperature=0 的冒烟测试，把本节"未查证"项补实。

## 2. api.deepseek.com（DeepSeek 官方）

- ① 路径/鉴权：`POST https://api.deepseek.com/chat/completions`；`Authorization: Bearer <TOKEN>`；`Content-Type: application/json`。官方同时提供 Anthropic 格式 base URL `https://api.deepseek.com/anthropic`。
  来源：https://api-docs.deepseek.com/api/create-chat-completion/ ；https://api-docs.deepseek.com/quick_start/pricing
- ② response_format：支持 `{"type":"json_object"}`（取值 `text` | `json_object`，默认 text）。**硬性要求：必须在 system 或 user prompt 中包含 "json" 字样并给出期望 JSON 样例**，否则模型可能无限输出空白直到 max_tokens。需合理设置 max_tokens 防止 JSON 被截断。
  来源：https://api-docs.deepseek.com/api/create-chat-completion/ ；https://api-docs.deepseek.com/guides/json_mode ；https://api-docs.deepseek.com/zh-cn/guides/json_mode/
- ③ temperature：number，0–≤2，默认 1；官方明示 **"Has no effect in thinking mode"**。top_p 默认 1，仅 thinking 模式生效（有效区间 0.95–1.0）。
  来源：https://api-docs.deepseek.com/api/create-chat-completion/
- ④ 上下文缓存（Context Caching on Disk，**默认对所有用户开启，无需改代码**）：
  - 命中条件：每次请求会在"用户输入末尾"和"模型输出末尾"产生两个独立的**缓存前缀单元**；跨请求公共前缀会被识别并持久化为独立单元；长输入/输出按固定 token 间隔切分单元。**后续请求必须完整匹配某个已持久化的缓存前缀单元才算命中**（部分重叠不命中；例：第一次 A+B，第二次 A+C 不命中，但系统会把公共前缀 A 持久化，第三次 A+D 命中）。
  - TTL："缓存构造需数秒；一旦不再使用通常在数小时到数天内自动清除"（无精确 TTL）。
  - 命中后计费：cache hit 单价远低于 miss（2026-10 现行价，每 1M tokens）：
    - deepseek-flash：hit 峰时 $0.006 / 非峰 $0.003；miss 峰 $0.30 / 非峰 $0.15；输出峰 $1.2 / 非峰 $0.6。
    - deepseek-v4-pro：hit 峰 $0.044 / 非峰 $0.022；miss 峰 $1.32 / 非峰 $0.66；输出峰 $3.96 / 非峰 $1.98。
    - 峰时 = UTC 周一至周五 01:00–04:00 与 06:00–10:00。
  - 观测字段：`usage.prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` / `usage.prompt_tokens_details.cached_tokens`。
  来源：https://api-docs.deepseek.com/guides/kv_cache ；https://api-docs.deepseek.com/quick_start/pricing ；https://api-docs.deepseek.com/news/news0802
- ⑤ 推荐模型名（2026-10 现行文档）：
  - `deepseek-flash`（对应 DeepSeek-V4.1-Flash，上下文 1M，最大输出 384K，并发 2500）。
  - `deepseek-v4-pro`（对应 DeepSeek-V4-Pro-0813，上下文 1M，并发 500，**不支持 vision**）。
  - 旧名 `deepseek-v4-flash` / `deepseek-v4-flash-vision-exp` 仍被接受但已退役，请求被路由到 V4.1-Flash 并按 Flash 价计费。`deepseek-chat` / `deepseek-reasoner` 在现行文档中已不再出现（旧别名现状**未查证**，建议实拉 /models 确认）。
  - 来源：https://api-docs.deepseek.com/quick_start/pricing
- ⑥ 已知坑：
  - thinking 模式默认 `enabled`，temperature 在 thinking 下无效；要用 temperature 采样需 `reasoning_effort:"none"` 或 `thinking.type:"disabled"`。
  - JSON 模式不写 "json" 字样会空转。
  - v4-pro 无 vision；flash 有 vision。
  - 缓存是 best-effort，不保证命中率。
  来源：https://api-docs.deepseek.com/api/create-chat-completion/ ；https://api-docs.deepseek.com/guides/kv_cache

## 3. api.openai.com（OpenAI 官方）

- ① 路径/鉴权：`POST https://api.openai.com/v1/chat/completions`；`Authorization: Bearer <API_KEY>`（标准 OpenAI 协议；与 prompt caching 文档中 `chat.completions.create`、`usage.prompt_tokens_details.cached_tokens` 字段一致）。
  来源：https://platform.openai.com/docs/guides/prompt-caching
- ② response_format：支持 `{"type":"json_object"}` 与 `json_schema`（结构化输出）；官方文档明示"structured output schema 作为 system message 前缀可被缓存"。
  来源：https://platform.openai.com/docs/guides/prompt-caching
- ③ temperature：标准支持（OpenAI Chat Completions 原生参数；推理模型系列对采样参数的额外约束以官方模型页为准）。
  来源：https://platform.openai.com/docs/guides/prompt-caching（同源体系）
- ④ Prompt Caching（**全自动、无额外费用、无需改代码**，gpt-4o 及更新模型默认开启）：
  - 命中条件：**精确前缀匹配**；静态内容（指令、few-shot、tools、image、schema）放最前，动态内容放最后；images/tools/detail 必须逐字节一致。
  - 门槛：**prompt ≥ 1024 tokens** 才可能命中；<1024 的请求 `cached_tokens` 恒为 0。
  - 路由：按 prompt 前 ~256 token 哈希路由到机器；可用 `prompt_cache_key` 影响路由；同 prefix+key 组合 >约 15 req/min 会溢出分流降低命中。
  - TTL：默认内存策略——静默 5–10 分钟后淘汰，最长约 1 小时；`prompt_cache_retention:"24h"` 扩展 retention（支持模型：gpt-5.2 / gpt-5.1 / gpt-5.1-codex 系 / gpt-5 / gpt-5-codex / gpt-4.1 等）最长保留 24h。
  - 计费：缓存读取按输入折扣，最高省 90%、降延迟最高 80%；GPT-5.6 及之后模型写缓存可能另计费（Azure 文档口径，OpenAI 官方 caching 文档称"无写缓存费用"，二者以最新 pricing 页为准）。
  - 观测：`usage.prompt_tokens_details.cached_tokens`。
  来源：https://platform.openai.com/docs/guides/prompt-caching ；https://learn.microsoft.com/en-au/Azure/foundry/openai/how-to/prompt-caching ；https://openai.com/api/pricing/
- ⑤ 推荐模型名（2026-10）：
  - GPT-5.6 家族 2026-07-09 GA：`gpt-5.6-sol`（旗舰推理）、`gpt-5.6-terra`（均衡）、`gpt-5.6-luna`（低成本）。
  - `gpt-5.5`（2026-04-24 GA）；`gpt-5.4-mini`（文档示例价：输入 $0.75 / 缓存输入 $0.075 / 输出 $4.50 每 1M）。
  - GPT-6 家族：2026-09-22 OpenAI 官宣"为 GPT-6 打造更出色的提示词缓存"（30 分钟窗口内复用共享前缀享缓存折扣），但**确切 API model ID（如 gpt-6-*）未在官方文档检索页证实，标注未查证**。
  来源：https://learn.microsoft.com/en-ca/Azure/foundry/openai/concepts/model-retirement-schedule ；https://openai.com/api/pricing/ ；https://openai.com/zh-Hans-CN/index/better-prompt-caching-for-gpt-6/
- ⑥ 已知坑：
  - 缓存前缀必须逐字节一致——system prompt 里插入时间戳/随机串会击穿缓存。
  - <1024 tokens 的请求永远不命中。
  - 缓存不跨组织共享；cached tokens 仍计入 TPM 限流。
  - 用 Responses API 的 extended caching（24h）会失去 Zero Data Retention 资格。
  来源：https://platform.openai.com/docs/guides/prompt-caching

## 4. generativelanguage.googleapis.com 的 OpenAI 兼容层（Gemini）

- ① 路径/鉴权：`base_url = https://generativelanguage.googleapis.com/v1beta/openai/`，完整路径 `POST https://generativelanguage.googleapis.com/v1beta/openai/chat/completions`；`Authorization: Bearer $GEMINI_API_KEY`（key 在 Google AI Studio 申请）。
  来源：https://ai.google.dev/gemini-api/docs/openai
- ② response_format：
  - `json_object` 被兼容层解释为 Gemini 的 `responseMimeType="application/json"`。
  - `json_schema` 支持，但**不接受完全递归的 schema**；`additional_properties` 接受。
  - 官方推荐走 `client.beta.chat.completions.parse(model=..., response_format=PydanticModel / zod)` 结构化输出。
  - 已知坑：Batch API 场景下 `response_format.json_schema` 曾报 invalid JSON 错误（2026-02 开发者论坛贴）；chat 实时路径可用。
  来源：https://ai.google.dev/gemini-api/docs/openai ；https://docs.cloud.google.com/vertex-ai/generative-ai/docs/migrate/openai/overview ；https://discuss.ai.google.dev/t/structured-outputs-in-batch-using-openai-compatibility-mode/126309
- ③ temperature：兼容层透传 `temperature`（Vertex 迁移表确认 temperature/top_p/stop/seed 均映射）；Gemini 3 系列范围 0.0–2.0、默认 1.0；采样参数在思考预算下的具体行为见模型文档。
  来源：https://ai.google.dev/gemini-api/docs/openai ；https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/models/inference ；https://docs.cloud.google.com/vertex-ai/generative-ai/docs/migrate/openai/overview
- ④ 缓存：**OpenAI 兼容层不提供 OpenAI 式自动前缀缓存**。官方方式是用 Gemini 原生显式内容缓存：先建 cachedContents，再在请求里 `extra_body={"google":{"cached_content":"cachedContents/<id>"}}`。无"前缀自动命中+折扣"的等价物。
  来源：https://ai.google.dev/gemini-api/docs/openai（extra_body / cached_content 章节）
- ⑤ 推荐模型名（2026-10 文档当前示例）：`gemini-3.8-flash`（文档主示例）；检索中还出现 `gemini-3.6-flash`、`gemini-3.5-flash`、`gemini-3.1-pro-preview`。用 `GET /v1beta/openai/models`（Bearer key）可拉实时清单。
  来源：https://ai.google.dev/gemini-api/docs/openai ；https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/start/openai
- ⑥ 已知坑：
  - 兼容层**仍标 beta**；**未列出的参数会被静默忽略**（不报错），路由层调试时要留意"参数不生效≠传错"。
  - `reasoning_effort:"none"` 仅 Gemini 2.5 可关思考；**Gemini 2.5 Pro 与 Gemini 3 系无法关闭思考**。
  - `reasoning_effort` 与 `thinking_level`/`thinking_budget` 互斥，不能同传。
  - Grounding with Google Search 仅 Gemini 3+ 可用，且需走 extra_body。
  - service_tier 映射 flex/priority，默认 standard。
  - system 角色消息兼容层支持（文档示例 messages 首条即 role=system）。
  来源：https://ai.google.dev/gemini-api/docs/openai

---

## 附：对 VoxSign 路由层 schema 的启示（基于上述事实）

1. **统一契约**：四个端点都认 `POST {base}/chat/completions` + `Bearer`，路由层可只用 base_url + api_key + model 三元组。
2. **json_object 不是免费的**：DeepSeek 必须在提示词里写 "json"；Gemini Batch 路径 json_schema 不稳；OpenAI 最干净。路由层应按端点注入 prompt 后缀提示。
3. **temperature 在 DeepSeek thinking 模式下被吞**：若 VoxSign 需要确定性输出（如指令解析），DeepSeek 路由要显式 `reasoning_effort:"none"`，否则 temperature=0 不生效。
4. **缓存策略差异**：OpenAI/DeepSeek 自动前缀缓存（要求静态 system prompt 前置、逐字节一致、DeepSeek 需完整前缀单元）；Gemini 兼容层无自动缓存，要省钱需自建 cachedContents。路由层应把 system prompt 设计为固定前缀。
5. **模型名要做成可配置映射**：2026-10 时点 DeepSeek 已切到 deepseek-flash/deepseek-v4-pro，OpenAI 在 GPT-5.6/GPT-6 交替期，网关自报默认 gpt-4o-mini——schema 里不要硬编码旧名。
