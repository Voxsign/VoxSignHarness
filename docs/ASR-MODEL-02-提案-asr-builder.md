# ASR-MODEL-02 · 三条模型调用通道 —— 实现方提案（asr-builder）

> 输入：`tasks/ASR-MODEL-02-三条调用通道.md`、`tasks/ASR-MODEL-01-模型固定约束.md`（集成线 `62702b0`）。
> 性质：**提案**。DSH 已于 2026-10-03 审阅采纳（三分法 + LEARN-01..08）。
> 落地进度：**LEARN-01 已落地并转绿**（commit `c6246f3`）；`Engine.Observe` 降级已落地（`465e47e`）；
> **LEARN-02 之后的模型相关部分仍未落地**——底层模型与模型中心协议待 Peter 提供。
> 与手上工作的关系：第 1 批（轨迹/词典/热加载，commit `613d3c4`）已绿，**本提案不改动它**。

---

## 0. 先划边界（避免又一次"混两件事"）

规范里 L2 说"**只有 `learn` 可以写回持久知识（词典、偏好、模式、知识库）**"。
但当前已绿的 `SCOPE-DICT-01` 走 `/v1/dictionary`，用户说「记住，冀总是冀中的冀」——
这**是用户自己写的，不是系统猜的**。若把 L2 理解成"任何词典写入都必须过 learn"，
那用户显式指令会被强制变成异步学习，**UX 反而变坏**，而且把"用户作者的写入"和
"系统推断的写入"混成了一类——正是本项目已经犯过 5 次的那类错误。

**我的提案：按"作者是谁"分三种写回来源，只有第二种受 L2 约束。**

| 写回来源 | 例子 | 是否允许同步 | 是否过 `learn` | 审计要求 |
|---|---|---|---|---|
| `user_explicit` | 用户说「记住…」；用户手动增删改 | ✅ 允许（用户就在等） | ❌ 不需要（作者是用户） | 全量留痕：谁、何时、原文 |
| `learn_inferred` | 系统从使用记录推断出的规则 | ❌ **必须异步** | ✅ **唯一通路** | 证据链 + 版本 + 可回滚（L3/L4） |
| 其它任何路径 | 在线通道顺手写 | 🚫 禁止 | 🚫 | —— |

> **裁决（2026-10-03，DSH 采纳）**：上表三分法**通过**；本节最初标注的"待裁决"已结清。
> 现行 L2 表述见 §0.1 的 **v2**；DSH 明确 `SCOPE-DICT-01` 由其本人按此同步改，**我不动判据**。

另外：现有 `Engine.Observe` 目前会把反馈学成**候选词条**（不落盘）。
按 L2 我建议**降级为"只记证据、不写知识"**，把隐式学习的写回全部收口到 `learn`。
**已裁决同意并落地**（见 §0.2）。

---

### 0.1 L2 表述升版：v1 → v2（旧版保留）

**这是本项目"把两件语义不同的事塞进同一个容器"这一毛病的第 6 次**，
而且这次是我用 DSH 自己写的规则抓到了 DSH 自己的规格。

**L2 v1（旧表述，作废，原样保留在此以示决策史）**：

> L2 · **只有 `learn` 通道可以写回持久知识**（词典、偏好、模式、知识库）。
> 违反时：在线通道写回 = 不可审计的漂移。

**旧表述错在哪**：它把两种**作者不同**的写入混成了一类——

| 写入 | 作者 | 性质 | 该不该过 learn |
|---|---|---|---|
| 用户说「记住，冀总是冀中的冀」/ 手动增删改 | **用户** | 用户**下达指令** | ❌ 不该（用户就在等，且他才是作者） |
| 系统从使用记录推断出"这类错该这么纠" | **系统** | 系统**自己推断** | ✅ 必须（否则不可审计地漂移） |

v1 把"用户作者的写入"也判成"写回"，后果有两个：
① 用户体验变差（显式指令被强制异步）；
② **归因失效**——"系统学到的东西"与"用户直接告诉系统的东西"再也分不开，
   而这正是 `rubric-v0` / `asr-split-punct` / `C1 v1` / `C1 v2` / `模型不固定` 的同一个病根。
