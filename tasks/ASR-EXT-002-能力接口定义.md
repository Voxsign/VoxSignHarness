# ASR-EXT-002 · peterzou.com 能力接口定义（L0 HTTP + L1 MCP）

> **指示**：Peter 2026-10-03 ——「如果你需要 MCP 式的工具调用接口也是 OK 的，确定这个我们也可以设计出来。
> 把你认为的调用方式接口变成怎么样的，我可以马上准备一下。**你觉得哪样方式更好，给我提建议。**」

---

## 0. 我的建议（先说结论）

### **推荐：L0 纯 HTTP 必做，L1 MCP 兼容层可选加。两层语义一致。**

| | L0 纯 HTTP + JSON | L1 MCP（JSON-RPC 2.0） |
|---|---|---|
| 实现成本 | **极低** | 中（多一层信封） |
| harness 侧 | 零依赖，`net/http` 十几行 | **不能用 MCP SDK**（违反零第三方依赖）→ 仍要手写 |
| 互操作 | 只有我们自己的客户端 | **任何 MCP 客户端都能连**（Claude Desktop / Cursor / …） |
| 覆盖"调用工具" | ✅ | ✅ |
| 覆盖"**申请新工具**" | ✅（扩展） | ❌ **MCP 没有这个语义** |

### 为什么这么建议

1. **harness 是 Go 零依赖单二进制**——**用不了 MCP SDK**。所以无论选哪个，MCP 那层都得手写。
   MCP 的**消息格式**（JSON-RPC 2.0）很小，对齐它成本低；但**引入 SDK 不行**。
2. **MCP 只解决"调用已有的工具"，不解决"申请不存在的工具"**——而后者正是你说的重点
   （「缺什么工具就可以说，让他去增加什么工具」）。
   **这一块 MCP 标准里没有，必须我们自己定义。**
3. 所以最优是：**语义用我们自己定（三个动作），信封给两种**。

> **一句话**：**能力调用可以走 MCP 标准；能力申请是我们独有的扩展，MCP 装不下。**

---

## 1. 三个动作（语义层，两种信封共用）

| 动作 | 语义 | 谁发起 |
|---|---|---|
| **list** | 你有哪些能力？（含参数 schema、版本、限制、成本） | harness 规划器 |
| **call** | 调用一个已有能力 | harness 执行层 |
| **request** | **申请一个不存在的能力** | harness（**只能申请，不能自动获得**） |

**为什么是这三个**：它们正好对应 `VHS-PLAN-001` 的规划闭环——
规划器需要 `list` 才知道边界（SK-1/2），执行需要 `call`，
而 `request` 是**能力清单唯一的更新来源**（外部补上，本机不安装）。

---

## 2. L0 · 纯 HTTP 定义（建议先做这个）

鉴权：`Authorization: Bearer <key>`（与"能力外置"原则一致）

### 2.1 列能力
```http
GET /api/v1/capabilities?lang=zh
→ 200
{
  "schema_version": "1.0",
  "generated_at": "2026-10-03T…Z",
  "capabilities": [
    {
      "id": "research.catalog",
      "version": "1.0",
      "kind": "content",                    // content | tool | model
      "title":      {"zh":"…","en":"…","ar":"…"},
      "description":{"zh":"…","en":"…","ar":"…"},
      "input_schema":  { …JSON Schema… },   // **必须有**，规划器据此判断能不能用
      "output_schema": { …JSON Schema… },
      "limits": { "rate": "…", "timeout_ms": 30000, "irreversible": false },
      "auth": "key",
      "cost_hint": "free|metered"
    }
  ]
}
```

**硬要求**：
- `input_schema` **必须存在**——没有 schema 的能力，**规划器无法判断能否使用**
- `limits.irreversible` 必须诚实标注（对齐域门禁的默认拒绝）
- 字段三语（与现有 `catalog` 一致）

### 2.2 调用
```http
POST /api/v1/call
{ "id": "research.search", "version": "1.0",
  "args": { "q": "校准闭环" },
  "caller": { "name": "vhs", "trace_id": "…" } }
→ 200
{ "ok": true,
  "result": { … },
  "meta": { "capability_id":"…", "version":"1.0", "elapsed_ms":123,
            "cost": { "unit":"…", "amount": 0 },
            "model_id": "…",              // **若有模型参与，必须带**（ASR-MODEL-01 M2）
            "evidence_ref": "…" } }
```
**失败必须结构化**（不能只有 500）：
```json
{ "ok": false, "error": { "code": "RATE_LIMITED|NOT_FOUND|BAD_ARGS|UPSTREAM",
                          "message": "…", "retryable": true } }
```

