# 真值库签署请求 2026-10-05

- 对象：`eval/cases/cases.jsonl`（17 条种子用例）
- 性质：**签署请求，待 Peter（或指定签署人）执行**。按 `eval/cases/README.md` 治理规则，
  真值库由人增删改并签署 `ratified_by`；agent 只提议、不代签。
- **本文件不改 `cases.jsonl`、不伪造任何签署**（当前 17 行 `ratified_by` 仍全为 `null`）。

---

## 一、当前现状（逐条索引，2026-10-05 实测）

状态分布：**8 met / 8 not_met / 1 unverified，ratified_by 全部为 null（无人签署）**。

| # | id | status | 内容摘要（expect_behavior 缩写） | verify_cmd 标签 |
|---|---|---|---|---|
| 1 | VHS-ASR-0001 | met | 空串/纯空白/仅换行 → Ask 非空且 Allowed=false | e2e asrsim |
| 2 | VHS-ASR-0002 | met | 非空输入 done 时 receipt 四行标签齐全且非空 | e2e asrsim |
| 3 | VHS-ASR-0003 | met | COMMIT/DEPLOY 未人工放行 → Confirmed=false | e2e asrsim |
| 4 | VHS-ASR-0004 | met | 无域归属写操作 → NeedsClarification 或拒绝 | e2e asrsim |
| 5 | VHS-ASR-0005 | met | fz-01…fz-11 模糊句 → 判对或回问 | e2e asrsim |
| 6 | VHS-ASR-0006 | met | 模糊句落到唯一终态、无死循环 | server asrsim |
| 7 | VHS-ASR-0007 | met | answer 续跑 200，task_id 与首次一致 | server asrsim |
| 8 | VHS-ASR-0008 | met | 三条并发任务 task_id 唯一、各自 done | server asrsim |
| 9 | VHS-ASR-0009 | **unverified** | Providers 非 nil 的真实 LLM 路径未评测（显式保留） | 无自动化 |
| 10 | VHS-ASR-0010 | not_met | G1 否定句不得判可执行删除 | asrsimgap |
| 11 | VHS-ASR-0011 | not_met | G2 缺必需槽位 → Ask 非空，不直接执行 | asrsimgap |
| 12 | VHS-ASR-0012 | not_met | G3 元指令不得判为可执行任务 | asrsimgap |
| 13 | VHS-ASR-0013 | not_met | G4 UNKNOWN 的澄清原因不得被 refer 覆写 | asrsimgap |
| 14 | VHS-ASR-0014 | not_met | G5 多动作不得无声丢弃 | asrsimgap |
| 15 | VHS-ASR-0015 | not_met | G6 条件句不得无条件执行 | asrsimgap |
| 16 | VHS-ASR-0016 | not_met | G7 自我修正标记，修正前内容不入槽位 | asrsimgap |
| 17 | VHS-ASR-0017 | not_met | G8 澄清续跑不得逐字重复同一问题 | asrsimgap |

> 注：行 #10–#17 当前声明 `not_met`，但漂移探测器（见 §三）显示其 `verify_cmd` 已转绿——
> 即缺口已修、状态待人工重签为 `met`。这正是本次要 Peter 拍板的核心。

---

## 二、需要 Peter（或指定签署人）执行的步骤

1. **审阅**：先读 `RATIFICATION-PROPOSAL-2026-10-03.md`（G1–G8 修复提交与复算证据），
   再逐条核对上表 #10–#17 的 `expect_behavior` / `forbidden_behavior`。
2. **复算（建议）**：跑漂移探测器，确认声明与实测一致：
   ```bash
   go test -tags evaldrift ./eval/cases -run TestTruthSourceDrift -v
   ```
3. **拍板 G1 口径**（proposal §二.1 的遗留问题）：
   `不要删除那个文件` 修复后是「ASK 回问（确认不做吗）」。
   - 若认可"回问即安全"→ 签 VHS-ASR-0010 为 `met`；
   - 若认为"否定就该静默不执行、连问都不该问"→ 维持 `not_met`，并在签署意见注明。
4. **补录签署**：对每条你确认的行，在 `cases.jsonl` 编辑该行：
   - `status`：`not_met` → `met`（你认可修复的 #10–#17）；已 `met` 的 #1–#8 补签即可、不改状态。
   - `ratified_by`：填签署人姓名（如 `"peter"`）。
   - `ratified_at`：填签署日期（如 `"2026-10-05"`）。
   - **#9（unverified）保持 `unverified`，不要折叠成 met**（proposal §二.3：LLM 路径本轮未覆盖）。
5. **约定复算周期**：proposal §二.2 指出 rebase 后状态会漂移，签署时一并约定"多久重跑一次漂移探测器"。

---

## 三、签署验收标准

- [ ] #1–#8（已 met）每行 `ratified_by` / `ratified_at` 已补录（人签）。
- [ ] #10–#17 按 Peter 拍板口径，状态与签署一致（预期全部转 `met`，G1 例外另注）。
- [ ] #9 维持 `unverified`，未被折叠。
- [ ] `go test -tags evaldrift ./eval/cases -run TestTruthSourceDrift -v` 复算一致（无新漂移）。
- [ ] 签署后 `cases.jsonl` 的改动**仅**为 `status`/`ratified_by`/`ratified_at` 三字段，未篡改 expect/evidence。

---

## 四、与 RATIFICATION-PROPOSAL-2026-10-03.md 的关系

- 03 号提案提出了「8 条 not_met → met」的改状态建议 + 漂移探测器，但**无人签署**（ratified_by 全 null）。
- 本文件（10-05）是**把该提案落到可执行的签署清单**：逐条现状、签署步骤、验收标准。
- 两者不冲突：03 号是"为什么改"，10-05 是"谁来签、怎么签、签完验收什么"。
- agent 在本批 P0 修复中**未触碰** `cases.jsonl`（见 `git status`），保持真值库只读待签。
