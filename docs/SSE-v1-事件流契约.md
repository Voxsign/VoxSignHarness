# VoxSign · SSE 事件流契约（M6-1 冻结，server/web/iOS 三方共同依据）

- 版本：v1.0 · 2026-10-02 · 冻结状态（实现方按此对接，改动需组织者裁决）

## 端点

- `GET /v1/tasks/{id}/events`，认证 `Authorization: Bearer <token>`（全来源必带）。
- 响应：`Content-Type: text/event-stream`；连接保持到任务终态（done/failed/canceled）后关闭。
- 旧轮询端点（GET /v1/tasks/{id}）保留不动——SSE 客户端以此做断线兜底。

## 事件格式（SSE 标准）

```
event: <type>
data: <json>

```

- data 内**必含 `seq` 序号**（单调递增，同任务内不重复；从 1 起）。
- data 内的其余字段见下；字段缺失时消费者按缺省处理，不得因缺字段崩溃。

## 事件类型

| event | data 字段 | 语义 |
|---|---|---|
| `stage` | `seq, role, phase, step` | 阶段变更（执行卡滚动行）；role=planner/executor/verifier；step 为中文阶段名（如"意图分类""域裁决""风险分级""确认闸""执行""校验""归因"） |
| `need_ask` | `seq, question, options:[{id,label}]` | 回问决策点（一屏一决策点；answer 后继续推） |
| `need_confirm` | `seq, question` | 强确认决策点（红条；answer:"执行"） |
| `done` | `seq, receipt, attribution, reversible, role` | 完成（receipt 为四行文本字符串；role 为终态角色） |
| `failed` | `seq, error` | 失败终止 |
| `interrupt` | `seq, applied, notApplied, canRollback` | 打断生效广播（"停"经 POST /v1/tasks/{id}/cancel 触发后立即推给该任务 SSE 连接；applied/notApplied 为描述数组） |
| `canceled` | `seq` | 取消终态（interrupt 后随至，或直接 cancel） |

## 重连幂等

- 客户端保存 `lastSeq`（收到的最大 seq）；断线重连请求带 `?after=<lastSeq>`（或标准 `Last-Event-ID` 头）。
- server 从事件记录重放 `seq > after` 的全部事件，**不重复、不丢**；after 缺省=从首事件起。
- 事件记录与任务持久化同生命周期（重启后 SSE 按重放语义仍可用；未完成任务的 events 记录保留）。

## 打断语义（交互 v2.1 规则 2 三语义）

- 用户说"停"或点停止 → 客户端 POST `/v1/tasks/{id}/cancel`（legacy `/v1/cancel` 兼容）。
- server 立即向该任务 SSE 连接广播 `interrupt`（已生效/未执行/可撤销三语义），随后 `canceled` 关闭。
- SSE 客户端以 interrupt 事件为即时信号（优于轮询感知）；轮询兜底客户端以 canceled 状态为准。

## 验证器（server 侧测试，M6-1 交付必含）

1. 事件序列有序：stage→need_ask→(answer)→stage…→done，seq 递增。
2. 重连幂等：`?after=<lastSeq>` 重连只收到其后事件，无重复。
3. 决策点事件：need_ask/need_confirm 事件在挂起时到达、answer 后续跑推流。
4. 打断即时性：cancel 后 interrupt 事件送达；事件流先于轮询看到终态。
