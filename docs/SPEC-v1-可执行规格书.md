# VoxSign Harness · SPEC v1（可执行规格书）

- 版本：v1.0 · 2026-10-02 · 依据：设计总览 v2 / VSL v1-v2 / 边界审计方法论
- 北极星：**认知闭环周期**——record → discuss → control → 归因，转一圈的速度 = 10 倍指数
- 本 SPEC = 定义层（先定义后代码）；每模块按四层判据（schema / 边界五轴 / 判断层 / 用例）过，填不出标 UNKNOWN

---

## 1. 代码基线（M1 已交付，接续点）

| 包 | 职责 | 状态 |
|---|---|---|
| main.go | 入口（CLI） | 已实现 |
| input/ | ASR 原文留底 → 纠错（clean.go）→ 意图解析（intent.go） | 已实现（9.9µs） |
| memory/dictionary.go | 实体词典（美墅→Mansour 等） | 已实现 |
| provider/openai.go | OpenAI 兼容客户端（模型中心） | 已实现 |
| router/ | 路由 | 已实现 |
| trajectory/ | 轨迹 JSONL append-only | 已实现 |
| contract/ | 契约基础 | 已实现 |
| config/ | 配置 | 已实现 |
| bench/ e2e/ | 基准与端到端演示 | 已实现 |
| server/ tools/ | 空（M2 目标） | 未实现 |

接续点：M2 在 input/memory 之上新增 space/risk/verify/search 包；server 放手机 API 面。

## 2. 意图 Schema（8 类）

```json
{
  "intent": "EDIT",
  "raw_text": "在 VoxBuyBot 里把错误提示改中文",
  "cleaned_text": "在 voxbuybot 里把错误提示改成中文",
  "space": "project:voxbuybot",
  "target": { "entity": "voxbuybot", "ref_type": "explicit" },
  "params": { "action": "replace", "object": "错误提示", "value": "中文" },
  "boundary": { "scope": "voxbuybot/**", "exclude": [".env*", "node_modules"] },
  "risk": { "reversible": true, "impact": "small", "confidence": 0.9 },
  "confirm": "auto",
  "acceptance": "目标文件改完，无越界改动",
  "context": ["project-map:voxbuybot"]
}
```

### 意图分类与动作动词
| 意图 | 触发词（示例） | 反例（不算） |
|---|---|---|
| NOTE 记 | 记一下 / 存 / 记 | "删掉那条" → 不是记，是 EDIT/DELETE（未定义则 ASK） |
| QUERY 查 | 查 / 看 / 找 / 上次 | "查一下能不能…" → 是 ASK |
| EDIT 改 | 改 / 换成 / 把…改成 | "帮我看看这段" → QUERY |
| DEBUG 修 | 修 / 报错 / 为什么失败 | "修一下这个 bug 的思路" → ASK 还是 DEBUG？→ 低置信回问 |
| TEST 跑 | 跑测试 / 测一下 | "测一下性能" → RUN（bench） |
| COMMIT 提交 | 提交 / 推 | 提交前未跑测试 → 强确认提示 |
| DEPLOY 发 | 发 / 上线 / 部署 / 生成报表 | "发个想法" → NOTE |
| ASK 问 | 为什么 / 怎么办 / 你觉得 | — |

**判断层**：意图低置信（<0.7）→ 回问选项，不猜；意图与域冲突（如 NOTE 指向 project）→ 按域语义修正并回显。

## 3. 域 Manifest Schema（.space）

```yaml
name: voicesign-harness
type: project            # project | sandbox | vault-notes | vault-creds | external | global
scope: ["/Users/zouyongming/DoubaoWork/chats/2026-10-01/new-chat-23/voicesign-harness/**"]
tools: ["search", "file", "git", "test", "verify"]
perms:
  read: true
  write: true
  exec: ["go build", "go test"]
context: ["project-map:harness", "decisions:harness"]
acceptance: "go test ./..."
risk_default: auto
cross_refs: []           # 跨域引用声明（如 vault-creds 只读引用）
```

**边界五轴判据**（每域必填五项，缺一项 = UNKNOWN）：
- 范围 scope：可访问路径/资源
- 排除 exclude：显式不可碰（.env*、密钥、外部发布）
- 权限 perms：读/写/执行白名单
- 验收 acceptance：域内任务完成判据
- 上下文 context：注入的认知切片

**六域初版**：global（只读兜底）· project:voicesign-harness · project:voxbuybot · sandbox · vault-notes · vault-creds · external（读取/发布按动作区分）。数量 3–7 达标。

