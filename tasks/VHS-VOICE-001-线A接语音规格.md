# VHS-VOICE-001 · 线 A 接语音（`/v1/voice`）规格与判据

> **状态**：规格已定；实现进行中（`RunWithIntent` + `IntentFromASR`）。
> **来源**：2026-10-03 Peter 选「线 A 为主入口，ASR 服务保持独立」之后的一系列裁定。
> **⚠️ 本文是规格，不是实现说明。判据先于实现。**

---

## 1. 背景：两条线，分工本就是对的

```
线 A = main.go + server/server.go   ← 真正的 harness
  /v1/health · /v1/roles · **/v1/run** · /v1/status
  **/v1/tasks** · /v1/tasks/{id} · **/v1/confirm** · /v1/cancel
  pipeline.Run(ctx, &o, text) 在 server.go:295 与 :464

线 B = cmd/vhs-asr                  ← ASR 个性化服务（设计上不执行）
  /v1/correct · **/v1/process** · /v1/task · /v1/observe · /v1/feedback
  /v1/lexicon · /v1/blacklist · /v1/dictionary · /v1/testpage · /v1/testlog · /v1/health
```

**唯一缺口**：线 A 没接语音。`POST /v1/voice` 即为此而建。
**⚠️ 线 B 必须独立** —— 它服务多消费方（Peter 定的），**不得为了线 A 的方便改它的契约**。

---

## 2. 规格：**意图优先 + 原文兜底**

**依据（职责划分）**：
```
线 B     ：原始文本 → **个性化纠错 + 意图识别**
           ★ 它认用户专名（实测：`把爱ops接上` → 内部改写留痕 `爱ops→aiops-portal`）
pipeline ：意图 → 域门禁 → 工具选择 → 执行 → 回执
           ★ 它负责"把意图变成动作"，不负责"听懂用户"
⇒ 权威的识别源是线 B
```

```
① 意图可用        ⇒ 以意图为准，**跳过 pipeline 的 classifier**
② 意图缺失/低置信  ⇒ 回落原文解析，**且必须标 `intent_source: "text-fallback"`**
③ 线 B 不可达      ⇒ **明确报错 + degraded**，**不许静默降级为"直接执行原文"**
④ 无论哪条        ⇒ 回执/响应**必带 `intent_source` ∈ {`asr-intent`, `text-fallback`}**
```

### ⚠️ 关键设计约束：**同一时刻只能有一个权威**

> **不许"意图给一部分、原文解析一部分，然后合并"** —— 那是两个真相源融合，出了错无法归因。
> **要么全用意图，要么全用原文；不许拼。**

---

## 3. 已裁的三处语义（各有理由）

| # | 问题 | 裁定 | 理由 |
|---|---|---|---|
| **①** | `need_disambiguate` 落成什么 | **`Ask`** | 它是本契约里"需澄清"的正典字段，**红线 `Ask != '' → 绝不执行` 已挂在它上面**；`Confirm` 是**授权**语义（auto/light/strong/human），两件事 |
| **②** | `domain_suggestion` 写 `Space` 吗 | **只在"恰好 1 个候选"时写**；空 ⇒ 留空走 refer 兜底；**≥2 个 ⇒ `Space` 留空 + 候选进 `Context`，交 refer 裁决** | `Space` 注释是"**候选**、空=待兜底、**不猜**"。**多个时取第一个 = 替 refer 做决定 = 猜** |
| **③** | `path` 为空怎么写 | **不写这个键** | **"没有值"和"值是空串"在下游是两件事**（同 `unknown ≠ 没有`） |

**⚠️ `domain_suggestion` 的实际类型是列表**（实测：`[]`，且**恒为空**）
⇒ "恰好 1 个 / ≥2 个"在类型上天然成立（`len(...)`）；**但"≥2 个"目前无真实数据可触发**（判据须标 `forward-guard`）。

---

## 4. ⚠️ 撤回的一条条文（记在这里，防复犯）

**曾定**：「线 B 的意图**已经包含纠错语义** ⇒ 走意图时**必须把 cleaner/dict/classifier 三步一起跳过**」

**撤回原因**（实测）：
```
/v1/process 返回的 keys 里**没有 `corrected_text`**
  = [confidence, context_sources, context_sources_details, contract_version,
     control, domain_suggestion, need_disambiguate, params, path, traces, type]
⇒ 跳过 cleaner/dict = **丢掉纠错能力**
```

**⇒ 裁定 A：只跳 `classifier`，保留 `clean/dict`。**
```
理由：① 零契约变更（线 B 多消费方，不为线 A 的方便改它）
     ② A 下"纠错跑两次"**不成立** —— 两次跑的是**同一份 raw 文本**，同源
     ③ B（让线 B 返回 corrected_text）是**正解**，但要升版 ⇒ 留待将来
```

**⇒ 代价与对策**：**两份纠错实现同源但不同实例，逐字一致性 = `UNVALIDATED`。**
**用判据 ⑩ 保证它不会静默。**

---

## 5. 判定实验记录（供后人复用这个手法）

