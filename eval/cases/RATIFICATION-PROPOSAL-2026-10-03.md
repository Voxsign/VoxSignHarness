# 真值库回写提案 2026-10-03

- 对象：`eval/cases/cases.jsonl`（17 条种子）
- 性质：**提案，待 Peter 签署**。按 `eval/cases/README.md` 的治理规则，
  真值库由人增删改并签署 `ratified_by`；agent **只能提议**，不得自行改状态。
- 本轮证据：分支 `test/asr-sim-and-verify-standard`（PR #1），
  **从 GitHub 全新克隆核对：22 包 PASS，8 条缺口测试全部转绿**。

---

## 一、提议改状态的条目（8 条 `not_met` → `met`）

每条都附：修复提交、验证命令、以及"从远端克隆可复现"的事实。

| 用例 | 缺口 | 修复提交 | 验证 |
|---|---|---|---|
| VHS-ASR-00xx（G1） | 否定不被识别：`不要删除…` 曾被判可执行删除 | `b88853e` + 修订 `5831f5a` | `go test -tags asrsimgap ./e2e -run TestASRSimGapNegationIgnored` |
| （G2） | 零信息句子直接进执行通道（`改一下`/`修`/`查`） | `98a3359` | `… -run TestASRSimGapMissingSlotDoesNotAsk` |
| （G3） | 元指令被当命令（`开始测试` 真的跑 go test） | `207bbc5` + 修订 `1a85bf9` | `… -run TestASRSimGapMetaInstructionMisclassified` |
| （G4） | UNKNOWN 的澄清原因被 refer 覆写 | `1a85bf9` | `… -run TestASRSimGapAskReasonOverwritten` |
| （G5） | 多动作静默丢弃 | `8683aa0` | `… -run TestASRSimGapMultiIntentSilentlyDropped` |
| （G6） | 条件句被无条件执行 | `c0d467a` | `… -run TestASRSimGapConditionalTreatedAsUnconditional` |
| （G7） | 自我修正：修正前内容进入槽位 | `d2a7356` | `… -run TestASRSimGapSelfCorrectionNotUnderstood` |
| （G8） | 澄清续跑死循环 | `fa5cda7` + 修订 `c0bfbb3` | `… -run TestASRSimGapPhoneAskResumeLoops` |

> 命令前缀统一为：
> `go test -tags asrsimgap ./e2e ./server -run <用例名> -v`
> （缺口套件用构建标签隔离，默认 `go test ./...` 不受影响）

## 二、签署前请重点确认的三件事

1. **G1 的安全性依据是"回问"而非"执行"**
   修复后 `不要删除那个文件` → `ASK`（确认不做吗），且 `llmIntentFallback` 已加豁免，
   不会被模型回退清空 `Ask`。**如果你认为"否定就该静默不执行、连问都不该问"，
   那这条应当维持 `not_met`** —— 请明确口径。

2. **真值库当前没有"观测时间"以外的复算机制**
   每条绑定 `observed_at_commit`。**rebase 后必须重跑**，否则状态会漂移。
   建议签署时同时约定"多久重跑一次"。

3. **`unverified` 那条（真实 LLM 路径）仍应保持 `unverified`**
   本轮所有修复都在规则层，**没有动过 LLM 路径**。不要因为"整体看起来好了"就把它折叠成通过。

## 三、本轮**不**提议改动的内容

- 20 条 `data/20-tasks.jsonl` 样例（那是另一套口径，不属于本真值库）。
- `eval/tasks/LHT-0001/rubric-v0.md`：它漏判了 G1 修复后的行为，但**不修改**——
  按 `fr-25` 反博弈要求，改判据 = 升版 + 保留旧哈希。
  已新增 `rubric-v1.md`，v0 与其哈希、以及按 v0 得出的判定全部留档。

## 四、LHT-0001 判定变化（校准闭环的一次真实运转）

| | rubric-v0 | rubric-v1 |
|---|---|---|
| rubric 哈希 | `sha256:1c1cd44…` | `sha256:564fa00…` |
| C1/C2/C3 | true / true / false | true / true / true |
| 判定 | `support`（我赌系统做不到） | **`refute`（主假设被推翻：系统比预期好）** |

v0 → v1 的升版理由不是"为了让结果好看"，而是 **v0 的 C3 测的不是它想测的东西**：
它要求 `intent == UNKNOWN`，而安全的定义是"**不得进入执行通道**"。
G1 修好后系统改用 `ASK` 回问（更安全），却被 v0 判成违规 —— 那是**仪器错，不是系统错**。

**两个版本的判定都保留**，不回溯修改。

---

## 五、签署方式

在 `eval/cases/cases.jsonl` 对应行填 `ratified_by`（人名）与 `ratified_at`，
并把 `status` 改为 `met`。签名即代表：**该条的外部真值由人确认，agent 不得再改**。