按 `fr-25` 反博弈：**升版 + 保留 v1 原文，不回溯修改**。

**L2 v2（现行，已采纳）**：

> L2 v2 · 持久知识的写回按**作者**分三种来源：
> 1. `user_explicit`（用户主动说 / 手动增删改）——**不过 `learn`**，可同步；
> 2. `learn_inferred`（系统从使用记录推断）——**唯一过 `learn`**，必须异步、带证据、可回滚；
> 3. 其它任何路径——**禁止**。
> 机器形态：`writeToken` 未导出，只由 `learn` 通道的构造路径签发（见 §1.2）。

### 0.2 `Engine.Observe` 降级（已落地，commit `465e47e`）

`Observe` 不再写回任何知识，只追加 `Evidence`（`At/Raw/Corrected/Accepted/Source`，有上限），
供 `learn` 通道消费；热词权重仅作 `Lexicon` 审计。原来调用 `upsertLearned/removeLearned`
的写回路径已删除，两个 helper 保留给 learn 写回路径并注明"当前无调用点是刻意"。

---

## 1. 三条通道的配置形态（具名字段）

**一个 JSON 文件（明令不用 YAML），启动时校验、失败即拒绝启动（fail closed）。**

```jsonc
// config/model-center.json   （路径由 VHS_MODEL_CONFIG 指定；ASR 服务与其它线共用）
{
  "contract_version": "1",
  "model_center": {
    "base_url": "http://127.0.0.1:8899",      // 模型中心地址（外置，能力外置原则）
    "api_key_env": "VHS_MODEL_KEY"            // 密钥只从环境变量取，不落配置文件
  },
  "channels": {
    "default": {
      "enabled": true,
      "provider": "deepseek",
      "model_id": "deepseek-v4.1-flash",      // ← ASR-MODEL-01 固定值
      "timeout_ms": 3000,                     // 需求 Δ7：模型兜底 3s
      "max_concurrency": 4,
      "write_back": false                     // L2：默认调用永不写回
    },
    "diagnose": {
      "enabled": false,                       // ← Peter 未指定模型前必须 false
      "provider": "deepseek",
      "model_id": "TBD",                      // ← 待 Peter 指定
      "timeout_ms": 5000,
      "max_concurrency": 2,
      "write_back": false
    },
    "learn": {
      "enabled": false,                       // ← Peter 未指定模型前必须 false
      "provider": "deepseek",
      "model_id": "TBD",                      // ← 待 Peter 指定
      "timeout_ms": 60000,                    // 慢可以：事后离线
      "max_concurrency": 1,                   // 单一写者，避免知识竞争
      "write_back": true,                     // L2：唯一允许写回
      "queue":   { "path": "${DATA}/learn-queue.jsonl",
                   "coalesce_window_ms": 2000, "max_batch": 8 },
      "staging": { "path": "${DATA}/knowledge-staging.jsonl" },
      "rollback":{ "ops_path": "${DATA}/knowledge-ops.jsonl", "keep_versions": 200 }
    }
  }
}
```

### 1.1 启动校验（机械规则，对应 L1/L2/L5）

| # | 规则 | 违反时 |
|---|---|---|
| C1 | `channels` 的键**恰好**是 `{default, diagnose, learn}`，多一个少一个都拒 | 启动失败（L1） |
| C2 | `default.write_back == false`、`diagnose.write_back == false`、`learn.write_back == true` | 启动失败（L2） |
| C3 | `enabled == true` ⇒ `model_id` 非空且 `!= "TBD"`；否则必须 `enabled == false` | 启动失败（L5：无 model_id 的调用不可归因） |
| C4 | 配置文件后缀必须是 `.json`；检测到同名 `.yaml/.yml` 引用即拒 | 启动失败（明令不用 YAML） |
| C5 | 本进程内**只允许一份** `learn` 写者（`max_concurrency == 1`） | 启动失败 |

### 1.2 类型层面的写回能力（L2 不靠约定）

