# ASR-EXT-005 · AIOps 网关实测清单（给实现方的明确指令）

> **指示**：Peter 2026-10-03 ——「先查一下 aiops.peterzou.com 上这个东西，**你认真看一下**，
> 看完之后就**给他明确**。**这是你 DeepSeek 先搞完了，搞完了就教会他怎么干就行了。**」
>
> **性质**：**实测发现 + 给实现方的操作指令**。第 1 节是实测，第 3 节是命令，两者分开。

---

## 1. 实测发现（DSH 2026-10-03，真实调用）

### 1.1 网关就是它，不是模型中心

`aiops.peterzou.com` 返回的是 **AIOps 运行总览（内网大屏）**，而它的 `/api/*` 是**真 API**：

| 端点 | 实测 | 内容 |
|---|---|---|
| `GET /api/summary` | **200，22,029 字节** | 主机清单 + 实时状态 |
| `GET /api/cicd/status` | **200，1,356 字节** | CI/CD 台账 |
| `/api/email/config`、`/api/files/config` | 由 `configure.html` 引用 | 邮箱 / 文件服务配置 |
| `GET /configure.html` | 200 | 配置页 |

**⚠️ 关键：这些不需要 key 就能读**（`zone: "inner"` —— 内网区域）。

> 我此前误判了：把 `/api/health`、`/api/v1/models` 等**不存在的路径**当成了 SPA 兜底。
> **正确做法是读页面的 JS，看它 `fetch` 什么。**
> 教训（写进技能）：**探接口不要猜路径，要看调用方。**

### 1.2 `/api/summary` 的真实结构

```json
{ "ok": true, "ts": "...", "zone": "inner",
  "hosts": {
    "trelva": { "hostname": "VM-4-15-ubuntu",
                "purpose": "trelva（生产中枢：研究引擎/Koyee/peterzou.com/Headscale 控制面）",
                "domain": "trelva.ai", "status": "OK",
                "cpu": 70.8, "mem": 17.0, "disk": 84, "disk_free": 12.4,
                "ports": [22,53,80,443,3000,8080,...], "services": 36,
                "region": {"country":"US","city":"Santa Clara","provider":"tencent-intl"},
                "network": {"openai":"ok","github":"ok","outbound":"ok"},
                "env":"prod", "role":"research" },
    "center": { "hostname": "VM-0-14-ubuntu",
                "purpose": "center（voxsign.net 生产中心）",
                "domain": "aiops.peterzou.com", ... }
  } }
```

**这是一份"我有哪些机器、它们现在怎么样"的机器可读真值** ——
正是 `VHS-PROJMODEL-001` 里 `dependencies` 与 `state` 两节要的**外部世界模型**。

### 1.3 `/api/cicd/status`

```json
{ "ok": true, "zone": "inner", "status": "idle",
  "current_tag": "release/runtime-2026-10-02-01",
  "ledger": [ { "ts":"...", "tag":"release/runtime-2026-10-01-01",
                "status":"dry-run", "note":"build+gate passed, no compose up" },
              { "ts":"...", "tag":"release/runtime-2026-10-01-01",
                "status":"failed", "note":"deploy.sh exited non-zero" } ] }
```

**这是部署门的真值**——对应服务的 `cicd` 项（`type: internal`, `status: partial`）。

### 1.4 服务注册表（比接口定义更可靠）

`~/.aiops/service-registry-live.json` → **18 个服务**，带 `aliases` / `entry` / `status` / `desc`：

```
external : aiops-portal / model-center / main-site / translate / listen / vocab /
           consultant / zhiji / file-store
internal : cicd / headscale / collector / backup-pool / email-reader
harness  : research-opc / deepseek-harness / grok-bot / workbuddy
```

**这份表本身就是"外部有什么"的真值源**，比我在 `ASR-EXT-002` 里猜的接口定义可靠得多。
其中 `deepseek-harness` 写着「**trelva 研究引擎执行层（DeepSeek V4.1 Flash，服务端）**」
——**印証了模型固定**（`ASR-MODEL-01`）。

---

## 2. 那把 key 的结论（如实）

