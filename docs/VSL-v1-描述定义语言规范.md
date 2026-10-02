# VSL v1 —— 描述定义语言规范（VoiceSign Specification Language）

- 版本：v1.0 · 日期：2026-10-02 · 作者：Peter Zhou × MainAgent（Claude/Codex/Cursor 多轮评审后定稿）
- 定位：**代码语言与描述定义语言分离**后的"描述定义语言"——先定义、后代码

---

## 1. 两种语言的边界（核心前提）

| 语言 | 性质 | 执行者 | 示例 |
|---|---|---|---|
| **代码语言** | 严格逻辑、确定性 | 机器（Go/JSON schema） | `func spaceCheck(...) bool` |
| **描述定义语言（VSL）** | 软约束、规则、规范性 | 大模型（理解+执行） | `.space` `.dict` `.contract` `.policy` |

**使用前提**：大语言模型对 VSL 规范理解清楚——知道"说什么、意图是什么"。本语言成立的基础就是这个前提（2026 起 LLM 对结构化规范的理解已足够）。

**工作顺序**：先定义（VSL 讲清楚）→ 再落代码（硬代码按定义实现）。提问也更好：定义越清楚，模型执行越准。

## 2. 词汇表（VSL 名词）

| 词 | 含义 |
|---|---|
| 域 space | 活动空间边界（manifest） |
| 实体 entity | 代指对象（实体词典） |
| 契约 contract | 工具能力与副作用 |
| 意图 intent | 8 类动作（EDIT/DEBUG/QUERY/TEST/COMMIT/DEPLOY/NOTE/ASK） |
| 验收 acceptance | 完成标准 |
| 指代 reference | 模糊→精确的消解 |
| 确认 confirm | 共识机制（回问/强确认/固化） |
| 轨迹 trace | 证据记录（append-only） |

## 3. 语法（一句话结构）

```
[在<域>里] <动作> <对象>（对象可代指）   →   意图JSON
```

- 动作动词 → 意图：改→EDIT · 查→QUERY · 修→DEBUG · 跑→TEST · 记→NOTE · 发→DEPLOY · 提交→COMMIT · 问→ASK
- 对象可代指：实体名 / 别名 / "它" / "那个"（域内最近实体）
- 域省略 → 回落 global（只读）

## 4. 共识机制（语言的目标：四种共识）

1. **决策标准共识**：验收 + 确认策略（什么算完成、什么时候问）
2. **代指对象共识**：指代消解 + 确认后固化词典
3. **活动空间共识**：域 manifest（能碰哪、能干什么）
4. **执行语义共识**：契约（工具做什么、副作用）

规则：低置信 → 回问；高风险 → 强确认（展示 diff）；确认 → 固化；拒绝 → 回落 global 只读。**指代不清绝不瞎执行。**

## 5. 定义块（VSL 文件类型）

| 文件 | 内容 |
|---|---|
| `*.space` | 域定义（scope/tools/perms/context/acceptance） |
| `*.dict` | 实体词典（名称/别名/属性/项目） |
| `*.contract` | 工具契约（能力/参数/副作用/允许域） |
| `*.policy` | 确认策略（三信号矩阵/权限交集/逃逸规则） |
| `*.accept` | 验收模板（指标/样例） |
| `*.ogsm` | 项目目标定义（O/G/S/M） |

## 6. 实例：VoxSign Harness 项目定义（dogfood 第一例）

### 6.1 OGSM 定义 `harness.ogsm`
```
O: 两周内 20 个真实任务语音闭环跑通
G: 风险分级 + 独立校验 + 域边界 + 指代消解
S: 方向验证(20任务) → 域注册表+域校验 → 指代消解 → 分级+verify → 四元缓存
M: 连续5天×4任务语音完成 / 零手动重查 / 单工对比3任务中2个更快
```

### 6.2 域定义 `harness.space`
```
name: voicesign-harness
type: project
scope: /Users/zouyongming/VoxSign/** + 本项目目录
tools: search, file, git, test, verify
perms: read+write; exec: ["go build","go test"]
context: [project-map:harness, decisions:harness]
acceptance: go test ./...
risk_default: auto
```

### 6.3 实体词典 `harness.dict`
```
冀总 → 冀总(Amber, amberqiang13@gmail.com, 沙特医疗耗材厂房咨询)
美墅 → Mansour(SFDA 项目合作者)
季总 → 冀总(同音纠正)
model.peterzou.com → 模型中心(OpenAI兼容网关)
voxsign-ai / trelva / center → 服务器(域 external)
```

### 6.4 契约 `harness.contract`（六工具 @1.0）
```
git    : status/diff/log/commit/checkout(revert)        [project, sandbox]
file   : read/write/patch(+diff 预览)                    [project, vault-notes]
search : grep/符号定位                                    [全部域]
test   : go test ./...                                    [project]
run    : 白名单命令+超时                                  [project, sandbox]
verify : 独立校验(读实际状态 diff 预期，不读自报)          [全部域]
```

### 6.5 确认策略 `harness.policy`
```
三信号：可逆性(硬事实) × 影响面(静态:引用面/热度/测试覆盖) × 置信度
矩阵：
  可逆+小+高 → 自动执行+标待抽查
  可逆+小+低 → 自动执行+待抽查+回执高亮
  可逆+中   → 展示 diff 摘要 → 轻确认
  可逆+高   → 强确认(diff 预览+影响分析)
  不可逆    → 永远人工确认(硬规则，不可被学习掉)
权限交集：有效权限 = 平台 ∩ 域 ∩ 契约声明 ∩ 本次授权
逃逸规则：单次操作按(操作类型+资源类型)独立评估，宽松域不能掩盖高风险
```

### 6.6 验收 `harness.accept`
```
V0: 连续5天 × 每天≥4个真实任务语音完成
  + 没有一次因"不信任"手动重查(有 verify)
  + 单工对比: 3 个任务 ≥2 个语音更快或等快
  + 边界: 越界操作 100% 被 space_check 拦截(测试覆盖)
```

## 7. 使用方式

1. **定义先行**：先写 `.space/.dict/.contract/.policy/.accept/.ogsm` → 再落代码
2. **会话循环**：意图解析读定义 → 执行 → 共识更新（确认固化词典/缓存/契约）
3. **演化**：定义块版本化（@1.0 → @1.1），只增不改 + supersede 归档
4. **自举**：REGISTER_TOOL 走通（语音新增契约）→ 定义语言驱动自身演进

## 8. 自然语言优先（v1.1 修订）

**每个定义块必须能用一句话自然语言讲清**——你未来说的全是自然语言，VSL 是它的规范层。

规则：
1. 定义块 = **一句话人话在前**（"先记录碎片，一个字不改"）→ 结构化字段在后（purpose/input/output/consensus/acceptance 自动对应）
2. 新增机制 **对比归因**：control 域执行后对账"是模型/预算没想明白，还是执行没做到"，结论回写 discuss（建模）
3. record 域约定：**只追加不改写**，原文与结构化并存互为索引
4. 三域骨架（harness-core 大域）：record（碎片→素材）/ discuss（想法→模型）/ control（意图→结果）——见《VSL-大域定义-Harness三域.md》

---
*关联：详细设计-语音驱动开发-v2定稿-20261002.md（架构）、research-inputs/12~14（域/指代/语言评审）、VSL-大域定义-Harness三域.md（定义实例 2）*