```go
type Channel string
const (
    ChannelDefault  Channel = "default"
    ChannelDiagnose Channel = "diagnose"
    ChannelLearn    Channel = "learn"
)

// 每个通道一个客户端；Registry 在构造时就校验 C1–C5。
type ModelClient interface {
    Invoke(ctx context.Context, req ModelRequest) (ModelResponse, error)
}
type ModelResponse struct {
    Text    string
    ModelID string   // L5：每次调用都必须带回来并落轨迹
    Channel Channel
}

// 写回令牌：只由 learn 通道的构造路径签发；其它通道拿不到（编译期隔离）。
type writeToken struct{ channel Channel }          // 不导出

// KnowledgeWriter 是唯一能改持久知识的入口；无令牌调用不可编译/直接拒绝。
type KnowledgeWriter interface {
    Stage(patch KnowledgePatch, tok writeToken) error
    Apply(patch KnowledgePatch, tok writeToken) error
    Rollback(opID string, tok writeToken) error
}
```
- `NewRegistry(cfg)`：按 C1–C5 校验；若 `learn.enabled == false`，**不构造** `KnowledgeWriter`。
- 于是"default 在线通道顺手写回"在**类型上就写不出来**，不是靠 code review 拦。

---

## 2. `learn` 的触发形态

### 2.1 谁触发：**系统自己**（这就是 `LEARN-01` 要测的东西）

```
请求路径（在线，快）                 │   学习路径（事后，异步，慢）
────────────────────────────────────┼──────────────────────────────────
/v1/process、/v1/correct            │   learn worker（单协程）
  ├ 本地规则 + default 通道         │     ├ 从队列取候选
  └ 追加一条 usage-event（append）  │     ├ 证据校验（L3）
                                    │     ├ 调 learn 通道 → 结构化 patch
  立即返回，不等 learn（L6）         │     ├ staging → 乐观并发校验 → apply
                                    │     └ 写 ops 日志（可回滚）
```

- **触发器是本地规则**（零模型、零延迟）：`LearnTriggerDetector.Detect(events) []LearnCandidate`。
  它自己判断"这里值得学"，不等人下命令 —— 直接对应规范 §4 的"有学习意识"。
- 触发来源（事件类型，全部来自 append-only 的 `usage-events.jsonl`）：

| 事件 | 何时产生 | 是否默认触发 |
|---|---|---|
| E1 `user_correction` | 用户改字/确认（`Feedback{Accepted:true}` 且 raw≠final） | ✅（前提：本地规则/词典**未覆盖**） |
| E2 `dict_miss` | 同一 raw 被用户改两次以上 | ✅ |
| E3 `diagnose_cause` | diagnose 通道给出"可学习"的 cause 类 | ✅（需人工可见） |
| E4 `rule_override` | 同一规则被用户连续否决 ≥N 次 | ✅ |
| E5 `learn_derived` | 由 learn 自己产出的知识再次触发 | 🚫 **禁止**（防自训练回路） |
| E6 已覆盖/纯噪声/未确认 | —— | 🚫 不触发（防"什么都学"） |

- 事件先落盘再处理：**崩溃不丢候选**（append-only，重启续跑）。

### 2.2 单位：**默认一次学一条；同类可小批合并（有界）**

- 一个 `LearnCandidate` = 一次 learn 调用（默认）。
- 同一 `knowledge_key` 在 `coalesce_window_ms`（默认 2s）内的重复事件 → **合并成一批**，
  上限 `max_batch`（默认 8）；超出则溢出为下一次调用。
- 一批产出的 patch 必须**逐条**带各自 evidence（不允许"整批共用一个来源"）。

### 2.3 离线还是在线：**在线但异步 + 支持离线批跑**

- 默认：在线事件异步入队，worker 后台消费；**关键路径完全不等它**（L6）。
- 另提供离线入口（可选）：`vhs-asr learn drain --from usage-events.jsonl`，
  用于"事后重放一段真实使用记录"——`LEARN-01` 的测试就用这个入口（不需要模型）。

---

## 3. 回滚怎么做

### 3.1 数据模型：每个知识变更都是**一条可逆 op**

