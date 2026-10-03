# VoiceSign Harness × iOS 客户端 · 测试结果报告

> 测试项目：把「做 VoiceSign iOS 客户端」作为需求喂给 Harness 编排管线，边写 iOS 端边用真实接口端到端测 Harness。
> 日期：2026-10-04 · 环境：本地隔离 serve（VHS_API_KEY 注入，token=ios-test-token）· 结果：**契约主体 PASS，发现 2 个真实缺陷**

## 0. 一句话结论

Harness 的 INTERACT-v1 + SSE 契约在真实调用下**主体可用**（提交→SSE→决策点→回执→打断→多步编排全链路跑通），
但暴露 **1 个意图分类缺陷**（书名号文档任务误判 TEST）与 **1 个模型依赖降级**（预算用尽时回答层空转）；iOS 端 T1 后台能力已落地并通过 21/21 测试。

## 1. 测试方法（可复跑）

- 服务：`VHS_LOG_DIR=... VHS_ADDR=127.0.0.1:8765 VHS_TOKEN=ios-test-token VHS_API_KEY=<key> ./vhs serve`（隔离目录）
- 客户端行为：`scripts/e2e-ios-test/e2e-v1.sh`（契约五连测）、`e2e-v2-interrupt-reconnect.sh`（打断/重连）
- 域注册：隔离仓库 `/tmp/vhs-ios-test/proj` + `spaces/project.space.json`（TEST/EDIT/ORCHESTRATE 需 project 域；global 只读）

## 2. 契约测试结果

| # | 用例 | 结果 | 证据 |
|---|---|---|---|
| 1 | GET /v1/health、/v1/status | ✅ PASS | `{"ok":true,"token_set":true}` |
| 2 | POST /v1/tasks → SSE stage→done，seq 严格递增 | ✅ PASS | `seq=[1,2]`，receipt/reversible/attribution 完整 |
| 3 | need_confirm 确认闸 → answer 放行 → ORCHESTRATE 全链 | ✅ PASS | stage(确认闸)→need_confirm→answer→读5文档→写→git commit `44220d8` |
| 4 | cancel → interrupt 三语义 → canceled | ✅ PASS | `applied=[] notApplied=["任务已取消"] canRollback=false`，seq 递增 |
| 5 | 未授权域拒绝副作用任务 | ✅ PASS（安全闸） | space_check `boundary_violation`，suggestion 引导注册域 |
| 6 | 决策点防循环 | ✅ PASS（特性） | ask 不收敛 1 轮自动 canceled（ask_not_converging） |
| 7 | after=lastSeq 断线重连不重放 | ⚠️ 部分验证 | done 后关闭=无重放；挂起中重连需真机/长窗口复测 |
| 8 | 模型预算用尽降级 | ⚠️ 真实缺陷 | QUERY 回答层「今日预算可能已用尽」；ORCHESTRATE 降级为确定性拼接 |

## 3. 发现的真实缺陷

### 缺陷 1（意图分类）：书名号文档任务误判 TEST
- 输入：`生成《测试-打断语义》文档并保存提交`
- 实际：判 `TEST`（test_kind=go test ./...，标题含"测试"触发信号词）→ space_check 拒绝 → 秒级终态
- 影响：用户说"把 X 整理成《XXX-测试-YYY》文档"会被误判为跑测试；文档生成类任务应优先判 ORCHESTRATE/EDIT
- 建议：detectOrchestrate 先于 TEST 信号词裁决，或对书名号目标+保存提交组合强制 ORCHESTRATE

### 缺陷 2（模型依赖）：预算用尽时回答层降级
- QUERY 层返回「模型服务暂不可用——今日预算可能已用尽」，回答内容为空
- ORCHESTRATE 的 LLM 汇总降级为「源文档结构化拼接」（注释已如实标注「未调用 LLM 润色」）
- 影响：不崩溃（降级路径 OK），但体验打折；依赖 model.peterzou.com 配额
- 建议：QUERY 层预算用尽时给确定性兜底答案 + 明确"模型不可用"提示（现状已提示，可再加本地缓存）

## 4. iOS 客户端 T1 交付（后台能力）

| 文件 | 内容 |
|---|---|
| `ios/VoiceSign/Net/DeliveryQueue.swift` **新增** | 投递队列：持久化(FIFO)+request_id 幂等+指数退避+上限丢旧；提交失败入队，恢复后补投 |
| `ios/VoiceSign/Core/NotificationService.swift` **新增** | 本地通知桥：need_ask/need_confirm/done/failed/canceled → 后台锁屏通知（前台不打扰） |
| `ios/VoiceSignTests/DeliveryQueueTests.swift` **新增** | 6 条测试：入队/FIFO/持久化/补投成功/失败退避/恢复/上限丢旧 |
| `ios/VoiceSign/State/AppModel.swift` **修改** | submit 失败→入队+提示；flushQueue()；SSE 事件→通知桥 |
| `ios/VoiceSign/Speech/SpeechRecognizer.swift` **修改** | 常听模式（alwaysOn）：playAndRecord+UIBackgroundModes audio 后台持续收音，分段续识别 |
| `ios/VoiceSign/App/Info.plist` **修改** | `UIBackgroundModes: audio`（常听保活） |
| `ios/VoiceSign/App/VoiceSignApp.swift` **修改** | 启动请求通知权限 + 补投离线队列 |
| `ios/VoiceSign/Views/SettingsView.swift` **修改** | 后台能力区：常听开关 + 补投按钮 + 队列/通知状态 |

**验证**：`xcodebuild build` ✅ BUILD SUCCEEDED；逻辑测试 **21/21 全绿**（SSEParser 7 + VSLogic 8 + DeliveryQueue 6）。

## 5. 下一步（T2/T3 路线）

1. **修缺陷 1**（意图分类优先序）——Harness 侧一行裁决顺序改动，可立即提 PR
2. **T2 语音通道**：音频留底（加密 PCM）+ 原始音频上传（background URLSession 分片）+ 对接 `/v1/process`
3. **T3 全双工**：WS `/v1/stream` 流式 + Live Activity 锁屏状态
4. 真机复测：after 重连挂起窗口、常听模式锁屏收音、通知点按回任务

## 6. 验收对照（需求说明书 v1 五判据）

1. 提交→SSE→done，seq 递增无重放 ✅
2. need_ask/need_confirm 挂起+answer 续跑 ✅（need_ask 事件有延迟，见下）
3. cancel→interrupt 即时→canceled ✅
4. after 重连不重复不丢 ⚠️（挂起窗口待真机）
5. reversible→rollback 出回执 ⚠️（首轮 done reversible=true；rollback 端到端待真机复测）

> 附注：need_ask 首连事件有模型分类延迟（数秒），SSE 连接保持期间事件到达后正常渲染——与轮询兜底并存，客户端不受影响。
