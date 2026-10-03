# VHS-PROJMODEL-001 · 同构项目模型格式（一致性校验的地基）

> **为什么先做这个**：`VHS-SELFPROJ-001` §3 的一致性校验要求
> **harness 与 DSH 各自独立产出、然后逐字段比对**。
> 但**两边产出同构的模型**这件事此前没有格式——**没有它，比对做不了，校验是空话**。
>
> **性质**：格式定义。**零实现**。DSH 出；实现方按它产出。

---

## 1. 三条硬要求（缺一不可）

| # | 要求 | 违反的后果 |
|---|---|---|
| **F1** | **可从代码机械导出**（或明确标 `inferred`/`unknown`） | 变成手写 README，`SM-4` 直接红 |
| **F2** | **每条带三态**：`verified` / `inferred` / `unknown` | 把推断写成事实（`SM-3` 红） |
| **F3** | **两边都能独立产出**（同一 commit，互不参照） | 只能自我确认，不是校验 |

---

## 2. 格式（JSON，单一文件）

```json
{
  "schema_version": "1.0",
  "subject": {
    "repo": "smithpeter/voicesign-harness",
    "commit": "<完整 sha>",
    "produced_by": "harness|dsh",          // F3：谁产的
    "produced_at": "<RFC3339>",
    "independence": "no_prior_access"       // 声明独立产出（F3）
  },

  "capabilities": [                          // 「我能做什么」——SM 组的核心
    {
      "id": "file.write",
      "kind": "tool",                        // tool | intent | command
      "source": "tools/registry.go",         // F1：真值指针
      "status": "verified",                  // F2：三态
      "limits": ["write:high"],              // 硬限制（SK-3）
      "needs_confirm": true,
      "note": ""
    }
  ],

  "dormant": [                               // 声明了但不可用（SK-2：不隐瞒、也不虚报）
    { "id": "FILE_WRITE", "source": "contract/contract.go", "status": "verified",
      "why": "有常量声明，无生产代码引用" }
  ],

  "unresolved": [                            // 认不出、对不上的（**必须显式列出**）
    { "id": "deploy", "seen_in": "space.Manifest.Tools / pipeline.go:503",
      "status": "unknown", "why": "域词表里的别名，工具注册表中不存在" }
  ],

  "domains": [
    { "name": "project", "source": "space/", "status": "verified",
      "perms": {"read": true, "write": true, "exec": true},
      "aliases": ["deploy", "http"], "risk_default": "medium" }
  ],

  "boundaries": [                            // 「我做不到什么」——SW 组的地基
    { "id": "no-deploy", "claim": "本 harness 无部署工具",
      "source": "tools/registry.go", "status": "verified" }
  ],

  "dependencies": [                          // 「我依赖谁」——SW-2 分对类别
    { "on": "aiops.peterzou.com", "kind": "gateway",
      "for": "模型 / 工具 / 内容", "status": "inferred",
      "note": "内网服务，DSH 未实测" },
    { "on": "peter (人)", "kind": "human", "for": "授权 / 产品取舍", "status": "verified" }
  ],

  "state": {                                 // 「我在哪」
    "tests": { "default": "23 pkg PASS", "vhsplan": "15 red", "vhs002": "7 red" },
    "branches": ["main", "integration/merge-20261003"],
    "source": "实跑命令", "status": "verified"
  },

  "open_questions": [                        // 「我不知道什么」——**最有价值的一节**
    { "q": "代码真值与意图真值不一致时以谁为准？",
      "why_it_matters": "C 类分歧的主要来源", "status": "unknown" }
  ]
}
```

---

## 3. 为什么这么切（每一节对应一条判据）

| 节 | 对应判据 | 作用 |
|---|---|---|
| `capabilities` | **SM-1 / SM-2 / SK-1 / SK-2** | 「有什么」——双向对账的主战场 |
| `dormant` | **SK-2** | 声明了但没引用 —— **不隐瞒、也不虚报** |
| `unresolved` | **SK-1** | **对不上就列出来**，不许悄悄丢 |
| `domains.aliases` | 已批准的取舍 | 别名**另立字段**，不混进能力 |
| `boundaries` | **SW-1 / SW-2 / SW-3** | 「做不到什么」——**有了它才能回答"该找谁"** |
| `dependencies` | **SW-2** | 把卡点分到 **人 / 网关 / 模型通道** 三类 |
| `state` | **SC-3** | 「我在哪」——计划可续的前提 |
| `open_questions` | **C 类分歧的来源** | **不知道就说不知道** |

> **`unresolved` 与 `open_questions` 是这份格式里最重要的两节。**
> 前六节是"我知道什么"，这两节是"**我不知道什么、我对不上什么**"。
> **`SM-3` 要区分事实与推断，而这两节就是"承认不知道"的落点。**
> 一个没有 `unknown` 的项目模型，**不是模型，是宣传**。

---

## 4. 一致性校验怎么做（格式定完就能做）

```
同一 commit
  ├─ harness → projmodel.json (produced_by: "harness", independence: "no_prior_access")
  └─ DSH     → projmodel.json (produced_by: "dsh",     independence: "no_prior_access")
                        │
                        ▼
              逐字段比对 → 分歧清单
                        │
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
     A 它错了        B 我错了        C 规格歧义
     （代码真值       （代码真值       （代码真值
       站它那边）       站我那边）       判不了）
```

### 4.1 为什么 `open_questions` 决定 C 类的数量

**若两边都诚实地写 `open_questions`，两份清单的差集就是最纯的 C 类。**
**若某一份 `open_questions` 为空——先怀疑它，不是先庆祝一致。**

### 4.2 指标

| 指标 | 定义 | 健康的样子 |
|---|---|---|
| **一致率** | 一致字段 / 总字段 | —— |
| **A/B/C 分布** | 三类分歧计数 | **C 类 > 0**（零 C 类要怀疑走过场） |
| **可核率** | `status=verified` 且 `source` 非空的字段占比 | 高 |
| **诚实率** | `status != verified` 的字段占比 | **不为零**（全 verified = 没有 unknown，可疑） |

---

## 5. 诚实边界

- **本格式是我的设计，未经任何一方实际产出验证。** 字段可能过多或过少。
- **`independence` 字段是自我声明，不可强制**——两边都可能无意中参照过对方。
  **真正的隔离要靠流程**（先各自产出、再交换），格式只能记录声明。
- **`source` 指向文件不等于真值稳定**：代码会变，`source` 会失效。
  **模型的时间维度本文未解决**（`VHS-SELFPROJ-001` §5 已登记）。
- **两份模型"字段对不上"时无法比对**——本节假设两边都按本格式产出。
  若一边不听格式，校验退化为人工阅读。
- **未覆盖**：历史决策（为什么这么做）、依赖的版本、性能基线。

---

## 6. 下一步

| 步 | 谁 |
|---|---|
| 1. 按本格式产出**第一份**（DSH 侧） | DSH |
| 2. 按本格式产出第一份（harness 侧），**不得参照 1** | 实现方 |
| 3. 比对 → A/B/C 归档 | DSH |
| 4. C 类升级为规格问题交 Peter | DSH |

**第 2 步之前，实现方要先把 `plan/` 的 15 条红判据里 `SM` 组实现**（`ExportManifest` 落地）——
格式里的 `capabilities` / `dormant` / `unresolved` / `domains` 正是它的直接产物。

---

*VHS-PROJMODEL-001 · 2026-10-03 · 由 DSH 出具。这是 VHS-SELFPROJ-001 一致性校验的地基；没有它，校验做不了。*