**问题**：`/v1/process` 对 `把爱ops接上` 判 `ASK`，而 `/v1/correct` 能把它纠成 `aiops-portal`
　　　　 ⇒ (a) 两端点各走各的？还是 (b) 走了纠错但别的原因？

**手法**：**读 `/v1/process` 返回的 `traces`**（它本来就返回 traces）

```
traces 5 步（实测，PID 17927）：
  retain     input_raw=把爱ops接上
  correct    正文纠错（Correct 纯函数）
  lexicon    词典纠错（JSON 快照）
  cache      按热词/别名/近音改写（K9）：**改写留痕 爱ops→aiops-portal**
  punctuate  标点恢复（独立字段，不改正文）
⇒ **(b) 成立** —— 走了完整纠错链；ASK 来自**低置信 0.3**
```

**⇒ 真问题不是"纠错跑两次"，而是**：
> **`/v1/process` 在纠错后的文本上判定意图，却不返回纠错后的文本**
> —— **这才是"两个真相源"的真正位置。**

---

## 6. 判据（12 条，先红）

```
① 同一意图 + 不同原始措辞 ⇒ 执行结果必须相同（证明用的是意图）
② 反例：改原始措辞而意图不变 ⇒ 结果不得变
③ 意图缺失/低置信 ⇒ 回落原文 + `intent_source=text-fallback`
④ `intent_source` 必须始终非空
⑤ **不许拼**：意图与原文解析结果不得混合（用"两者故意冲突"验）
⑥' 纠错链走几次必须可观测（trajectory 或 traces，**不许静默**）
⑦ `need_disambiguate=false` ⇒ **不得**置 `Ask`（防红线误拦）
⑧ `domain_suggestion` ≥2 个 ⇒ `Space` 必须留空 + 候选进 `Context`
⑨ `path` 为空 ⇒ `Params` 里**不得出现空的 `path` 键**
⑩ **两份纠错不一致必须能被发现**（不要求一致，要求不静默）
⑪ **判据不得引用未登记的 kind**（kind 须单一枚举来源；反例：写不存在的 kind ⇒ 必须失败，不得空过）
⑫ **同一阈值常数不得表达两个语义**（现 `0.70` 在两处：是否回问 + 是否调模型）
```

**两条"防假绿"的判据值得单独说**：

- **⑪** 来自一个真实风险：`trajectory` 的 10 个 kind 里**没有能表达"classifier 被跳过"的**
  ⇒ 若硬写一个不存在的 kind 名，判据会**"因为匹配不到任何轨迹而空过"**。
  **⇒ 要修的是"判据能在符号不存在时静默空过"这个结构，不是这一处。**

- **⑫** 来自实测：`intent_model.go:32` 与 `:71` **都引用字面量 `0.70`**，分别表达
  "是否调用模型"与"是否回问"。**⇒ 有人只改其中一处时，没有判据能发现。**

---

## 7. 阈值（实测，用实际比较式，不用"低置信"这种词）

```
阈值 = 0.70；比较是 `< 0.70`（**严格小于**）
⇒ 恰好 0.70 ⇒ `NeedDisambiguate = false`（不回问）；0.69 才回问
⇒ "把爱ops接上" 的 confidence=0.3 **严格低于 0.70** ⇒ 触发 ASK
```
**⚠️ 但"0.70 是唯一阈值"未证** —— 只看到这两处比较，**未核 `/v1/process` 完整调用链是否有其它改写。**

---

## 8. 实现形态

```
新增 RunWithIntent(ctx, o, in contract.Intent, rawText string)
  · in 非零 ⇒ **跳过 classifier**；**保留 clean/dict**（裁 A）
  · in 为零/低置信 ⇒ 走现有 Run 路径 + `intent_source=text-fallback`
轨迹小扩展：`KindIntentSource = "intent_source"`（值 ∈ asr-intent | text-fallback）
  · **必须在 `intent` 之前写入**（否则读轨迹时看不到因果）
  · **必须登记进单一枚举来源**（判据⑪的地基）
适配函数：`IntentFromASR(线B JSON) → contract.Intent`（**类型同构，无契约变更**）
```

**⚠️「类型同构」只指类型层面** —— `input.NewTaskClassifier` 与线 B 的解析**语义覆盖度未比对**
（线 B 认用户专名、classifier 认不认**未验**）。**文档一律写「类型同构」，不许简写「同构」。**

---

## 9. 诚实边界

- **两份纠错逐字一致 = UNVALIDATED**（裁 A 接受的代价，用判据⑩兜住"不静默"）
- **`domain_suggestion` ≥2 个候选** 无真实数据 ⇒ 判据⑧须标 `forward-guard`
- **`0.70` 是否唯一阈值未证**
- **`trajectory` 的完整 kind 枚举**已核（10 个），**但 `/v1/process` 的 traces `source` 字段实现细节未核**
- **端到端未在真浏览器里走过**（A1–A12 验收覆盖的是线 B 的测试页，**不含 `/v1/voice`**）

---

*VHS-VOICE-001 · 2026-10-03 · DSH 出具。判据先于实现；本文随裁定变更而升版。*