### 2.3 **申请一个能力**（你说的重点）
```http
POST /api/v1/requests
{ "need": "把 markdown 转成 pdf",          // 自然语言即可
  "why": "任务 X 第 3 步需要",              // **必填**
  "evidence": { "trace_id": "…", "step": 3, "error": "…" },  // **必填**
  "blocking": true,
  "caller": { "name": "vhs" } }
→ 202
{ "request_id": "req_…", "status": "accepted|duplicate|rejected",
  "note": "…" }
```
**状态查询**：`GET /api/v1/requests/{request_id}`

**三条硬要求**：
1. **`why` 与 `evidence` 必填** —— 无证据的能力申请 = 幻觉固化（对齐 `ASR-MODEL-02` L3）
2. **`202` 而不是 `200`** —— 申请是**异步的请求**，不是立即生效的调用
3. **申请 ≠ 授权** —— 外部加上能力后，**本机仍需域/权限门禁放行**（红线：默认拒绝）

> **第 3 条最重要**：否则"缺什么就加什么"会变成**绕过权限的后门**。

---

## 3. L1 · MCP 兼容层（想加就加，语义同上）

**传输**：Streamable HTTP，`POST /mcp`，JSON-RPC 2.0。
**鉴权**：同上（`Authorization: Bearer`）。

标准方法（**照 MCP 规范**，不自创）：
```
initialize            → serverInfo / capabilities
tools/list            → 映射自 L0 的 GET /capabilities
tools/call            → 映射自 L0 的 POST /call
```

**每个 capability 映射成一个 MCP tool**：
```json
{ "name": "research_catalog",
  "description": "…",
  "inputSchema": { …与 L0 的 input_schema 同源… } }
```

**我们的扩展（MCP 标准没有，必须自定义）**：
```
tools/request         → 映射自 L0 的 POST /requests
```
在 `initialize` 的 `capabilities` 里显式声明这是一个扩展，**不伪装成标准方法**。

> **诚实原则**：**扩展就标成扩展**。把自创方法混进标准方法里，会让第三方客户端误判能力。

---

## 4. 两条硬约束（不管选哪种信封都成立）

| # | 约束 | 为什么 |
|---|---|---|
| **E1** | **契约只增不改**；破坏性变更必须**升版本号**（`v1` → `v2`），旧版本保留一段过渡期 | 本项目的域/契约/判据都走这条，接口不能例外 |
| **E2** | **每次调用必须可归因**：`capability_id` + `version` + `caller` + `model_id`（若有模型）+ `elapsed_ms` | 对齐 `ASR-MODEL-01` M2：**没有 model_id 的评测结论作废** |

---

## 5. 我建议的落地顺序

| 步 | 做什么 | 谁做 |
|---|---|---|
| **1** | L0 三个端点（capabilities / call / requests） | **Peter 侧** |
| **2** | harness 侧手写 L0 客户端（零依赖）+ 能力清单缓存（**只缓存元数据，不囤正文**） | 实现方 |
| **3** | 判据：`EXT-01` 清单与真实能力**双向对账**（接 `VHS-PLAN-001` SK-1/2） | DSH 出，实现方落地 |
| **4** | 判据：`EXT-02` 申请必须带 evidence；`EXT-03` **申请后不得自动获得权限** | DSH 出 |
| **5** | 需要时再加 L1 MCP 层（映射，不改语义） | Peter 侧 |

**第 5 步可以永远不做**——如果只有 harness 一个消费方，L0 就够了。
**做它的唯一理由是开放给第三方 MCP 客户端。**

---

## 6. 诚实边界

- 我**实测过**的只有 `GET /api/public/catalog` 与 `/llms.txt`（HTTP 200，结构见 `ASR-EXT-001`）。
  **本文档的 `/api/v1/*` 是我建议的接口，目前不存在**——不是对已有 API 的描述。
- 我**没有**逐字核对 MCP 规范版本（凭既有了解写的 `initialize`/`tools/list`/`tools/call` 三方法）。
  **若要把 L1 当真，请以 MCP 官方规范为准核一遍方法名与字段**——我不拿记忆当规范。
- 鉴权形态（Bearer key 的具体签发/轮换/作用域）**未定义**，需 Peter 定。
- 本文件**零实现**。

---

*ASR-EXT-002 · 2026-10-03 · 由 DSH 出具。第 2、3 节是**建议的接口**，不是已有接口的描述。*