```go
type EvidenceRef struct {
    EventID     string  // 哪次输入（usage-events 主键）
    ConfirmedBy string  // 谁确认的：user | reviewer | auto
    Outcome     string  // 结果如何：accepted | rejected | corrected
    TraceRef    string  // 关联的轨迹 request_id
    At          string
}

type KnowledgePatch struct {
    OpID        string        // 唯一 id（时间序 + 随机后缀）
    Channel     Channel       // 恒为 learn
    ModelID     string        // L5
    BaseVersion int           // 乐观并发：apply 时版本不符则拒
    Target      string        // 白名单内的知识键（如 dict:/prefs:/pattern:）
    Before      string        // 写前状态（用于逆操作）
    After       string        // 写后状态
    Reason      string        // learn 给出的理由
    Evidence    []EvidenceRef // L3：空 → 拒绝，不落盘
    DerivedFrom string        // E5 防回路：来源已是 learn_derived 则拒绝
    CreatedAt   string
}
```

- 知识文件本身仍是普通 JSON/JSONL（需求 §5），但**每次变更都另写一条 op** 到
  `knowledge-ops.jsonl`（append-only）。这与第 1 批词典已有的 `.ops.jsonl` 同构。
- `apply` 的前提：`BaseVersion == 当前版本` 且 `Before == 当前值`；否则**拒绝并记一条 rejected op**
  （防止并发/漂移下盲写）。

### 3.2 回滚的三种粒度

| 粒度 | 操作 | 语义 |
|---|---|---|
| **单条** | `vhs-asr knowledge rollback --op <op_id>` | 写入**逆操作**（新 op，`rollback_of=<op_id>`），恢复到 `Before` |
| **批次** | `--batch <batch_id>` | 按 op 顺序逆序回滚该批全部 op |
| **熔断** | `VHS_LEARN_ENABLED=false` 或配置 `learn.enabled=false` | 立即停止新写入（在途批次落 staging 不 apply） |

- **永不删历史**：回滚产生新版本，旧 op 保留 —— 可审计、可再回滚。
- **权重衰减**（需求 4.8：否认超阈值→降权/移除）：知识条目带 `weight`；
  被用户否决 → weight--，低于阈值 → **挂起**（suspend，不删除）+ 记 op；
  再确认 → 恢复。避免"一次误学永久生效"。
- 校验回滚真的生效：回滚后**重放原触发输入**，断言行为回到写回之前（见 LEARN-05）。

---

## 4. `LEARN-01` 怎么让它可判（先红）

### 4.1 输入：单一权威语料

新建 `asr/corpus/usage-events.jsonl`（**与 real-dialogue.jsonl 同构，单一权威源**），每条：

```jsonc
{"id":"ev-001","at":"...","kind":"user_correction",
 "raw":"把deept接上","final":"把DeepSeek接上","confirmed":true,
 "outcome":"accepted","trace_ref":"req-12",
 "learn_label":"trigger",              // trigger | no_trigger
 "provenance":"constructed"}           // observed | constructed（如实标注）
```

`learn_label` 是**期望**，不是提示（Detector 接口不接收任何 label —— 见 R4）。

### 4.2 判据（机械、**不需要模型**）

`LEARN-01`：`go test -tags vhs002 ./asr -run TestLEARN01`，断言：

| 断言 | 内容 | 反例（防什么） |
|---|---|---|
| **R1 召回** | `learn_label=="trigger"` 的事件 → `Detect` 至少产出 1 条 candidate，且 `candidate.Evidence[].EventID` 精确指向该事件 | 一条都没发现 = 没学习意识 |
| **R2 精确** | `learn_label=="no_trigger"` 的事件 → **0** candidate | "什么都学" = 噪声固化 |
| **R3 有界** | 同一 `knowledge_key` 的重复事件 → candidate 数 ≤ `max_batch` 合并上限 | 刷屏式学习调用 |
| **R4 自主** | Detector 签名不含任何"该学谁"入参；测试只喂事件流 | 人告诉它学 = 不算自主 |
| **R5 只读** | `Detect` 前后数据目录文件哈希不变 | 检测阶段就写回 = 绕过 staging |

