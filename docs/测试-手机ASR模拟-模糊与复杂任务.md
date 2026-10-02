# 测试：手机 App ASR 模拟 —— 模糊识别 × 复杂任务

- 日期：2026-10-03
- 被测：voicesign-harness 主管线（`pipeline.Run`，即 server `/v1/tasks` 背后的同一条链）
- 性质：**只新增测试，未改动任何产品代码**
- 新增文件：`e2e/asrsim_test.go`（绿）、`e2e/asrsim_gaps_test.go`（红）

## 0. 场景

模拟 iPhone 把 ASR 出来的**文本**投给 harness，不涉及音频：

```
用户说话 → iOS ASR 出文本 → App POST /v1/tasks → 清洗/纠错 → 意图分类 → refer → 域/风险门 → 执行 → 四行回执
                                 └── 本套件从这里开始，直接喂文本给 pipeline.Run
```

两条主线对应产品最核心的两个能力：

1. **模糊识别**：指代不明、元指令、填充词、极短句、否定 —— 系统该判对，或**该回问**；绝不允许猜错后静默执行。
2. **复杂任务**：一条口述里的多动作、顺序、条件、自我修正 —— 系统该拆解，或**明确告知一次只能做一件**。

## 1. 怎么跑

```bash
# 只看契约/回归（绿）—— 可用于门禁
go test ./e2e ./server -run TestASRSim -skip TestASRSimGap -v

# 只看已知缺口（红）—— M8 进度看板
go test ./e2e ./server -run TestASRSimGap -v

# 全仓门禁（保持绿）
go test ./... -skip TestASRSimGap
```

设计：**不依赖真实 LLM**。`Options.Providers` 为 nil，走纯规则层，确定性、可重放、无外发。
不可逆意图（COMMIT/DEPLOY）在测试里一律 `ConfirmFn → false`，不会真提交/真部署。

两个层次各管一段：

| 套件 | 打到哪一层 | 回答的问题 |
|---|---|---|
| `e2e/asrsim_*_test.go` | 直接调 `pipeline.Run` | 理解与拆解对不对 |
| `server/asrsim_*_test.go` | 真起 HTTP，走 `POST /v1/tasks` + 轮询 | **手机上到底看到什么**（回执、决策卡、按钮、串号） |

`server` 那一套刻意不复用内部 `taskState`，而是按 `writeTaskView` 的真实字段建模
（`task_id/status/role/question?/options?/receipt?/attribution?`），否则会掩盖手机实际收到什么。

## 2. 绿套件：已经成立、必须保持的契约

### e2e（6/6 PASS）

| 用例 | 锁住的行为 |
|---|---|
| `TestASRSimEmptyUtteranceAsksWithoutExecuting` | 空/纯空白指令必须回问且不放行 |
| `TestASRSimReceiptAlwaysFourLines` | 任何结果都给手机四行回执（动作/文件/结果/撤销） |
| `TestASRSimIrreversibleNeverExecutesWithoutHuman` | COMMIT/DEPLOY 未获人工放行绝不执行 |
| `TestASRSimDomainGateDeniesOutOfScope` | 默认拒绝：无域归属的写操作被域门拦下 |
| `TestASRSimFuzzyAcceptsIntentOrAsk` | 11 条模糊句只允许两种结局：判到可接受意图，或明确回问 |
| `TestASRSimMetaInstructionNotTreatedAsRunnableCommand` | 元指令不得表现为"已执行" |

### server（HTTP，6/6 PASS）

| 用例 | 锁住的行为 |
|---|---|
| `TestASRSimPhoneFuzzyNeverBlankOrStuck` | 13 条 ASR 模糊句不得让手机出现空白页或卡住 |
| `TestASRSimPhoneReceiptAlwaysFourLines` | done 的 `receipt` 必须四行齐全（M7 空渲染的数据侧根因） |
| `TestASRSimPhoneAttributionAlwaysClassified` | done 必须有归因六格 |
| `TestASRSimPhoneAskOptionsWellFormed` | 候选按钮 ID/Label 非空、≤4 个 |
| `TestASRSimPhoneAnswerEndpointAccepts` | 澄清回答的管道通、task_id 不漂移 |
| `TestASRSimPhoneConcurrentTasksDoNotCross` | 连说三句不串号，各拿各的回执 |

## 3. 红套件：8 个缺口（M8 待办）

每条都有实测失败信息；按严重度排序。

### G1 否定不被理解（严重度：高 · 安全）
- 输入：`不要删除那个文件`、`别删掉这条记录`
- 实测：判 `EDIT / params.action=delete / conf=0.90` —— 是**可执行的删除**。
- 今天没删成，靠的是域门兜底（`BOUNDARY_VIOLATION`）和指代回问，**不是意图层看懂了"不要"**。
- 根因：`input/taskintent.go:127` 的删除仲裁只看 `deleteTriggers`，全表没有任何否定词；VoxSign 侧 `understanding/parser.py` 有 `_NEGATION_PATTERNS`，本仓没有对应物。
- 最小改法：仲裁最前面加否定前缀检测（`不要|别|不用|先别|不需要`），命中即标 `state=negated`，不进执行通道。

### G2 关键槽位缺失不追问（严重度：高）
- 输入：`改一下`、`修`、`查`
- 实测：`EDIT 0.85`、`DEBUG 0.85`、`QUERY 0.85`，`Ask` 全为空，直接进执行通道。
- 根因：单类触发恒 0.85（`taskintent.go:174-192`），默认阈值 0.6（`taskintent.go:74-80`），
  而 `askForKind` 只在 `conf < threshold` 时触发（`taskintent.go:256-260`）——**默认配置下这条回问分支几乎不可达**。
