# 三 OPC 核心开发团队：组织方式与可执行 SOP

> 适用引擎：research-opc（trelva，`/opt/research-auto/run_loop.py` v2，经 codex exec → gpt-6-astra）
> 原则锚点：北极星 = 每个合格结果消耗的人工注意力分钟数；责任清楚、交互充分；fail-closed。
> 日期：2026-10-01。本文只给可照抄的命令，不重新发明入队协议。

---

## 1. 现状盘点（单 worker 顺序消费）

- 访问：`ssh trelva`（tailnet 100.64.0.1，user=ubuntu，免密 sudo）。**run_loop 必须以 root 跑且 `HOME=/home/ubuntu`**（root 读 0600 契约；codex 登录态在 ubuntu 下）。
- 消费模型：**单 worker、顺序**。`run_loop.py` 每次取 `tasks.json` 中 `status:queued` 且 `priority` 最小的一条执行，跑完落 `results/`。没有自动轮询（crontab 的 `run_pool_cycle.sh` 整段 `OPC_HOLD` 注释）。
- 硬约束（门校验，任一不过即 `BLOCKED_PAID_EXECUTION`）：
  1. prompt 必须在 `/opt/research-auto/prompts/{ID}.md`，正文自带 `REPORT_BEGIN`…`REPORT_END`（取最长 >100 字段落落盘）。
  2. `tasks.json` 被 `chattr +i` 锁死，**sanctioned 写法 = `chattr -i` → 追加 → `chattr +i`**；run_loop 自己 `save_tasks` 撞 +i 崩是设计如此，结果已先落盘，正常。
  3. 契约 `/etc/research-opc/paid-approvals/{ID}.json` 必须 `root:root mode 600`，且 `model` 恒为 `gpt-6-astra`、`prompt_sha256` 与当前 prompt **逐字节一致**、`max_attempts:1`、仅 `attempts==0` 放行。
- 实测量级：单任务 ~47,386 tokens / 300s / 26.5KB 报告。这是成本与排期的基准。

**推论**：角色差异**不能靠换模型**（model 被锁死），只能靠「三套提示词模板 + 任务队列 + 验收标准」区分。需要更强/更便宜模型（如评审）走 model-center 直连，见 §2 末。

---

## 2. 三 OPC 角色设计

| 路 | ID 前缀 | 职责 | 任务粒度 | priority |
|---|---|---|---|---|
| PLANNER | `VS2-PLAN-{NNN}` | 把一个 feature 拆成可并行的 CODER 任务清单（含依赖、验收标准、顺序） | 1 个 feature = 1 条 PLAN | 10 |
| CODER | `VS2-CODE-{NNN}-{slug}` | 实现单个任务：输出补丁/代码 + 自测证据 | 1 个无依赖的子任务 = 1 条 CODE | 20（同批内按依赖再分 20/21/22） |
| REVIEWER | `VS2-REV-{NNN}` | 独立评审 CODE 产出是否满足验收，给 PASS / 修订项 / 驳回 | 1 个 CODE = 1 条 REV | 30 |

**每个 prompt 模板必含四块**（写进 `{ID}.md` 正文，且包住 `REPORT_BEGIN/END`）：
1. **输入契约**：明确本任务拿到的上游材料（PLAN 摘要 / spec 路径 / 代码 diff），以及「不读、不猜、不补」的边界。
2. **输出格式**：要求 `REPORT_BEGIN` 开头，结构化小节（PLAN→任务表；CODE→文件清单+补丁+如何验证；REV→逐条验收 PASS/FAIL+证据）。
3. **验收标准**：可机械核对的条目（如 `go build` 过、`go test ./...` 全绿、30 条样例 ≥27 条符合预期）。
4. **禁止事项**：禁止改无关文件、禁止自报 `confidence` 当依据、禁止把 PLAN 的自述当事实、禁止对外副作用、失败就明确写 `OUTCOME_UNKNOWN`。

**角色差异实现方式**：同模型 gpt-6-astra，差异 100% 在模板与验收清单。REVIEWER 的输入**只给 spec + code diff**，**不给 PLANNER 的完成自述**（防自我强化，见 §7）。