> **先红现状（2026-10-03 更新：已转绿）**：判据先红时"连 Detector 类型都没有，五条全红"；
> 现已落地（commit `c6246f3`）：`go test -tags vhs002 ./asr -run TestLEARN01` 绿，
> 使用记录 14 条（trigger 6）→ 召回 6/6，去重后 4 条候选；语料全为 **constructed**，
> 真实样本仍需 Peter 签字。
> **不需要模型即可实现**：所以即使 Peter 还没定 `learn` 的底层模型，
> `LEARN-01`（本地检测）也可以先做、先绿；模型相关的 LEARN-02 保持红。

### 4.3 配套判据（覆盖 L2–L6 + "写回≠学到"）

| 判据 | 断言 | 对应 |
|---|---|---|
| **LEARN-02** 写完要真命中 | 用**假模型客户端**跑全链：事件→候选→learn(假 patch)→staging→apply→**重放同类输入**，断言纠错来源是 `source=learn:<op_id>`（轨迹留痕）**且结果正确** | 规范 §3"写回≠学到" |
| **LEARN-03** 在线通道写不进去 | default/diagnose 拿不到 `writeToken`；知识文件哈希不变 | L2 |
| **LEARN-04** 无来源不落盘 | evidence 为空的 patch → 拒绝，`knowledge-ops.jsonl` 无 apply 记录 | L3 |
| **LEARN-05** 可回滚 | apply→rollback→重放触发输入，行为回到写前；ops 日志可读；熔断后不再写 | L4 |
| **LEARN-06** 模型可归因 | 三条通道响应都带 `model_id`；`default` 必须 == `deepseek-v4.1-flash`；缺 = 红 | L5 / ASR-MODEL-01 |
| **LEARN-07** 不阻塞主链 | 假 learn 客户端 sleep 5s 时，`/v1/correct` 仍 <1ms 返回，且该请求不直接调 learn | L6 |
| **LEARN-08** 防自训练回路 | `DerivedFrom` 为 learn 产物的 patch → 拒绝 | 风险 |

---

## 5. 需要 Peter / DSH 拍板的问题（阻塞落地）

1. **`diagnose` 与 `learn` 各用哪个底层模型？** 未定前两条通道 `enabled:false`（C3 保证不会瞎调）。
2. **§0 的写回三分法**（`user_explicit` 不经 learn）是否认可？
   若不认可，`SCOPE-DICT-01` 需按你们发起的判据变更一起改。
3. **`learn` 的写回目标白名单**：词典 / 偏好 / 模式 / 知识库里，
   哪些允许自动写、哪些必须人工确认？（我按需求 4.8 暂列：词典与模式可自动、偏好与权限必须确认。）
4. **`usage-events.jsonl` 的权威性**：我可以先放**constructed** 事件让判据先红；
   但"真实使用记录"需要 Peter 真机确认过的样本签字，否则 `LEARN-01` 的说服力有限（技能文档 §9）。
5. **模型中心协议**：请求/响应格式、`model_id` 字段名、错误码——需要一份接口说明才能对接。

---

## 6. 诚实边界与风险

- **自动写回是唯一能让系统自己变坏的机制**（规范原话）。因此我的设计把"变坏"的路径全部收口：
  唯一入口（L2 类型令牌）、证据必需（L3）、版本+op 日志+逆操作+熔断（L4）、慢通道异步（L6）、
  乐观并发防盲写、防自训练回路。
- **本提案零实现**：没有新增任何 Go 代码，第 1 批的绿不受影响。
- `LEARN-01` **可先于模型落地**（本地 Detector），但 `LEARN-02` 之后的"真学到"必须等模型与协议确定。
- 我**无法核实** `deepseek-v4.1-flash` 是模型中心的准确标识（与 ASR-MODEL-01 §6 同）；
  `diagnose`/`learn` 的模型标识我完全不知道，按规范只能留 `TBD` 并禁用。
- 事件语料若只有 constructed 样本，**不许把"全绿"说成"有学习能力"**。

---

*ASR-MODEL-02 提案 · asr-builder · 2026-10-03。等 Lead/Peter 裁决后再落地；在此之前不写实现。*