| 测试 | 结果 |
|---|---|
| `model.peterzou.com/v1/models` + `Bearer` | **401 `invalid_api_key`** |
| 同上 + `X-API-Key` | **401 `invalid_api_key`** |
| `aiops.peterzou.com/api/summary` | **200（无需 key）** |

**结论**：
- `peter-mac.key`（42 字符，前缀 `aio…`）**不是 model-center 的 key**；
- **aiops 网关的读接口不需要 key**（内网 zone）；
- **写操作 / 模型调用是否需要 key、用哪把 —— 未知，需 Peter 明确。**

**已安全安装**：`.env`（权限 600，**已确认被 `.gitignore` 忽略**，未进 git）。

---

## 3. 给实现方的明确指令（**这就是"教他怎么干"**）

### 3.1 立即可做（不需要 key，不需要授权）

**把 aiops 网关接成"外部世界模型"的数据源**：

```
GET https://aiops.peterzou.com/api/summary      → 机器 / 服务 / 状态
GET https://aiops.peterzou.com/api/cicd/status  → 部署台账
```

**用途**（直接对应 `VHS-SELFPROJ-001` 的三层含义）：

| 产出 | 用哪份数据 |
|---|---|
| `dependencies`（我依赖谁） | `summary.hosts[*].services` / `ports` / `network` |
| `state`（我在哪） | `cicd.current_tag` / `ledger` |
| **`③ 知道遇到问题该找谁`** | `hosts[*].purpose` + 注册表的 `aliases` —— **它就能回答"这事该找谁"** |

### 3.2 实现要求（按既有规范，不新增约束）

| # | 要求 | 依据 |
|---|---|---|
| **A1** | **只缓存元数据，不囤全量响应**；缓存可重建可清除 | `EXT-04` |
| **A2** | 每条数据带 `source`（端点 + 抓取时间）+ `status` | `VHS-PROJMODEL-001` F1/F2 |
| **A3** | **读失败必须 fail-open 且标 `unknown`**，不得静默当"没有" | `ASR-MODEL-02` L3 同理 |
| **A4** | **不得把读接口的成功当成写权限的证明** | 红线：默认拒绝 |
| **A5** | **不要猜路径**——需要新端点时，**读页面的 `fetch` 调用** | 本文 §1.1 的教训 |
| **A6** | 单一出网配置（当前 `aiops.peterzou.com`），**不硬编码多个地址** | `ASR-EXT-004` G1/G2 |

### 3.3 判据（先红）

| 编号 | 判据 |
|---|---|
| **EXT-05** | `summary` 解析成功时，`dependencies` 必须包含实际的 host 列表，且每条带 `source` |
| **EXT-06** | 端点不可达时，模型**不得**把这些依赖标成"不存在"，必须标 `unknown`（`A3`） |
| **EXT-07** | **读接口 200 不得被解释为"我可以部署/写"**——`boundaries` 仍须保留 `no-deploy`（`A4`） |
| **EXT-08** | 缓存目录可删可重建；删掉后重新抓取结果一致（`A1`） |

### 3.4 停止条件（`技能 §11`）

**遇到下列情况立即停手上报，不许往下做**：
- 需要**写操作**或**模型调用**但没有明确授权的 key；
- 需要**未在注册表里出现**的端点；
- 需要判定"某条数据该不该信"而规格未覆盖。

---

## 4. 诚实边界

- **我只实测了 3 个端点**（`/api/summary`、`/api/cicd/status`、`/configure.html`）。
  `/api/email/*`、`/api/files/*` **只见到引用，未调用**。
- **`/api/summary` 的数据我没有独立核实**——那是网关自报的。**自报 ≠ 验证**（沿用本项目原则）。
- **写操作与模型调用的鉴权形态完全未知**。
- 注册表文件是**本机快照**（`ts: 2026-10-01T20:05:34Z`），**可能已过期**。
- 本文 §3 是指令，**不是已实现**。

---

*ASR-EXT-005 · 2026-10-03 · 由 DSH 出具。§1 实测、§2 结论、§3 指令，三节分开写。*