- 最小改法：把"缺必需槽位回问"与"低置信回问"拆成两条独立路径；EDIT 要 object、DEBUG 要报错、SHELL 要 cmd。

### G3 元指令被当成可执行任务（严重度：高）
- 输入：`开始测试`、`继续测试`
- 实测：判 `TEST 0.85`，`test_kind="go test ./..."` —— 真的去跑测试。
- 根因：`testTriggers` 含裸词"测试"（`taskintent.go:95`），分类器没有"对话控制层/元指令层"。
- 最小改法：进入八类分类**之前**加元指令判定（`开始|继续|先这样|停|再说一遍|推进`）→ 走控制流，不走任务执行。

### G4 澄清原因被下游覆写（严重度：中）
- 输入：`嗯那个呃记一下`
- 实测：`intent=UNKNOWN`，但 `Ask="你说的「那个」指的是哪个？"` —— 分类器本来要说"我不知道你想干嘛"。
- 根因：`refer.ResolveOptions` 无条件写 `it.Ask`（`refer/refer.go:231`），不检查上游是否已有 Ask。
- 最小改法：refer 只在"确有指代且意图非 UNKNOWN"时覆写；否则保留原 Ask。

### G5 复杂口述多动作被静默丢弃（严重度：高 · 能力）
- 输入与实测：
  | 输入 | 实测 | 被静默丢弃 |
  |---|---|---|
  | `查一下库存，然后记一下结果，最后提交` | 只执行 NOTE（真写了 notes.md） | QUERY、COMMIT |
  | `把报价模板改成新的公司抬头，然后跑一下测试` | 只执行 EDIT | TEST |
  | `跑一下测试然后提交这批改动` | 只执行 TEST | COMMIT |
  | `把主栈改成中文然后部署到服务器` | 只执行 EDIT | DEPLOY |
  | `记一下明天开会然后查一下上次的报价` | NOTE + 指代回问（兜住了） | QUERY |
- 根因：`contract.Intent` 单意图（`contract/contract.go:203-222`），`ClassifyTask` 首个命中即 `return`（`taskintent.go:172-193`），无子句切分、无"其余未执行"提示。
- 影响：**用户说三件事，系统做一件，且不吭声**。这是"复杂任务能力"最大的洞。
- 最小改法（M8 最大件）：按 `然后/再/最后/接着/，` 切分子句 → 逐句分类 → 产出任务序列；做不到时回问"一次只做一件，先做哪个？"。

### G6 条件句被无条件执行（严重度：中）
- 输入：`如果测试通过就提交`、`测试过了就部署`
- 实测：判 `TEST 0.85` 直接跑，无 Ask；`contract.Intent` 无任何字段能承载前置条件。
- 最小改法：识别 `如果/若/只要/除非` 条件标记 → 不直接执行，产出"待条件确认的计划"。

### G7 自我修正不被理解（严重度：中）
- 输入：`记一下A，不对，改成记B`
- 实测：`EDIT`，`object="记一下A，不对"` —— 把修正前的整段吞进了目标槽位。
- 根因：`extractReplace` 取"改成"之前的整段当 object（`taskintent.go:339-359`），无修正信号识别。
- 最小改法：`不对|说错了|改成|重新说` 作为修正标记，丢弃其前的子句。

### G8 澄清续跑不收敛：死循环（严重度：高 · 闭环断点）
- 位置：HTTP 层（`server/asrsim_gaps_test.go`），因为这是**手机才看得见**的断点。
- 复现（实测三遍逐字相同）：
  ```
  POST /v1/tasks   {"text":"把这个改一下"}
  → need_ask       "你说的「这个」指的是哪个？请再说清楚一点"
  POST .../answer  {"answer":"main.go"}
  → need_ask       同一句问题，一字不差
  ```
- 根因：`handleAnswer` 把续跑文本拼成 `原文 + " 澄清：" + 答案`（`server/server.go:515-518`），
  而 refer 仍拿 `CorrectedText` 里的"这个"找候选；`Recent` 为空（答案没有变成候选来源），
  于是 refer 又写出同一句 Ask。既没把答案喂给 refer，也没有"同一问题重复 N 次就升级/放弃"的退避。
- 影响：**手机走到这一屏就再也走不出去** —— 语音闭环在需求澄清处断掉。
- 最小改法：把澄清答案注册为 refer 候选（或直接把 `Target` 钉成答案），并给回问加轮次退避
  （第 2 次仍无进展 → 换策略或明确放弃并给出可执行建议）。

## 4. 边界（本套件不覆盖）

- 不覆盖音频/VAD/ASR 本身，只从文本开始。
- 不覆盖 iOS 壳的 Swift 渲染、SSE 事件流细节、CORS（`server` 包已有专项测试）。
- 不覆盖真实 LLM 路径（`Providers` 故意为 nil 保确定性）；LLM 参与后的分类需另测。
- 不覆盖性能基线（bench）与自愈层（selfheal）。

## 5. 与 M8 的对应

建议 M8-1 到 M8-6 与缺口一一对应，按风险排序：

1. **M8-1（安全）**：G1 否定识别 + G4 澄清优先级 —— 让"不要做"和"我没听懂"回到正确语义。
2. **M8-2（闭环）**：G8 澄清续跑收敛 —— 这是**手机上直接卡死**的一条，建议一并先修。
3. **M8-3（入口）**：G2 槽位完备门 + G3 元指令层 —— 拦住零信息句子，别让"改一下"进执行通道。
4. **M8-4（复杂）**：G6 条件标记 + G7 自我修正。
5. **M8-5（最大件）**：G5 子句切分与任务序列 —— 一句口述多动作，要么排成序列，要么明确回问"先做哪个"。

每修一条，把对应 `TestASRSimGap*` 从红变绿；全部转绿即 M8 语音闭环最小集的验收信号。