## 4. 契约 Schema（.contract）

```yaml
name: git
version: "@1.0"
caps: ["status", "diff", "log", "commit", "checkout"]
params: { path: "string,required", range: "string,optional" }
side_effects: ["修改工作区/索引", "commit 不可逆(本地)"]
allowed_spaces: ["project", "sandbox"]
risk: { commit: "medium", checkout: "medium", status: "none" }
```

六工具契约 @1.0：git / file / search / test / run / verify。
- verify 契约 = 独立校验器：读实际状态 diff 预期，**不读自报**（防自证）
- REGISTER_TOOL 契约 = 语音注册新契约（自举充分条件）

## 5. M2 目录结构（目标）

```
voicesign-harness/
  main.go            # 入口（已有）
  input/ memory/ provider/ router/ trajectory/ contract/ config/   # 已有
  space/             # 新增：space.go（registry+check）
  risk/              # 新增：risk.go（三信号，<150 行）
  refer/             # 新增：reference.go（词典+上下文消解）
  verify/            # 新增：verifier.go（独立校验器）
  search/            # 新增：search.go（grep/符号定位）
  cache/             # 新增：quad.go（四元组缓存）
  server/            # 已建空：api.go（手机 HTTP 端点）
  tools/             # 已建空：工具实现
```

## 6. 验收用例（核心集，正例/反例/边界）

| # | 模块 | 用例 | 期望 |
|---|---|---|---|
| 1 | input | "记一下冀总那个厂房下周一出报价" | NOTE + 指代冀总 + 时间锚点 |
| 2 | input | "美墅那个项目聊到哪了" | 词典：美墅→Mansour（ASR 纠错） |
| 3 | refer | "那个文件改好了吗"（跨域歧义） | 显式回问"哪个域？"，不猜 |
| 4 | refer | "它"指上一条回执文件 | 上下文命中，不再问 |
| 5 | space | project 域内 EDIT 放行 | 执行 |
| 6 | space | vault-creds 外发请求 | BOUNDARY_VIOLATION 拦截 |
| 7 | space | 域 manifest 与实际路径漂移 | 检测并失效，回问重建 |
| 8 | risk | 可逆+小+高 | auto 执行 |
| 9 | risk | 不可逆（DELETE/外发） | 永远 human，不可被学习掉 |
| 10 | verify | 改完跑测试 | 读实际结果，不读自报 |
| 11 | receipt | 回执格式 | 一屏 4 行：动作/文件/结果/撤销 |
| 12 | loop | 执行后归因 | 模型错 vs 执行错，回写 discuss |

## 7. M2 任务卡（12 步：输入/输出/完成定义）

| # | 任务 | 输入 | 输出 | 完成定义 |
|---|---|---|---|---|
| 1 | 20 真实任务方向验证 | M1 demo + 真实工作场景 | 20 条验证记录 | 每条有 意图JSON+回执+归因 |
| 2 | space registry | 六域 manifest | registry.go + 域表 | 加载/版本/漂移检测测试绿 |
| 3 | 指代消解 | 词典+上下文 | refer/ | 用例 3/4 通过 |
| 4 | space_check | registry+意图 | 执行前校验 | 用例 6 拦截 |
| 5 | 风险分级器 | 三信号 | risk/ | 用例 8/9 通过 |
| 6 | verify 契约 | 校验器 | verify/ | 用例 10 |
| 7 | search 契约 | grep 定位 | search/ | 冒烟 |
| 8 | 四元组缓存 | 意图/域/权限/指代 | cache/ | 命中率+失效测试 |
| 9 | REGISTER_TOOL | 语音注册 | 自举验证 | 语音新增契约成功 |
| 10 | 手机 API 面 | HTTP 端点 | server/ | 手机 curl 回执 |
| 11 | 每日摘要 | 轨迹聚合 | 摘要输出 | 手机可读 |
| 12 | 单工对比 | 3 任务 | 对比记录 | ≥2 个语音更快/等快 |

## 8. UNKNOWN 清单（本轮填不出，诚实暴露）

1. M1 的 router/provider 内部实现细节（next 轮读代码补齐）
2. 实体词典初版完整清单（除冀总/美墅外待定）
3. input/intent.go 现有意图分类是否与 8 类一致（需读码）
4. 手机 API 面的认证方式（先按内网 + 简单 token）
5. 每日摘要的具体聚合规则（可后补）
