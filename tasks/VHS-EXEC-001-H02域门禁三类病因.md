# VHS-EXEC-001 · H-02 执行链：域门禁「一切 BOUNDARY_VIOLATION」的三类病因

> **状态**：**根因已坐实（含 8 次真跑）· 修法待 Peter 定方向 · 未改任何代码**
> **依据**：Peter `docs/校准报告-产品与实现-L01.md` §5.1 P0「H-02 执行」+ §2「H-02 ✗」
> **落盘日期**：2026-10-03 · 证据全部可复现（见 §4）

---

## 1. 现象（真跑，8 个输入）

| # | 输入 | 动作 | 结果 |
|---|---|---|---|
| 1 | 搭建一个新的计费服务，产出可编译的代码 | （无） | **停在意图层**：`stage=discuss`「待澄清」 |
| 2 | 跑测试 | **TEST** | `space_check 拒绝（boundary_violation）` |
| 3 | 改代码 | **EDIT** | `space_check 拒绝（boundary_violation）` |
| 4 | 记一下想法：明天开会 | **NOTE** | **✅ 通过并真执行**（`appendd: ~/.voicesign/harness/notes.md`） |
| 5 | 组织 VoxSign 项目的发布流程 | **DEPLOY** | `space_check 拒绝（boundary_violation）` |
| 6 | 编排 VoxSign 的发布步骤 | **DEPLOY** | `space_check 拒绝（boundary_violation）` |
| 7 | 跑 VoxSign 的测试 | **TEST** | `space_check 拒绝（boundary_violation）` |
| 8 | 提交 VoxSign 的改动 | **COMMIT** | `space_check 拒绝（boundary_violation）` |

**⇒ 6 个走到门禁的输入里 5 个被拒；唯一通过的是 NOTE（有域特例）。**
**⇒ §5.1 的「一切 BOUNDARY_VIOLATION」准确。**（我曾两次怀疑它，两次被真跑推翻。）

---

## 2. 三类病因（**修法不同，风险不同**）

### ① 域落错（可一行修）—— TEST / COMMIT / EDIT

```go
pipeline/pipeline.go  defaultSpaceFor(it):
    Note → "vault-notes" · Query/Ask → "global" · Orchestrate → "project"
    **default → "global"**          ← TEST / EDIT / DEBUG / COMMIT / DEPLOY 全落这里

space/space.go:144  builtinTemplates()["global"]:
    Tools: ["read","query","ask"] · Perms{Read:true}
    Acceptance: "**只读兜底，无写权限**"

planCaps:  EDIT → ["file","read","run"] · TEST → ["test","run","read"] · COMMIT → ["git","read"]
space.Check 第 ④ 道：ToolCaps ⊄ m.Tools ⇒ boundary_violation
⇒ EDIT 的 [file,read,run] ⊄ global 的 [read,query,ask] ⇒ **拒**
```
**⇒ `project.Tools = ["file","git","search","test","run","read"]` 已含所需**
⇒ 修法：给写/执行类意图指定 `project`（或 `sandbox`）
⚠️ **但它改的是"未指定域时落到哪"的默认 ⇒ 属授权语义 ⇒ 待 Peter 定**

### ② 无域可容（需授权决定）—— DEPLOY

```go
planCaps:  case IntentDeploy: return []string{"deploy","http","read"}

五个域的工具集：
  global: read,query,ask · project: file,git,search,test,run,read
  sandbox: file,run,search · vault-notes: note,file-append,read · vault-creds: read
⇒ **没有任何域含 `deploy` 或 `http`**
⇒ DEPLOY **不论落哪个域都会被拒** —— 不是"落错域"，是"**没有域能容纳它**"
```
⇒ 修法：**新域声明**（如 `deploy` 域）或**给 project 扩权**
⚠️ **属"放开外发"方向 ⇒ 风险最高 ⇒ 必须 Peter 定**

### ③ 意图识别不出来（属 H-03）—— ORCHESTRATE

```
4 个输入（"组织…发布流程"/"编排…发布步骤"）**全被分到 DEPLOY**，**没触发 ORCHESTRATE**
⇒ 与 §5.1 的 H-03「意图 8+1 无长程类 → UNKNOWN」呼应
⇒ 下一步（读代码即可，不改）：核 `planner` / `input` 里 ORCHESTRATE 的触发词表
```

---

## 3. 修法顺序建议（**待 Peter 确认**）

```
① **先修"域落错"**（一类）：改动小 + 语义自洽 + 判据清晰
   ⇒ 判据：**同一输入应从 `boundary_violation` 变为「执行」**（可复现、可先红）
   ⇒ ⚠️ 但仍改授权默认值 ⇒ 待批
② 再核 ORCHESTRATE 的触发词表（三类 · 纯读代码）
③ DEPLOY 留待 Peter 定（二类 · 涉及外发权限）
```

---

## 4. 可复现命令

```bash
# 起真装置（两条线）
go build -o /tmp/vhs-B ./cmd/vhs-asr && go build -o /tmp/vhs-A .
VHS_ASR_ADDR=127.0.0.1:8123 /tmp/vhs-B &
/tmp/vhs-A serve &            # 端口从日志读（--addr 被忽略）

# 打一个写类意图
TID=$(curl -s -X POST http://127.0.0.1:8765/v1/run \
  -H 'Content-Type: application/json' -d '{"text":"跑测试"}' | python3 -c 'import json,sys;print(json.load(sys.stdin)["task_id"])')
curl -s "http://127.0.0.1:8765/v1/tasks/$TID"
# 期望看到：attribution.detail = "space_check 拒绝（boundary_violation）"
#           receipt = "…结果：BOUNDARY_VIOLATION：越界（不在域声明的工具/范围内）…"
```

---

## 5. 未确认（如实）

```
· **只跑了 8 个输入**（6 个到门禁）⇒ **不外推**
· **未核** ORCHESTRATE 的触发词表 ⇒ "识别不出来"**可能只是样本不足**
· **未真跑** `Query/Ask`（按代码应能过 global）与 `RegisterTool`（caps=[read]）
· **未核** `docs/` 是否已有"默认域 / deploy 域"的既有裁决 ⇒ **可能已有结论我没读到**
· **未核** `Orchestrate → project` 是否已能过门禁（我的输入含指代词，被 refer 层拦了）
· `space.Check` 的 `Grant` 是**硬编码 `Authorized: true`** ⇒ ⑤`default_deny` **不是**拒因（已读代码，未改）
· **未改任何代码**（本卡只是定位与选项）
```
