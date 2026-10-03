# ASR-EXT-006 · 统一入口打通实录（含真实模型清单与鉴权）

> **触发**：Peter 2026-10-03 —— VoiceSign Harness 专用 Key 已创建，模型中心并入 AIOps 统一入口。
> **性质**：**实测记录 + 对旧文档的修正**。DSH 2026-10-03 真实调用验证。

---

## 1. 实测：全部打通（真实调用）

| 端点 | 结果 |
|---|---|
| `GET /api/model/models` | **200** — 15 个模型 |
| `POST /api/model/chat` | **200**，耗时 **1.23s**，OpenAI 兼容格式 |
| `GET /api/services` | **200** — 服务注册表（带 aliases） |
| `GET /api/hosts` | **200** — 主机与状态 |
| `GET /api/summary` | **200**（前一轮已测） |
| `GET /api/cicd/status` | **200**（前一轮已测） |

**鉴权**：`Authorization: Bearer <AIOPS_KEY>`，**一把 key 调全部**。

**chat 响应样例**（截断）：
```json
{ "object":"chat.completion", "model":"deepseek-flash",
  "choices":[{"message":{"role":"assistant","content":"收到"},
              "finish_reason":"length"}],
  "usage":{"prompt_tokens":35,"completion_tokens":16,"total_tokens":51,
           "completion_tokens_details":{"reasoning_tokens":14}} }
```

**它是标准 OpenAI 兼容**：`chat.completion` / `choices[].message.content` / `usage`。
→ **既有 `provider/` 的 OpenAI 兼容客户端可直接复用**（无需新写协议）。

---

## 2. ⚠️ 修正：`ASR-MODEL-01` 记的模型标识是**错的**

`ASR-MODEL-01` 按 Peter 原话记的是 `deepseek-v4.1-flash`。
**实测的 15 个模型里没有这个名字。**

**真实清单（`GET /api/model/models`，逐字）**：

| # | id | # | id |
|---|---|---|---|
| 1 | `deepseek-chat` | 9 | `gpt-4o-mini` |
| 2 | **`deepseek-flash`** | 10 | `gpt-5.4-pro` |
| 3 | `deepseek-reasoner` | 11 | `gpt-5.5` |
| 4 | `deepseek-v4-pro` | 12 | `gpt-5.6-luna` |
| 5 | `gemini-2.5-flash` | 13 | `gpt-6-luna` |
| 6 | `gemini-2.5-pro` | 14 | `qwen3.8-flash` |
| 7 | `gemini-3.8-flash` | 15 | `qwen3.8-max-0902` |
| 8 | `gpt-4o` | | |

**结论**：Peter 说的「DeepSeek V4.1 Flash」= **`deepseek-flash`**。
（旁证：服务注册表里 `deepseek-harness` 写着「trelva 研究引擎执行层（DeepSeek V4.1 Flash，服务端）」）

**对实现方的要求**：**用 `deepseek-flash`，不要用 `deepseek-v4.1-flash`**（后者不存在）。

---

## 3. 三条通道的模型安排（按 Peter 前轮答复 + 本轮实测）

Peter 说过：`diagnose`/`learn` 的模型**会变**，`diagnose` 可能用 JEV；做不到就用默认。

| 通道 | 模型 | 依据 |
|---|---|---|
| `default` | **`deepseek-flash`** | Peter 指定；实测存在 |
| `diagnose` | **待定**（候选：`deepseek-reasoner` / `deepseek-v4-pro`；JEV 走 `/v1/decisions` 类比面） | Peter 未定 → **`enabled:false` 保持** |
| `learn` | **待定**（候选：`deepseek-reasoner` 等更强档） | 同上 |

**硬要求不变**（`ASR-MODEL-02` L1）：
- **通道名固定**（`default`/`diagnose`/`learn`），**底层模型可变**；
- **评测时三条通道必须指向同一模型**，否则归因失效。

---

## 4. Key 的安装（已完成，安全）

```
.env（仓库根，权限 600）
  AIOPS_KEY       = vh-…（46 字符，**不入档、不入 git**）
  AIOPS_GATEWAY   = https://aiops.peterzou.com
  MODEL_CENTER    = https://aiops.peterzou.com/api/model
  MAIN_SITE       = https://peterzou.com
```

**安全校验（已实测）**：
- `.gitignore:4  *.env` → **已忽略** ✅
- `git status` → **不含 .env** ✅

**⚠️ 纪律**：key **永不**写入仓库、文档、commit message、日志。
实现方读它**只从 `.env` 或环境变量**，**不得硬编码**。

---

## 5. 现在解除的阻塞

| 原阻塞 | 现状 |
|---|---|
| 无 key → 三条通道跑不了 | ✅ **已解** |
| 模型标识不确定 | ✅ **已解**（`deepseek-flash`） |
| 网关接口不明 | ✅ **已解**（6 个端点实测） |
| 写操作鉴权 | ⏳ 有"临时写 key 申请"接口，**未见文档，未测试** |

---

## 6. 诚实边界

- **`/api/model/chat` 我只发了一次最小请求**（`max_tokens:16`，返回 `finish_reason: length`）。
  **流式、长上下文、错误码、限流，全部未测。**
- **15 个模型我只列了 id**，未验证各自真实能力/价格/上下文窗口。
- **写操作（发布/部署/申请写 key）我一次都没调** —— 按 `ASR-EXT-005` A4：**读通不等于有写权**。
- `.env` 里的 key 是**本机文件**；**换机器要重新配**（这正是"一个地址一把 key"的成本）。
- 本轮**未**读 Peter 提到的 GitHub 指南（`ops/aiops/docs/VOXSIGN-HARNESS-GUIDE-2026-10-03.md`）——
  那是**另一个仓库**，需要单独取。

---

*ASR-EXT-006 · 2026-10-03 · 由 DSH 出具。§2 是对 ASR-MODEL-01 的**修正**，不是补充。*
