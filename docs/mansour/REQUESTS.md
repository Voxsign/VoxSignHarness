# Mansour（美墅）需求台账 REQUESTS.md

> 统一登记美墅对 VoxSign Harness / UniFusion 的需求（M 编号）。
> 流转规则：豆包只登记与监督验收；**改码由 Harness（美墅）自主执行**，不代写。
> 状态：`OPEN`（待美墅认领）→ `IN_PROGRESS`（已认领/改码中）→ `DONE`（已部署并验收）→ `BLOCKED`（受阻，需澄清）。

---

## M001 · 邮件正文 HTML 需提取纯文本（bodyText）

- **状态**：OPEN（2026-10-09 登记）
- **背景**：工作指令邮件以 HTML 发送，`Email.bodyText` 为空（仅存 `bodyHtml` + 300 字 `bodyPreview`）。Harness 的洞察生成与知识内化读取 `bodyText || bodyPreview`，导致**只看到摘要、看不到完整指令**，无法按邮件执行任务。
- **需求**：邮件同步（`src/lib/email.ts` 的 `splitBody`）时，HTML 正文需同时提取纯文本存入 `bodyText`（去标签/实体解码/压缩空白），且对存量邮件补写一次。
- **验收标准**：新同步的 HTML 邮件 `bodyText` 非空；存量邮件补写后知识内化/洞察可读取完整正文。
- **备注**：豆包曾临时打过一版补丁（已编译未重启生效），美墅可采纳/改进/重写后自行部署验证。

## M002 · 洞察上下文应使用完整正文（bodyText 优先）

- **状态**：OPEN（2026-10-09 登记）
- **背景**：`harnessMl.ts` 构建【新邮件】上下文时仅取 `bodyPreview` 前 180 字，无法支撑基于完整邮件内容的洞察判断。
- **需求**：上下文改用 `bodyText || bodyPreview`（完整正文优先、摘要兜底），截断长度放宽到 800 字。
- **验收标准**：含完整指令的邮件进入上下文后，Harness 生成的洞察能识别并引用指令内容（如"3天成长计划""Karim 缺口清单"）。

---

## 执行说明（写给 Harness）

- 认领：在对应条目状态改为 `IN_PROGRESS` 并回信确认。
- 完成后：更新状态 `DONE` + 写明改动文件、验证结果，回信 Sofia 报进展。
- 涉及服务器部署：改码 → `npm run build` → `pm2 restart unifusion` → 实测验收。