**需要模型差异时的替代通道（model-center 直连，不走 paid-approvals）**：
```bash
# 在 trelva 或本机均可（creds 在本机 ~/.modelcenter/creds.txt；服务器上若没有就 scp 一份过去）
KEY=$(grep '^MAIN_KEY' ~/.modelcenter/creds.txt | sed -E 's/.*=\s*//')
curl -s https://model.peterzou.com/v1/chat/completions \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"model":"<强模型名，以 GET /v1/models 实际返回为准>",
       "messages":[{"role":"user","content":"<独立评审 prompt>"}]}'
```
用途示例：用更强模型做二审、或用更便宜模型批量初筛。**这是手工调用，不进 run_loop、不占付费契约、不计入 OPC 队列。**

---

## 3. 任务拆解与结果合并协议

一个 2.0 feature 的流水线：
```
[人] 写 PLAN prompt → OPC(PLAN) → 任务清单(含依赖 DAG)
   → [人] 选第一批无依赖 CODER（2~3 条，勿一次塞满）
   → OPC(CODER 批1) → 人工取回补丁
   → [人] 入队对应 REVIEWER（每条 CODE 一条 REV）
   → OPC(REVIEWER) → 逐项 PASS/FAIL
   → [人] 人工 merge PASS 的补丁；FAIL 的打回重写（新 CODE 任务，非重试）
   → 依赖解锁后入队 CODER 批2……循环
```
- **并行真相**：run_loop 单 worker，"并行"=**人工分批入队 + priority 排序**，物理执行仍是顺序；收益是省掉人在每步之间的构思时间，不是墙钟并行。
- **priority 用法**：PLAN=10 先跑；同批 CODER 按依赖设 20/21/22（基础模块 20，依赖它的 21）；REV=30 最后。
- **结果回流**：REVIEWER 报告是合并的唯一依据。**REVIEWER 没 PASS 的代码，即使 CODER 自称完成也不 merge。**

---

## 4. 队列与并行机制

**当前可行（V0，已实测）**：单 worker 顺序 + 人工分批 + priority。一次只让队列里有「一个 PLAN 或一个小批 CODER/REV」，跑完一批再起下一批。
- 命令模板（详见 §6）：入队一条 → `setsid run_loop` → 等 `results/{ID}-*.md` → 取下一批。

**V1 多 worker 设计（未实测，先纸面）**：
- 做法 A（推荐起点）：**分目录分队列**。复制为 `run_loop.planner.py / run_loop.coder.py / run_loop.reviewer.py`，各自读 `tasks.{role}.json`、写各自结果目录，三个 `setsid` 实例同时跑。
- 做法 B（不推荐）：多实例共用一个 `tasks.json`——**并发写 + `chattr +i` 必然冲突**：任一实例保存时 `EPERM`，且无文件锁会丢条目。**禁止直接这么做。**
- 解法约束：多 worker 时**每个 role 一个独立 json 文件**（各自 chattr -i/+i），CODER 之间也只共享"任务清单"这个 PLAN 产出，不共享可变队列。真正的并发安全由人在批与批之间把关。

---

## 5. 人工验收门禁（paid-approvals 天然赋予）

- **签署即门禁**：每写一份 `paid-approvals/{ID}.json` = 人花一次注意力、付一次钱。没有签署就不会执行，天然 fail-closed。
- **`max_attempts:1`**：失败/超时不自动重试；要重跑 = 人重新写 prompt、重算 sha256、重新签契约（这是特性，防止烧钱空转）。
- **合并前必核**：人工比对 `results/{ID}-*.md` 回执与实际文件/测试输出，**一致才 merge**；不一致标 `OUTCOME_UNKNOWN`，不重放、不换模型硬跑。
- **human hard-stops（禁入 OPC，必须人亲自做）**：① 动钱/付费/订阅/发邮件/对外承诺；② 突破既定边界（发外部消息、开端口、改 crontab 的 OPC_HOLD、改 chattr 治理锁）；③ 越权读写生产数据；④ 任何"让运行中的 harness 自编译并给自己增权"的动作。OPC 只产出**草稿/补丁/评审意见**，落盘与发布永远由人完成。

---

## 6. 一个 feature 走完三 OPC 的完整命令序列

