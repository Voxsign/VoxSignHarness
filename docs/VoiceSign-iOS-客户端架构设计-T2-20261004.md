# VoiceSign iOS 客户端 · T2 豆包式交互架构设计

> 版本：2026-10-04 · 基线工程：vhs-live/ios（VoxSign Harness 工作区）
> 目标：把 iOS 客户端做成"豆包式交互"，把 VoxSign Harness 的执行优势全部亮出来。

---

## 1. 背景与目标

Harness（后端）已具备：意图分类（纯规则）、编排（ORCHESTRATE 多步）、SSE 流式事件、决策点确认、回执可撤销、request_id 幂等、ASR 个性化纠正。iOS 客户端的问题（用户实测）：**一说就卡死**——根因是 SSEClient `waitsForConnectivity=true` 在网络不可达时无限期等待 + 重连循环无上限刷屏；且客户端是"胖状态机"，对网络切换无感知。

T2 目标（对齐豆包式交互体验）：
1. **开口即达**：常听模式下说完一句话自动提交，不用按发送键。
2. **流式呈现**：SSE 事件驱动执行卡/角色/回执渐进渲染（已具备，保持）。
3. **随时打断**：说"停"立即生效（已具备，保持）。
4. **网络透明**：顶部连接胶囊（绿/黄/灰），断网指令自动排队，恢复自动补投——不再黑盒卡死。
5. **展示自建服务**：连接状态、执行过程、决策点全透明，让 Harness 的能力看得见。

## 2. 范式检查（paradigm-first）

| 维度 | 旧范式（现状，vhs-live 旧版） | 新范式（T2） |
|---|---|---|
| 客户端定位 | 胖状态机：自管轮询/SSE/状态，出问题就卡 | **薄客户端**：只做"语音转文本 + 流式呈现 + 决策交互 + 离线补偿" |
| 网络处理 | 盲目重连（waitsForConnectivity 无限等、0.8s 无限刷屏） | **连接感知**：NWPathMonitor + 心跳 + 指数退避 + 状态机 |
| 断网行为 | 提交失败 → 卡死/报错 | **离线排队**：指令进 DeliveryQueue，恢复自动补投（request_id 幂等去重） |
| 语音交互 | 点按开始/停止 + 手动发送 | **常听 + 说完自动提交**（onFinalSegment 回调） |

约束变化：手机网络随时切换（Wi-Fi↔蜂窝↔离线），服务器可重启。客户端必须"连接感知 + 自动自愈"，否则任何一次断网都是卡死事故。

## 3. 质量属性（排序）

1. **可用性**（最高）：断网不丢指令、恢复自动补投、卡死场景根除。
2. **低延迟感知**：首响应 <1s（提交前探测避免 15s 傻等）。
3. **可观测性**：连接状态胶囊 + 诊断行 + DiagLogger，不黑盒。
4. **可维护性**：分层清晰（连接层/传输层/队列层/交互层），契约复用。
5. 成本/性能：零第三方依赖（纯 URLSession + SwiftUI），保持。

## 4. 系统架构（C4 摘要）

```
[iPhone]  VoiceSign iOS (T2)
├── 交互层 (Views)         ConnectionStatusView · RootView · SettingsView
├── 状态层 (AppModel)      连接订阅 · 提交前探测 · 离线入队 · 自动补投
├── 语音层 (Speech)        常听模式 + onFinalSegment 自动提交
├── 连接层 (ConnectivityService) ★新  状态机 + 心跳 + NWPathMonitor
├── 传输层 (APIClient/SSEClient)    契约复用，SSE 竞速超时修复
├── 队列层 (DeliveryQueue) T1 已有，离线补偿
└── 后台层 (NotificationService)    T1 已有，锁屏提醒
        │  HTTPS / SSE (Bearer)
[Mac]  VoxSign Harness（服务端，不改）
        /v1/tasks · /v1/tasks/{id} · /v1/tasks/{id}/events · /answer · /cancel · /rollback · /v1/status · /v1/roles
```

**接口设计结论：不加新端点**。底层标准化——复用 Harness 既有契约：
- 心跳 = `GET /v1/status`（轻量、快）
- 离线补投 = `POST /v1/tasks` + `request_id`（服务端 deduped 契约天然去重）
- SSE 断点续传 = `GET /v1/tasks/{id}/events?after=lastSeq`（已具备）
- 差异化全在客户端编排（连接感知/队列/语音自动提交）

## 5. ADR 决策记录

| ADR | 决策 | 备选（放弃） | 理由 |
|---|---|---|---|
| ADR-001 | 薄客户端范式：iOS 不做业务判断，全委托 Harness | 胖客户端（本地缓存决策逻辑） | 业务判断是 Harness 差异化资产；客户端做判断会漂移、双倍维护 |
| ADR-002 | 连接感知替代盲目重连：NWPathMonitor+心跳30s+指数退避(0.8→4s,5次上限) | 无限重连 / 手动刷新 | 无限重连=卡死根源；手动=体验差 |
| ADR-003 | 提交前探测（isReachable），offline 直接入队 | 无条件 POST 等 15s 超时 | 15s 傻等=用户感知"卡死" |
| ADR-004 | 语音常听+final 自动提交（onFinalSegment） | 仅点按录音 | 豆包式开口即达；常听可设置页关闭（省电/隐私） |
| ADR-005 | SSE 竞速超时 10s + 去 waitsForConnectivity | 保持原配置 | waitsForConnectivity=true 在不可达时无限等，是"卡死"直接根因 |

## 6. 关键流程

**在线语音指令**：常听收音 → final 文本 → sendVoice → 探测 online → POST /v1/tasks → SSE 流式推进执行卡 → 决策点/回执。

**离线指令**：常听收音 → final 文本 → sendVoice → 探测 offline → 入 DeliveryQueue（request_id）→ 顶部胶囊变灰 → 网络恢复 → onOnline 回调 → flushQueue 幂等补投 → 恢复在线流。

**SSE 断线**：竞速超时 10s 抛错 → 指数退避重连（0.8→1.6→2.4→3.2→4s）→ 第 5 次失败停止并提示 → 用户可重试。

## 7. 风险登记册

| 风险 | 影响 | 概率 | 缓解 |
|---|---|---|---|
| 常听模式耗电/隐私顾虑 | 中 | 中 | 设置页开关（默认关）；按需模式仍可用 |
| 心跳 30s 间隔期间服务重启 | 低 | 中 | NWPathMonitor 恢复/提交前探测兜底 |
| iOS 后台挂起 SSE 长连接 | 中 | 中 | NotificationService 锁屏提醒 + 回前台重连 |
| 离线队列积压过多 | 低 | 低 | DeliveryQueue 上限 + 设置页补投按钮 |

## 8. 演进路线

- T2（本次落地）：连接感知 + 卡死修复 + 语音自动提交 + 状态胶囊 ✅
- T3：流式打字机（回执逐字渲染）、按住说话按钮（VAD）、多任务面板
- T4：接入 aiops.peterzou.com 自建平台展示（用户偏好），把 Harness 能力对外开放演示
