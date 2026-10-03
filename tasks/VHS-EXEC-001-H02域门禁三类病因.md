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


---

## 6. ⭐ 补充：**裁决早已存在**（2026-10-03 本轮 `grep` 到）

**`docs/research-inputs/13-Codex-域评审.md`（**17 行**）**：
```
第 7 行：「按"资源归属与授权生命周期"划分，不按任务建域。
        **默认域应是"无写权限＋限定读取"，不能全局可读**；
        每个活跃项目一个**持久域**…类型只提供模板，**权限以显式策略为准**。」
第 13 行：「模型只输出**候选域**，代码用**注册名称、别名和当前工作位置**消歧。
        **唯一匹配且已有授权才执行**；歧义时可继续限定范围内的只读准备，**写入前确认**。
        **未匹配、跨域、目标不符均拒绝，不能退回更宽权限**。
        切域不自动继承授权，运行中**每次工具调用都重新校验**。」
第 16 行：「…先做版本化注册表、**默认拒绝的统一执行检查点**、越界测试与审计，再接意图解析。
        **先证明无法绕过，再优化一句话体验**。」
```

**⇒ 它排除修法 A 与 B**：
```
A（给 global 加写权限） ⇒ 违背「默认域应是无写权限＋限定读取」
B（写类意图默认落 project = 更宽权限） ⇒ 违背「**不能退回更宽权限**」
⇒ **答案 = C**（消歧失败即拒绝/回问）—— **而这是既有裁决，不是新选择**
```

## 7. C 的执行前提（已核）
```
`server/server.go  type runReq struct { Text string }` ⇒ **入参无域字段**
而裁决第 13 行说域由**代码消歧**（注册名 / 别名 / 当前工作位置）
⇒ **C 的完整实现 = 一套"域消歧"机制** ⇒ **是功能，不是一行改动**
⇒ 而 §5.1 说的「声明域内工具」**只是其中一环**；真正缺的是**域消歧**。
```

## 8. 未确认（如实）
```
· ⚠️ `docs/research-inputs/` 是**提案/评审** ⇒ **未必是最终采纳的裁决**
  ⇒ **未核**它是否被采纳（可能在 `docs/SPEC-v2` 或 v2 详细设计里被改写）
· 我**只读了 `13` 的 3 段**（共 17 行）⇒ 全文未逐行读
· **未读** `docs/DeepSeek-全景审阅稿-v1.md`（也可能含相关裁决）
· **未核**"域消歧"是否已有实现（如 `input/` 里是否已提取工作位置）
· **未改任何代码**
```

## 9. ⚠️ 我犯错的模式（本条最重要的教训）
```
我在 §5/§6 里提出 A/B/C 三选项「待 Peter 裁」—— 而**裁决文档就在 17 行的文件里**
⇒ **我提的 B 方案被裁决明确禁止** ⇒ 若按我的选项批了，**我会做出违背架构裁决的改动**
⇒ 这是今天**第三次**同族错误（前两次：读贵校准报告 / skill 层孤岛登记）
⇒ **共同点：我把"我没想到"当成了"不存在"，然后据此提方案。**
⇒ 机械化修法（下一轮做）：**写"未核/没有/不存在"之前，强制先跑一次搜索**
     `git grep -l "<关键词>" -- 'docs/*.md' 'tasks/*.md'`  ⇒ 有命中必须先读再写
