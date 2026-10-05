# VoxSign · iOS 原生壳（VoxSign App，SwiftUI）

M6-2 交付：把 M5 web 壳（`web/`）迁为**真 iOS App**（Xcode SwiftUI 工程），保留全部九元素，
对接真实 server（INTERACT-v1 REST 端点集 + SSE 事件流）。**零第三方依赖**（纯 Foundation / URLSession / SwiftUI / Speech）。

- 基准：飞书《VoxSign · iOS 对话界面模拟》+ `web/`（M5）+ `docs/SSE-v1-事件流契约.md`（M6-1 冻结）+ `docs/手机端真机测试指引-M3.md`。
- 约束：只新增 `voicesign-harness/ios/`；未触碰 `~/VoxSign`、未推 GitHub、未改 server/Go 代码（端点/SSE 均为只读契约）。

---

## 一、文件清单

| 文件 | 作用 |
|---|---|
| `VoxSign.xcodeproj/project.pbxproj` | Xcode 工程（用 Xcode 15+ 文件系统同步分组，免逐文件枚举；含 app + tests 两 target） |
| `VoxSign.xcodeproj/xcshareddata/xcschemes/VoxSign.xcscheme` | 共享 scheme（build/test/run） |
| `App/VoxSignApp.swift` | App 入口（@main，注入 AppModel/Settings/Speech） |
| `App/Info.plist` | 麦克风/语音识别权限说明 + 局域网 HTTP ATS 放开 |
| `Core/Models.swift` | 值类型：TaskView/Receipt/UndoInfo/Badge/DecisionPoint/SystemBarInfo/RoleInfo |
| `Core/VSLogic.swift` | **纯逻辑层**（1:1 移植 web/logic.js）：状态机、回执四行、撤销裁决、轻标签、决策点路由、角色映射、打断状态机、打断词 |
| `Core/SSEParser.swift` | **纯逻辑层**：SSE 分帧、强类型事件解码、lastSeq、断线重连 `?after=` |
| `Net/SettingsStore.swift` | server 地址 + Bearer token（UserDefaults，等价 web localStorage） |
| `Net/APIClient.swift` | INTERACT-v1 REST（POST /v1/tasks、GET /v1/tasks/{id}、answer、rollback、cancel、/v1/roles、/v1/status），全 Bearer |
| `Net/SSEClient.swift` | GET /v1/tasks/{id}/events，URLSession.bytes 流式 → SSEParser，断线按 after 重连 |
| `Speech/SpeechRecognizer.swift` | SFSpeechRecognizer + AVAudioEngine 基础实现（失败/未授权回退键盘） |
| `State/AppModel.swift` | 编排层（对应 web/app.js）：消息流、提交→SSE 流转、应答/撤销/打断、角色同步 |
| `Views/*.swift` | RootView / MessageViews（气泡·徽章·三点·执行卡·回执卡）/ DecisionZoneView / InputBarView / SystemAndRoleBar / SettingsView |
| `VoxSignTests/VSLogicTests.swift` | XCTest：逻辑层（对齐 web/test.js 44 用例） |
| `VoxSignTests/SSEParserTests.swift` | XCTest：SSE 分帧/seq/重连/打断三语义 |
| `DevCheck/DevLogicCheck.swift` | **开发期** macOS 命令行断言（不在 Xcode target 内，沙箱跑不了模拟器时本机直跑逻辑） |

---

## 二、九元素对照表（基准 → SwiftUI 位置）

| # | 基准元素 | SwiftUI 位置 |
|---|---|---|
| 1 | 对话流：用户=右蓝气泡（语音带声波）；Harness=左白气泡；三点处理中 | `MessageViews.swift` UserBubbleView/HarnessBubbleView/WaveView/TypingView；`RootView.swift rowView` |
| 2 | 轻标签：意图/域/风险小徽章 | `VSLogic.compressBadges`；`MessageViews.swift BadgeView`（每条 Harness/回执下挂） |
| 3 | 执行卡：实时滚动阶段（意图分类→域裁决→风险分级→确认闸→执行→校验→归因） | `VSLogic.execStages`；`AppModel.advanceExec(toStage:)` 由 SSE `stage` 事件驱动；`MessageViews.swift ExecCardView` |
| 4 | 回执卡：绿色四行（动作/文件/结果/撤销）+ 撤销按钮 | `VSLogic.parseReceipt/extractUndo`；`MessageViews.swift ReceiptCardView` → `AppModel.rollback()` 调 `POST /v1/tasks/{id}/rollback`（不可逆禁） |
| 5 | 打断系统条：说"停"→红条（已生效/未执行/可撤销），可关闭 | `VSLogic.interruptSystemBar` + SSE `interrupt` 三语义；`SystemAndRoleBar.swift SystemBarView`；`AppModel.maybeInterrupt` 调 `POST /v1/tasks/{id}/cancel`（404/405 回退 legacy `/v1/cancel`） |
| 6 | 麦克风输入条：真语音→文本可改，否则键盘兜底 + 发送 | `Speech/SpeechRecognizer.swift`（SFSpeechRecognizer zh-CN）；`InputBarView.swift`；未授权/模拟器自动回退键盘 |
| 7 | 多角色折叠条：Planner/Executor/Verifier，active 随状态 | `SystemAndRoleBar.swift RoleBarView`；`AppModel.syncRole`（SSE stage role 直接给 / 状态词经 `roleForStatus` 裁决）；`GET /v1/roles` 已在 APIClient 就绪 |
| 8 | 候选按钮/强确认：need_ask options→answer:{id}；need_confirm 红条→answer:"执行"；一屏一个决策点 | `VSLogic.nextDecisionPoint`；`DecisionZoneView.swift`；`AppModel.answer(_:)` |
| 9 | 服务设置：server 地址 + token（UserDefaults）；request_id 客户端 UUID 幂等 | `SettingsStore.swift`；`SettingsView.swift`；`VSLogic.genRequestId()`（UUID） |