```bash
# ===== 0. 本机(mac)准备 =====
mkdir -p ~/vs2-prompts
cat > ~/vs2-prompts/VS2-PLAN-001.md <<'EOF'
# PLAN: <feature 一句话>
输入契约：<本 feature 的目标、范围、已有约束>
REPORT_BEGIN
（PLANNER 在此输出：可并行子任务表 / 依赖 / 每条验收标准）
REPORT_END
禁止事项：不读现有代码以外的目录；不做范围外设计；未知标 unknown。
EOF

# ===== 1. 上传 prompt 并放到 prompts/ =====
scp ~/vs2-prompts/VS2-PLAN-001.md trelva:/tmp/
ssh trelva 'sudo cp /tmp/VS2-PLAN-001.md /opt/research-auto/prompts/VS2-PLAN-001.md \
  && sudo chown ubuntu:ubuntu /opt/research-auto/prompts/VS2-PLAN-001.md'

# ===== 2. 算 prompt 的 sha256（契约要逐字节一致） =====
SHA=$(ssh trelva "sha256sum /opt/research-auto/prompts/VS2-PLAN-001.md | cut -d' ' -f1")

# ===== 3. 签付费契约 root:root 600 =====
ssh trelva "sudo bash -c 'cat > /etc/research-opc/paid-approvals/VS2-PLAN-001.json' <<JSON
{\"approved\":true,\"task_id\":\"VS2-PLAN-001\",\"model\":\"gpt-6-astra\",\"prompt_sha256\":\"$SHA\",\"max_attempts\":1}
JSON
sudo chown root:root /etc/research-opc/paid-approvals/VS2-PLAN-001.json \
  && sudo chmod 600 /etc/research-opc/paid-approvals/VS2-PLAN-001.json"

# ===== 4. tasks.json 入队（chattr -i → 追加 → +i） =====
ssh trelva 'sudo chattr -i /opt/research-auto/tasks.json'
ssh trelva "sudo python3 -c \"import json;p='/opt/research-auto/tasks.json';d=json.load(open(p));d.append({'id':'VS2-PLAN-001','status':'queued','attempts':0,'priority':10});json.dump(d,open(p,'w'),ensure_ascii=False,indent=2)\""
ssh trelva 'sudo chattr +i /opt/research-auto/tasks.json'

# ===== 5. 手动触发单 worker =====
ssh trelva 'sudo HOME=/home/ubuntu setsid /usr/bin/python3 /opt/research-auto/run_loop.py >/tmp/runloop.log 2>&1 </dev/null &'

# ===== 6. 取回结果（约 5 分钟后） =====
sleep 300
scp 'trelva:/opt/research-auto/results/VS2-PLAN-001-*.md' ~/vs2-prompts/
cat ~/vs2-prompts/VS2-PLAN-001-*.md   # 人工读 PLAN 产出，挑第一批 CODER 子任务

# ===== 7. CODER / REVIEWER 复用同一套命令：换 ID、换 prompt、换 priority =====
#    CODER: VS2-CODE-001-notelist, priority=20；REVIEWER: VS2-REV-001, priority=30
#    把上面 VS2-PLAN-001 全部替换即可；sha256 对各自 prompt 重算。
# ===== 8. 人工 merge：只 merge REVIEWER=PASS 的补丁；FAIL 的开新 CODE 任务 =====
```

---

## 7. 护栏与失败模式

- **成本上限**：以实测 ~4.7 万 token/任务为基准。一个 feature ≈ 1 PLAN + N CODER + N REVIEWER；入队前先心算 N，**单 feature 总 token 预算建议 ≤ 30 万（约 6~7 个任务）**，超了先停手复盘拆解粒度，而不是继续塞队列。
- **OPC_HOLD 含义**：crontab 自动接单是关闭的。任何任务都是人手动 `setsid` 拉起——**没有"系统自己悄悄花钱"这回事**。不要为了省事去解开 OPC_HOLD。
- **防自我强化**：REVIEWER 与 PLANNER/CODER 在**输入与输出上隔离**——REVIEWER 只拿 spec + diff，禁看 PLAN 的完成自述；验收用可机械核对的条目，不采信 CODER 的自我评估。防止"模型越来越熟悉自己的叙事却偏离真实工作"。
- **fail-closed 文化**：超时/崩溃/sha 不一致/契约不符 → 一律 BLOCKED 或 OUTCOME_UNKNOWN，**不自动重试、不换端点硬跑、不把部分产出当成功**。失败要留痕（结果文件 + 人工备注），用于下次拆解。
- **最大警示**：最危险的指标是"本周排了多少 OPC 任务"，最有用的是"本周少花了多少注意力"。如果三 OPC 跑起来产出的报告比真实 merge 的代码多，方向已偏——回到那个最值得每天用的场景（参考第 14 天决定）。
