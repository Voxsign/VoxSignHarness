# VHS-JEV-001 · JEV 快判断接口：DSH 指定路径（供模型中心对接）

> **Peter 2026-10-03**：「JEV 的路径，这个网站下这个 decisions 不可用。
> **你现在先指定一个就行了，我等一下让那个模型中心自己会改。**
> **就符合你这个能力，你先往前推进就行了。**」
>
> **性质**：**由 DSH 指定接口形态，模型中心按此实现。** 本文即"能力需求书"。

---

## 1. 为什么需要 JEV（它解决什么）

来自 `VHS-FASTSLOW-001` + `VHS-WORKMEM-001` 的实测与设计：

| 层 | 能力 | 延迟目标 |
|---|---|---|
| L0 | 本地规则 + 别名路由 | 微秒 |
| **L0.5** | **JEV 快判断** ← **缺口** | **< 1s** |
| L1 | `deepseek-flash` | 秒级 |
| L2 | `deepseek-reasoner` / `v4-pro` | 数秒–更久 |

**L0.5 的定位**：
> **比别名匹配更会判断，比大模型更便宜更快。**

**典型要它答的问题**（都是"给定了事实，要一个判断"）：
- 「这句话里的『那个』指的是 A 还是 B？」
- 「这个动作在这个域下算不算越权？」
- 「这条记录值不值得学？」
- 「这个目标归到能力缺口还是授权缺口？」

**共同特征**：**输入是紧凑的态势，输出是一个裁决 + 理由。不需要长篇推理。**

---

## 2. 我指定的接口

### 2.1 端点

```
POST https://aiops.peterzou.com/api/decide
```

**为什么是这个名字**：与已实测的 `/api/route`（**路由谁**）、`/api/summary`（**世界状态**）、
`/api/model/chat`（**通用对话**）并列——
**`/api/decide` = 「给定态势，给一个判断」**，语义不重叠。

**⚠️ 我明确避开 `/api/model/decisions`**：实测它是 SPA 兜底、不存在；
且 `/api/model/*` 前缀暗示"模型能力"，而 JEV 的定位是**比模型更轻的快判断**。

### 2.2 请求

```json
POST /api/decide
Authorization: Bearer <AIOPS_KEY>
Content-Type: application/json

{
  "kind": "referent | permission | learnability | gap_class | custom",
  "question": "「那个」指的是 A（上一轮的文件）还是 B（本轮的项目）？",
  "situation": {                      // ← 紧凑态势，由本机渲染（WORKMEM-001 的桌面）
    "candidates": [
      { "id": "A", "label": "plan/planner.go", "why": "上一轮出现", "heat": 0.8 },
      { "id": "B", "label": "voicesign-harness", "why": "本轮出现", "heat": 0.9 }
    ],
    "constraints": ["当前域=project", "不可逆=false"],
    "memory": ["用户最近常提 harness"]
  },
  "options": ["A", "B", "ambiguous"],   // ← 允许的答案集（含"说不清"）
  "must_be_fast": true
}
```

**设计要求**：
| # | 要求 | 理由 |
|---|---|---|
| **J1** | **必须支持 `ambiguous` / `unknown` 选项** | 否则模型会被迫瞎猜（违反 `VHS-ZHIJI-001` §2.1） |
| **J2** | **`situation` 是紧凑的结构化数据，不是长文本** | 快的前提是输入不臃肿（"给指挥员看地图，不是念战报"） |
| **J3** | **`options` 由调用方给出** | JEV **不自己发明答案空间**（防幻觉） |
| **J4** | `kind` 可扩展，但**未识别的 kind 必须报错**而非静默降级 | 防止用错场景 |

### 2.3 响应

```json
{
  "ok": true,
  "choice": "A",
  "confidence": 0.82,                 // ← 必须有：本机据此决定是否升级 L1/L2
  "reason": "A 在上一轮被明确提及且 heat=0.8；B 仅背景出现",
  "evidence": ["memory[0]", "candidates[0].why"],
  "elapsed_ms": 210,
  "model_id": "jev-v1"                // ← 必须：ASR-MODEL-01 M2（无 model_id 结论作废）
}
```

**失败必须结构化**（对齐 `ASR-MC-001` M2）：
```json
{ "ok": false, "error": { "code": "TIMEOUT|BAD_KIND|UNSUPPORTED",
                          "message": "…", "retryable": true } }
```

### 2.4 硬要求

| # | 要求 | 依据 |
|---|---|---|
| **J5** | **`confidence` 必须存在且有意义** | 本机用它决定升级；无置信度则 L0.5 无法工作 |
| **J6** | **`model_id` 必须回传** | `ASR-MODEL-01` M2 |
| **J7** | **`elapsed_ms` 必须回传** | `VHS-FASTSLOW-001` 的台账要记 |
| **J8** | **超时可控**（建议默认 1s，可由 `must_be_fast` 收紧） | 快判断超时就失去意义 |
| **J9** | **`ambiguous` 必须是合法且常用的答案** | **不是失败，是正确答案之一** |
| **J10** | **只读**——不得产生任何副作用 | 判断不是动作 |

---

## 3. 本机侧如何使用（落地要求）

| # | 要求 |
|---|---|
| **U1** | **`/api/route` 命中即返回，不问 JEV**（L0 优先） |
| **U2** | **关联度不足才调 JEV**（L0.5） |
| **U3** | **`confidence` 低于阈值 → 升级 L1**；**`choice=ambiguous` → 直接回问用户**（不升级） |
| **U4** | **JEV 不可用/超时 → fail-open 降级 L0 + 标 `degraded`**（不得崩溃） |
| **U5** | **每次调用记台账**（层级/choice/confidence/elapsed/是否升级） |
| **U6** | **JEV 的判断入工作记忆时标 `judged_by: jev`**，**不得与事实混放**（`WORKMEM-001` §3.3） |

**U3 里那条要特别说清**：

> **`ambiguous` 不升级，直接回问用户。**
> 因为 JEV 说"说不清"，**再问大模型也是猜**——**正确答案就是问人**（`VHS-ZHIJI-001` §2.1）。

---

## 4. 诚实边界

- **`/api/decide` 目前不存在**，这是我指定的接口，**不是已有能力**。
- **JEV 是什么、底层是什么模型、是否真比 `deepseek-flash` 快** —— **我一概不知道**。
  实测：`/api/model/decisions` 与 `/api/decisions` 均为 SPA 兜底。
- **`< 1s` 的延迟目标是参考值，没有依据** —— 我不知道 JEV 的真实性能。
- **`confidence` 是否真有区分度，未验证**。若它对什么都给 0.8，**U3 的升级逻辑就失效**。
- **我没测过任何 JEV 调用**（因为它不存在）。
- **本设计的上游（`WORKMEM-001` 的 `situation` 渲染）零实现** ——
  所以即使 JEV 明天上线，**本机侧也还没有"态势"可送**。

---

## 5. 一句话

> **`POST /api/decide`：给它一张紧凑的地图和一个问题，它给一个判断、一个置信度、一个理由。**
> **它可以说"说不清"——那不是失败，那是正确答案。**

---

*VHS-JEV-001 · 2026-10-03 · 由 DSH 指定接口形态，供模型中心实现。*