---

## 三、编译 / 测试结果（如实标注）

### 编译门禁（已通过）

```bash
cd voicesign-harness/ios
xcodebuild -project VoxSign.xcodeproj -scheme VoxSign \
  -destination 'generic/platform=iOS Simulator' -configuration Debug \
  CODE_SIGNING_ALLOWED=NO build
# => ** BUILD SUCCEEDED **  产物：VoxSign.app（arm64）
```

测试目标编译：

```bash
xcodebuild -project VoxSign.xcodeproj -scheme VoxSign \
  -destination 'id=<simulator-id>' -configuration Debug CODE_SIGNING_ALLOWED=NO build-for-testing
# => ** TEST BUILD SUCCEEDED **（VoxSignTests.xctest 打包成功）
```

### 单元测试（如实标注缺口）

- **XCTest on iOS Simulator：因沙箱环境未跑通。** 本机沙箱内模拟器报
  `DTServiceHubClient failed to bless service hub ... lockdown`（无法连 instruments 服务），
  这是执行环境限制，**不是代码问题**——测试包已成功编译并打包（见上）。
  在非沙箱的普通终端 / Xcode 里直接跑即可：

  ```bash
  xcodebuild test -project VoxSign.xcodeproj -scheme VoxSign \
    -destination 'platform=iOS Simulator,name=<某个 iPhone>'
  # 或直接 Xcode 打开 ⌘U
  ```

- **逻辑层已在本机直接执行并全部通过（61 通过 / 0 失败）。**
  因 `Core/` 三个纯文件只依赖 Foundation（无 SwiftUI），用 macOS swiftc 直编直跑，
  等价执行 XCTest 里同一套断言：

  ```bash
  cd voicesign-harness/ios/DevCheck
  swiftc -o /tmp/vscheck DevLogicCheck.swift \
    ../VoxSign/Core/Models.swift ../VoxSign/Core/VSLogic.swift ../VoxSign/Core/SSEParser.swift
  /tmp/vscheck
  # => 结果: 61 通过, 0 失败
  ```

  覆盖：终态/决策点、回执四行（全/半角冒号、缺行）、撤销裁决（可逆/.bak/VHS_BACKUP_PATH/不可逆）、
  轻标签压缩、一屏决策点路由、角色映射、打断状态机、request_id、SSE 分帧/半截帧/seq/打断三语义/重连 URL。

---

## 四、真机运行指引

1. **打开工程**：Xcode → Open Project → 选 `voicesign-harness/ios/VoxSign.xcodeproj`。首次会自动识别 scheme `VoxSign`。
2. **起 server（Mac 上）**：按《手机端真机测试指引-M3.md》
   ```bash
   VHS_ADDR=0.0.0.0:8765 VHS_TOKEN=你的token ./vhs serve
   ```
3. **填设置**：App 右上角 ⚙ → Server 地址填 `http://<Mac内网IP>:8765`、Bearer Token 填同款 → 「测试连接」应回 `OK · v…`。
   - 真机与 Mac 须同 Wi-Fi；AP 隔离就开手机热点给 Mac 连。
4. **签名 + 跑真机**：Xcode 选你的 iPhone 为 destination → 选 Team（Signing & Capabilities）→ ⌘R。
   首次装后：设置 → 通用 → VPN与设备管理 信任开发者证书。
5. **语音权限**：首次点麦克风会弹「麦克风 / 语音识别」授权；拒绝或在模拟器 → 自动回退键盘输入（输入框可直接打字发送）。
6. **试一遍九元素**：
   - 输入「记一下明天给客户发报价」→ 用户右蓝气泡 → 三点 → 执行卡滚动 → 绿色回执卡（可逆时出「撤销」）。
   - 注册域 COMMIT 类任务 → 红色强确认条「执行 / 拒绝」。
   - 说「那个文件」类歧义 → 候选按钮点选续跑。
   - 执行中说「停」→ 红色系统条（已生效/未执行/撤销·继续）。

---

## 五、伪代码逻辑层位置（判断/裁决/状态机）

- `Core/VSLogic.swift`：`extractUndo`（撤销按钮裁决）、`compressBadges`（徽章压缩）、
  `nextDecisionPoint`（一屏决策点优先级）、`roleForStatus`（阶段→角色）、`interruptSystemBar`（打断三语义）。
- `Core/SSEParser.swift`：`SSEParser.feed`（SSE 状态机 / lastSeq / 半截帧 / 重连 after）。
- `State/AppModel.swift`：`send`（打断判定）、`submit/openSSE/handle(event:)`（提交→SSE→决策点流转）、
  `maybeInterrupt`（停止→已生效/未执行/可撤销）。

纯视图/样式（`Views/*.swift`）豁免伪代码注释。

---

## 六、与 web 壳的差异（M6 升级点）

1. **SSE 为主**：M5 web 用轮询 `GET /v1/tasks/{id}`；M6 iOS 改用 `GET /v1/tasks/{id}/events` 事件流驱动执行卡与决策点，
   轮询端点保留作断线兜底。
2. **stage 事件真驱动**：执行卡阶段行由 SSE `stage.step` 点亮（web 是前端模拟推进）。
3. **cancel 用新端点**：优先 `POST /v1/tasks/{id}/cancel`，404/405 回退 legacy `/v1/cancel`。
4. **request_id**：客户端 UUID（web 是时间戳+随机串），语义同为幂等去重。
