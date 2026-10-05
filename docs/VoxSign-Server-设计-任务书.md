# VoxSign Server 后端设计 · 任务书
（Claude Code × VoiceSign Harness 本机 CLI 互调执行版 · Peter Mac）

版本：v2 · 2026-10-05 · 执行机：Peter Mac（zouyongming）

---

## 一、执行模式（CUI/CLI 概念，本机互调）

- **方案 A（主）**：Claude Code（本机 `claude`，v2.1.284，已授权）作为指挥者，用 Bash 工具调用 VoiceSign Harness 本机命令，读实测结果，完成设计。
- **方案 B（对照）**：VoiceSign Harness 的 `run` 工具（`tools/executor.go` 的 `exec.CommandContext`）执行 `claude -p "<子任务>"`，由 Claude 完成子任务，harness 收结果写回执。
- 两个方案都实测：能否跑通、耗时、输出质量。

## 二、背景与硬约束

- VoxSign：语音驱动的开发助手（品牌 VoxSign/voxsign.ai）。本机：`~/voicesign-harness`（Go 主仓）、`~/voxsign`、`~/.voicesign/harness`（实测轨迹）。
- 目标：VoxSign Server（服务器端后端）独立完整设计。
- 三条硬约束（逐条核对，缺一不验收）：
  1. **独立完整**：覆盖背景/边界/架构/服务/企业端/安全/部署/路线图。
  2. **闭源**：Server 实现闭源，仅 API/MCP 契约公开，实现不进开源仓库。
  3. **企业端原生**：多租户/RBAC/SSO/审计合规/计费/SLA/私有化部署必须原生纳入。

## 三、执行步骤（严格按序）

1. **现状盘点（只读）**：`~/voicesign-harness/README.md`、`ARCHITECTURE.md`、`docs/` 最近定稿（详细设计-语音驱动开发-v2定稿、新方案-Claude定稿）、`config/model-center.json`、`tools/registry.go`、`tools/executor.go`、`~/voxsign/`、`~/.voicesign/harness/notes.md` + `trajectory-*.jsonl` → 产出「已实现能力清单」。
2. **能力基线实测（用 Harness）**：通过本机入口逐项验证，输出 capability model 表（能力/状态：真执行·占位·缺失/证据）：QUERY、EDIT(单行)、NOTE、COMMIT、ORCHESTRATE、TEST、DEBUG、DEPLOY，及语音链路、Center/WS。
3. **差异设计（现状→目标）**：边界与闭源策略；总体架构（接入/网关/闭源服务/数据/部署）；核心服务（身份租户、语音、任务编排、模型网关、计费配额、审计合规）；企业端（多租户/RBAC/SSO/审计/计费/SLA/私有化）；安全；部署运维（AIOps key、监控、备份、非中国境内）；开放问题（≤3，各给建议）；路线图（PHASE 0–4）。每条设计必须有出处（代码路径或实测证据）。
4. **产出**：`VoxSign-Server-设计.md` + 能力基线报告 + 差异映射表（现有能力→设计落点→固化/补全/新建）。
5. **自检**：三硬约束逐条核对；无依据不写。

## 四、验收标准

1. 设计锚定现状：每个服务设计引用具体代码/接口/实测证据。
2. 三条硬约束贯穿。
3. 开放问题 ≤3 且各有建议。
4. 方案 A/B 各有实测记录（能否跑通、耗时、输出质量）。
5. 交付：设计 md + 基线报告 + 差异表；豆包侧转 HTML + 飞书双交付（手机可点）。

## 五、环境事实

- Claude CLI：`claude`（PATH 内，v2.1.284，已授权）
- Harness：`cd ~/voicesign-harness && go run . repl`（交互）/ `go run . serve`；cmd：`./cmd/vhs-asr` 等
- AIOps key：`~/.aiops/keys/peter-mac.key`（export AIOPS_KEY=... 格式）；harness 网关鉴权 `X-AIops-Key`；本任务优先本机互调，网关仅对照
- 品牌：VoxSign/voxsign.ai（对外禁 VoiceSign）
