# ASR-MC-001 · 模型中心接口说明（DSH 实测记录）

> **指示**：Peter 2026-10-03 ——「模型中心的接口说明，**你现在直接去读**，读完了告诉他。」
> **来源**：DSH 2026-10-03 真实调用 `model.peterzou.com`（无凭据）
> **用途**：解除实现方对 `default` / `diagnose` / `learn` 三通道对接的阻塞

---

## 1. 实测记录（未经加工）

### 1.1 服务根
```
GET https://model.peterzou.com/          → 200
{
  "service": "modelcenter-gateway",
  "status": "ok",
  "endpoints": ["/v1/models", "/v1/chat/completions", "/v1/embeddings",
                "/v1/decisions", "/v1/custom/:name"],
  "default_model": "gpt-4o-mini"
}
```

### 1.2 鉴权（实测 401）
```
GET https://model.peterzou.com/v1/models   → 401
{
  "error": {
    "code": "missing_api_key",
    "message": "Missing API key. Pass \"Authorization: Bearer <key>\" or \"X-API-Key: <key>\".",
    "type": "auth_error",
    "param": null
  }
}
```

**这两条是接口说明的全部**——**其余路径未探测**（无凭据）。

---

## 2. 从实测能确定的事（不是推测）

| # | 事实 | 依据 |
|---|---|---|
| **M1** | 鉴权两种形式任选：`Authorization: Bearer <key>` **或** `X-API-Key: <key>` | 401 报错原文 |
| **M2** | **错误是结构化的**：`{error:{code, message, type, param}}` | 401 响应体 |
| **M3** | 服务自称 **OpenAI 兼容**（`/v1/chat/completions`、`/v1/embeddings`） | 根端点 endpoints 列表 |
| **M4** | 有一个 **`/v1/decisions`** 端点 —— 与 Peter 说的 **JEV 判断原语**吻合 | 同上 |
| **M5** | 有一个 **`/v1/custom/:name`** —— **具名通道**，`diagnose` / `learn` 极可能落在这里 | 同上 |
| **M6** | `default_model` 当前为 `gpt-4o-mini` | 根端点 |

> **M5 很关键**：Peter 说"两个通道的模型都会变、diagnose 可能用 JEV"——
> `/v1/custom/:name` **正是"具名通道"的形态**，所以
> **`default` / `diagnose` / `learn` 应该是三个具名 custom 通道**，而不是三个硬编码模型。

---

## 3. 对 `ASR-MODEL-02` 的修订（按 Peter 本轮答复）

Peter 明确：

> 「`diagnose` 和 `learn` 各用哪个底层模型？**这个目前都是它会变的**……
> **diagnose 可能是用的 JEV 模型，因为它未来会不断变化**。」
> 「**精确标识那个东西也没关系，做不到就用默认**……我会在那边写默认。」

**因此 `ASR-MODEL-02` 的 L1 需修正**：

| | 旧表述（v1） | **v2（本文件修订）** |
|---|---|---|
| L1 | 三条通道**具名固定**，底层模型由 Peter 指定 | 三条通道**具名**（`default`/`diagnose`/`learn`），**底层模型指向模型中心的具名通道**（`/v1/custom/:name`）；**模型可变，通道名不变** |

**关键区别**：**固定的是通道名，不是模型。**
这与 `ASR-MODEL-01`（评测时模型固定）**不矛盾**：
- **生产**：通道名固定，底层模型可换（Peter 在模型中心侧改）
- **评测**：**评测那一刻把三条通道都指向同一个模型**，否则归因失效

> 这条要写清楚，否则实现方会以为"模型可变"就等于"可以随便换"。

---

## 4. 解除阻塞的部分

| 原阻塞项 | 现状 |
|---|---|
| ① `diagnose`/`learn` 用哪个模型 | ✅ **不需要指定具体模型**——走 `/v1/custom/:name` 具名通道；模型由 Peter 在中心侧改 |
| ② 模型中心接口说明 | ✅ **本文档**（实测到鉴权 + 结构化错误 + 端点列表） |
| ③ 写回白名单 | ⏸ **Peter 明确：由产品判断，他自己不管** → 归产品侧，不再问 |
| ④ 精确模型标识 | ✅ **Peter 明确：用默认即可** |
| ⑤ 工具端点 | ✅ `peterzou.com`（`/api/public/catalog` 已实测）；`model.peterzou.com` 是模型中心 |

**⚠️ 仍缺一件**：**API key**。无 key 只能读到 401。
**没有 key，`default` 通道也无法真正调用**——`enabled:false` 的兜底必须继续保持。

---

## 5. 给实现方的行动项

1. **`default` / `diagnose` / `learn` 落成三个具名 custom 通道**（`/v1/custom/:name`），**不要把模型名写死**。
2. 客户端鉴权支持两种形式（**两种都实现**，按 M1）。
3. **错误处理按 M2 的结构化格式解析**，不要只按 HTTP 状态码判。
4. **`/v1/decisions`** 单列——那是 JEV 判断原语，**与 chat/completions 语义不同**，别混用。
5. **key 未到之前保持 `enabled:false`**，不得因"接口通了"就打开。
6. 评测时把三条通道指向**同一个模型**（`ASR-MODEL-01` M1 的实质要求）。

---

## 6. 诚实边界

- **我只实测了 2 个请求**（根端点 200、`/v1/models` 401）。**其余端点全部未探测**——
  没有 key，也无法知道 `/v1/decisions`、`/v1/custom/:name` 的请求/响应格式。
- **`/v1/custom/:name` 是"具名通道"是我的推断**（基于端点名与 Peter 的描述），**未经调用验证**。
- `ai.peterzou.com`、`open.peterzou.com` **均无法解析**（DNS 失败）。
  所以 Peter 说的「通过 AI open peterzou.com 去找」**我理解为就是 `peterzou.com` 本身**
  （其 `/llms.txt` 与 `/api/public/catalog` 已实测）；**若另有所指请纠正**。
- **本文档不是官方接口文档**，是**实测到的行为**。以官方为准。

---

*ASR-MC-001 · 2026-10-03 · 由 DSH 出具。第 2 节是实测事实，第 3 节是按 Peter 答复的修订建议，两者分开写。*
