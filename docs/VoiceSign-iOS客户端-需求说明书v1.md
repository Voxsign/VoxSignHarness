# VoiceSign iOS 客户端 · 需求说明书 v1

> 本文件由测试项目「VoiceSign Harness × iOS 客户端」输入，2026-10-04。
> 用途：作为喂给 VoiceSign Harness 编排管线的需求输入，同时作为 iOS 端接口设计的实体契约。

## 1. 目标

为 VoiceSign Harness（Go 后台服务）配套一个 iOS 原生客户端，把「做 iOS 客户端」作为 Harness 的**测试项目**：
客户端在真实设备/模拟器上通过 Harness 公开接口完成一轮完整交互（语音→意图→执行→回执→决策点→终态），
用端到端结果反证 Harness 的契约符合度、稳定性与体验。

## 2. 系统边界（接口设计，由本说明书定稿）

```
iOS 客户端（薄终端）                    VoiceSign Harness（厚服务）
─────────────────────────              ─────────────────────────
[语音采集 SFSpeechRecognizer]           /v1/tasks   POST 提交任务
[文本输入]                    ──→       /v1/tasks/{id}/events  SSE 事件流
[投递队列 DeliveryQueue]                /v1/tasks/{id}/answer   决策点应答
[通知 NotificationService]              /v1/tasks/{id}/rollback 回滚
[连接管理 ConnectionManager]            /v1/tasks/{id}/cancel   取消
                                       /v1/status /v1/health   探活
```

### 2.1 认证

- 全请求带 `Authorization: Bearer <token>`；token 在设置页配置（默认 `ios-test-token`）。

### 2.2 任务提交（POST /v1/tasks）

- 请求体：`{"text":"<用户原话>","mode":"voice|text"}`（voice 表示语音转写结果，text 表示键盘输入）。
- 响应：`{"task_id":"<id>","status":"pending","created":<unix>}`。
- 客户端保存 `task_id`，随后订阅 SSE。

### 2.3 事件流（GET /v1/tasks/{id}/events）

- 事件类型：`stage` / `need_ask` / `need_confirm` / `done` / `failed` / `interrupt` / `canceled`。
- data 必含 `seq`（单调递增，从 1 起）。
- 断线重连：`?after=<lastSeq>`，服务端重放 `seq > after`，不重复不丢。
- 连接保持到终态（done/failed/canceled）后关闭。

### 2.4 决策点（POST /v1/tasks/{id}/answer）

- 请求体：`{"answer":"<option_id 或自然语言>"}`。
- `need_ask` 应答后继续推流；`need_confirm` 答 `执行` 放行。

### 2.5 终态与回执

- `done`：data 含 `receipt`（四行文本）、`reversible`、`role`。
- `failed`：data 含 `error`。
- 可回滚任务：客户端展示 `rollback` 按钮 → POST /v1/tasks/{id}/rollback。

### 2.6 打断

- 客户端点「停」或识别到打断意图 → POST /v1/tasks/{id}/cancel。
- 服务端广播 `interrupt`（applied/notApplied/canRollback），随后 `canceled`。

## 3. 客户端架构（薄终端，三通道）

| 通道 | 职责 | 实现 |
|---|---|---|
| AudioChannel | 采集 + 本地识别（支持常听/按需切换） | SFSpeechRecognizer + AVAudioSession（playAndRecord） |
| DeliveryChannel | 提交、幂等重试、离线队列 | DeliveryQueue（持久化 + request_id） |
| EventChannel | SSE 订阅、全局重连、决策点驱动 | SSEClient（after 重放语义） |

配套：NotificationService（need_ask/need_confirm/done 后台通知）、ConnectionManager（网络探活 + 指数退避）、
SettingsStore（token/地址/常听开关）。

## 4. 后台能力（本次 T1 范围）

1. `UIBackgroundModes: audio`（常听模式后台持续采集，默认按需关闭）。
2. 投递队列：断网/后台期间提交入队，恢复后按序补投，幂等。
3. 本地通知：后台收到 need_ask / need_confirm / done / failed 时提醒。
4. 全局连接管理：SSE 断线指数退避重连 + `after` 重放。

## 5. 验收判据（端到端，真实服务）

1. 提交一条任务 → SSE 收到 stage…→ done，seq 严格递增、无重放。
2. 决策点：need_ask 到达后挂起，answer 后继续推流至终态。
3. 打断：cancel 后 interrupt 即时到达，随后 canceled 关闭。
4. 断线重连：after 重连不重复不丢。
5. 回滚：reversible=true 时 rollback 成功并出回执。
