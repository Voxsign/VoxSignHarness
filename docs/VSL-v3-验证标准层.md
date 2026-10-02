# VSL-v3 · 验证标准层

- 日期：2026-10-03
- 上游：`VSL-v1-描述定义语言规范.md`（什么是定义块）、`VSL-v2-判断语义层.md`（错了会怎样）
- 本层回答：**怎么判定它对、怎么判定它错、证据在哪、谁来判**
- 状态：v1 草案，待评审

---

## 0. 为什么需要这一层

VSL-v1 规定了"定义块怎么写"，VSL-v2 规定了"每个定义必须回答错在哪、代价是什么、机器该怎么反问我"。
但**判断语义层回答的是"错了会怎样"，不是"怎样算对"**。

缺了这一层，会出现一个逻辑上的死结：

```
可实施性 = 定义 ∧ 验证标准
若验证标准 = ∅  →  无法区分"做完了"与"没做完"
                →  不可实施（不是"难做"，是逻辑上不成立）
```

这正是"验证标准不清楚，就没办法在逻辑上讲明白是什么"的原因。
所以本层把 `VSL-共识-未来开发规范五原则:19`「**规范条目 = 声明 + 可执行断言；无验证器的条目 = 未完成**」
从一句口号，变成**可机器校验的字段约束**。

**一句话**：没有验证标准的定义，不是一个"待完善的定义"，而是一个**不可实施的定义**。

---

## 1. "验证标准不清楚"的三种形态（先能诊断，才能修）

| 形态 | 症状 | 为什么致命 | 修法 |
|---|---|---|---|
| **不可判定** | 只有形容词："稳定""高效""体验好" | 无法构造任何判据，只能靠人主观拍板 | 改成可判定的陈述（谓词 + 对象 + 条件） |
| **可判定但不可观测** | 判据依赖内部状态或被测方自述 | 外部无法证伪，等于信任被测方 | 判据锚定**外部产物**（落盘文件/日志/回执/事务记录） |
| **可观测但无阈值** | "要好""不能太慢" | 无法判"够不够"，事后可任意解释 | 给出**数字 + 容差 + 出处**；或显式标 `threshold: unset`（三态中的"未验证"） |

**规则**：任何定义块，若其验证标准命中以上任一形态且未显式标注，该定义**不得进入实现**。

---

## 2. 定义块强制字段（在 v2 六字段之后追加）

v2 已有：`purpose` / `priority` / `anti` / `cost` / `correct` / `ask`。
v3 追加七个字段，**缺一即"未完成"**：

```yaml
verify:
  claim: <可判定的陈述句。禁止形容词；必须是"在条件 C 下，系统做/不做 X"这种可被观测的形式>
  method: test | replay | measurement | review | audit
          # test=自动化断言；replay=冻结输出重放；measurement=数值测量；
          # review=人或强模型评审；audit=证据链核对
  evidence:
    level: E0 | E1 | E2 | E3     # E0 无来源禁止入结论；E1 线索；E2 多源交叉；E3 原始+异源复核
    kind: external_artifact | transaction_record | log | replay_bundle | expert_review
    ref: <文件/产物路径或 ID>     # 必须存在且可重新打开
  threshold: <数字 + 容差 + 出处；或 unset（则判定必为 unverified）>
  counterexample: <至少一条：这条定义错了，会观察到什么现象>   # 与 v2 的 anti 呼应但不同：
                                                                # anti 说"错因"，这里说"错的观测特征"
  verdict_states: [met, unverified, not_met]   # 三态，缺省即此；unverified 必须显式可表达
  approver: <谁判。禁止与实现者同一人/同一 agent>             # builder ≠ approver
  falsifier: <谁能证伪它、用什么反例>                        # 独立反方
```

### 2.1 字段级机器校验规则（可写成 lint）

| 规则 | 判据 |
|---|---|
| V3-01 | `claim` 非空，且**不含**形容词白名单外的模糊词（好/快/稳/优/强/满意…） |
| V3-02 | `method` ∈ 枚举 |
| V3-03 | `evidence.level` ∈ {E0,E1,E2,E3}，且 **E0 不得伴随 `verdict_states` 含 `met`** |
| V3-04 | `evidence.ref` 非空且指向真实存在的产物（可 `open` 验证） |
| V3-05 | `threshold` 为数字+单位，或恰为 `unset`；取值为 `unset` 时**禁止**判 `met` |
| V3-06 | `counterexample` 至少 1 条，且不是 `claim` 的简单取反（要写**可观测现象**） |
| V3-07 | `approver` 与实现者不同（同仓库内比对 owner 字段） |
| V3-08 | `falsifier` 非空 |

**不可机器校验的部分（必须人或强模型评审）**：`claim` 与真实意图是否一致、`counterexample` 是否真的是反例、
`threshold` 的取值是否有依据。这部分对应 `BOUNDARY-AUDIT:52`——**结构可校验，语义需评审**。

---

## 3. 一个填好的例子

**定义**（`docs/...`，大意）："低置信意图一律回问，不得直接执行。"

```yaml
purpose: 防止在模糊输入上猜错执行 —— 追问的代价远小于做错
priority: P0（安全红线，高于体验）
anti: 低置信时按"最近似规则"硬执行
cost: 多一次往返（用户多答一句）
correct: 置信 < 阈值 且 非 NOTE/ASK/REGISTER_TOOL 时，Ask 非空且不产生任何副作用
ask: "你是想让我做什么？请再说清楚一点"

# ---- v3 追加 ----
verify:
  claim: "对任意 utterance u，若 confidence(u) < c.ConfThreshold 且 intent(u) ∉ {NOTE,ASK,REGISTER_TOOL}，
          则 outcome.Ask ≠ '' 且该轮外部副作用数 = 0"
  method: test
  evidence:
    level: E2
    kind: external_artifact
    ref: "e2e/asrsim_test.go#TestASRSimFuzzyAcceptsIntentOrAsk"
  threshold: "副作用数 = 0（硬约束，容差 0）；覆盖率 ≥ 45/45 语料"
  counterexample: "输入『改一下』（无对象、无上下文）时系统产出 EDIT 且 Ask=''，
                   该轮进入执行通道 —— 观测特征：回执 result 出现 BOUNDARY_VIOLATION 或真实写盘"
  verdict_states: [met, unverified, not_met]
  approver: reviewer-agent（不得为实现者）
  falsifier: "任何一条 confidence<阈值 且 Ask='' 且产生副作用的用例，即可证伪"
```

**注意这个例子暴露的真实状态**：按上面的 `counterexample`，
当前 `input/taskintent_orchestrate` 相关实现**并未满足**——`改一下` 判 EDIT 0.85 且 `Ask=""`。
所以这条定义的判定是 `not_met`，而不是"看起来没问题"。**这正是加这一层的目的：让"没做到"变得无法被含糊过去。**

---

## 4. 与三态门禁、证据等级的关系

```
verdict_states:  met        ← 判据满足且证据 ≥ E2 且 approver 已签
                 not_met    ← 判据被反例证伪
                 unverified ← ①threshold=unset ②evidence.level=E0/E1
                              ③样本不足（观察不足 ≠ 通过，见 CICD-BOUNDARY-001:389）
```

**两条硬规则**（吸收自 `CICD-BOUNDARY-001`）：

1. **`unverified` 不得折算为通过，也不得折算为 0 分**——必须原样显性存在（`:224` 的 `failed/cancelled/timeout/unknown` 均不视为通过的推广）。
2. **禁止本次变更自行降低门槛**（`:107`）：验证标准属于"变更前可信策略"，
   改标准 = 升版 + 保留旧标准哈希与旧判定，**禁止回溯修改历史判定**（`fr-25` 的反博弈要求）。

---

## 5. 验证标准的验证标准（元层）

谁来判"这个验证标准本身合格"？

| 检查 | 谁做 | 判据 |
|---|---|---|
| 字段完整性与枚举合法性 | 机器（lint） | §2.1 的 V3-01…V3-08 全过 |
| `claim` 是否真的对应意图 | **异源**评审（不同模型/不同人） | 独立作答，不锚定原作者的表述（`auto-002` P3 的遮蔽式复核） |
| `counterexample` 是否真能证伪 | 独立反方（红队） | 反例可复现地触发 `not_met` |
| 阈值是否有依据 | 人或强模型 + 出处 | `threshold` 带来源；无来源 → `unverified` |
| 判分器是否独立于被测实现 | 结构检查 | `approver ≠ builder`；判分器与实现不同文件、不同作者 |

**元层红线**：`CICD-BOUNDARY-001:230`「防止同一智能体同时改实现、预期答案和放行标准」。
→ 所以本层的 lint 规则、金标准集、判分器**三者必须是只读资产**，改动需升版并经人批准。

---

## 6. 落地位置（待定，需讨论）

- 本规范：`docs/VSL-v3-验证标准层.md`（本文件）
- lint 实现：建议 `doccontract/`（已有 L0 契约一致性的基础）扩展为 `verifylint`
- 与 `长城任务-预注册与台账规范-v0.md` 的关系：那是**任务级**的验证标准（PREREG），本层是**定义级**的验证标准；两者共用 E0–E3 与三态语义，**不要各写一套**

---

## 7. 待评审的三个问题

1. 字段是否过多？`falsifier` 能否并入 `approver`？（我倾向保留：判对的人和证伪的人应当分离）
2. `threshold: unset` 是否允许存在？我主张**允许**——它诚实表达了"未验证"，比编一个数字好。
3. 本层是否要求**存量定义块回填**？我主张不回填，只对**新增与修改**生效（避免一次性大工程），
   但要在 `SPEC-v2` 的缺口表里登记"历史未回填项数"，让它显性存在。
