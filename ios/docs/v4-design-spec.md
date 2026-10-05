# V4 豆包式 UI 对齐规格（唯一基准：ios/docs/v4-ref-doubao-chat.png）

本规格是 V4 版本 UI 改造的唯一权威依据。执行者必须逐项对照实现，不得凭印象发挥。
对照差距图：ios/docs/v4-cur-state.png（改造前）。

## 0. 共享设计令牌

- 发件人标注常量：`enum VSBrand { static let agentLabel = "VoxSign·metasystem" }`
  - **定义权归 MessageViews 执行者**：在 MessageViews.swift 中 VSColor 枚举旁新增。
  - RootView 执行者**只引用** `VSBrand.agentLabel`，不得自行定义同名常量（会编译冲突）。
- 颜色：复用现有 VSColor（blue #3370FF、bg、harnessBubble=white 等），不新增色系。
- 消息文本字号统一 16pt（豆包消息字号）；元信息小字 11pt；发件人标注 11pt。

## 1. 顶栏（RootView.swift）

结构：`HStack`：左按钮 | Spacer | 中央 VStack(标题行 + 副信息小字) | Spacer | 右按钮。

- 左按钮：`bubble.left.and.bubble.right`，15pt medium，black；34×34 触控区；
  背景 `Color.black.opacity(0.05)` + `RoundedRectangle(cornerRadius: 10)`（低调圆角）；
  identifier `vhs.sessions` 保留；动作 model.showSessions = true 不变。
- 中央标题行：`HStack(spacing: 4)`{ ConnectionDotView(现状逻辑与 identifier `vhs.status.dot` **不动**)，
  Text("VoxSign") 17pt semibold black }。
- 中央副信息：`Text(VSBrand.agentLabel)` 11pt `.secondary`（小字，居中）。
- 右按钮：`ellipsis`，15pt medium，black；34×34；同款低调圆角背景；
  identifier `vhs.more` 保留；菜单内容（新建会话/设置）不变。
- 背景 `.ultraThinMaterial`、`.padding(.leading, 6)`、`padding(.trailing, 10)` 保留。

## 2. 空态（RootView.swift）

`model.rows.isEmpty` 时，替换现有一行灰字为居中组合：

- `VStack(spacing: 14)`，`.frame(maxWidth: .infinity)`，`.padding(.top, 120)`：
  - 图标：`ZStack`{ `Circle().fill(VSColor.blue.opacity(0.10)).frame(width: 64, height: 64)`，
    `Image(systemName: "waveform.and.mic").font(.system(size: 26, weight: .medium)).foregroundColor(VSColor.blue)` }
  - 文案：`Text("说点什么，或按住下方按钮说话")` 14pt `Color(red: 0.682, green: 0.682, blue: 0.698)`
- 简洁居中，不加卡片、不加示例。

## 3. 消息区

### 3a. 列表顶部居中时间戳（RootView.swift）

- rows 非空时，LazyVStack 顶部第一条之前插入：
  `HStack`{ Spacer; `Text(首条气泡 timestamp 的 "HH:mm")` 11pt `.secondary`; Spacer }，下方留 4pt 间距。
- 取"首条气泡"：从 rows.first 开始跳过 typing/execCard，取第一条 user/harness 气泡的 timestamp。
- DateFormatter `dateFormat = "HH:mm"`。

### 3b. AI 气泡发件人标注（MessageViews.swift）

- HarnessBubbleView：气泡文本上方加 `Text(VSBrand.agentLabel)` 11pt `.secondary`；
  放在现有 `VStack(alignment: .leading, spacing: 3)` 内文本之前，`padding(.leading, 6).padding(.bottom, 2)`。
- ReceiptCardView：卡片上方加同款发件人小字（左对齐，`padding(.leading, 2).padding(.bottom, 4)`）。
- HarnessBubbleView 与 UserBubbleView 的气泡文本 `Text(bubble.text)` 均加 `.font(.system(size: 16))`。
- 气泡圆角/渐变/阴影保持现状（AI 白底 18/4/18/18、用户蓝紫渐变 18/18/4/18、AI 阴影 shadowSoft）——已对齐豆包。

### 3c. 用户气泡下方元信息（MessageViews.swift）

- UserBubbleView 内、气泡（含附件 chips 之后）下方加一行右对齐小字：
  `HStack`{ Spacer; `Text("HH:mm")`; 若 `bubble.fromVoice && (bubble.voiceSeconds ?? 0) > 0`：
  `Text("· 共用时X分X秒")` }，11pt `.secondary`，`padding(.trailing, 4).padding(.top, 2)`。
- "共用时X分X秒" 格式（纯函数，写在 MessageViews.swift 内）：
  secs >= 60 → `"共用时\(secs / 60)分\(secs % 60)秒"`；否则 `"共用时\(secs)秒"`。

### 3d. AI 气泡元信息（现状保留）

- HarnessBubbleView 的 infoRow（消耗·时间·…菜单）11pt `.secondary` 保留不动——已豆包式。

## 4. 输入条默认态（InputBarView.swift）——不改

`[＋ 添加资料] | [输入框 "发消息或语音指令…" 约36pt 圆角18] | [🎤 麦克风]` 现状已对齐，保持不动。

## 5. 按住态（InputBarView.swift）——重点改造

### 5a. 新增 recordingHoldView 替换 redHoldBar

当 `speechRecording` 时，inputRow 内显示 `recordingHoldView`（原位替换 redHoldBar）：

```
VStack(spacing: 6) {
    HStack(spacing: 8) {
        if !cancelling { WaveView(meterLevel: speech.meterLevel) }
        else { Image(systemName: "xmark.circle.fill").font(.system(size: 13, weight: .semibold)).foregroundColor(.white) }
        Text(holdTopText).font(.system(size: 13, weight: .semibold)).foregroundColor(.white)
        Spacer()
    }
    Text(holdBarText).font(.system(size: 15, weight: .semibold)).foregroundColor(.white).frame(maxWidth: .infinity)
}
.padding(.horizontal, 14).padding(.vertical, 12)
.background(holdBarColor)
.clipShape(RoundedRectangle(cornerRadius: 20))
.shadow(color: holdBarColor.opacity(0.3), radius: 6, x: 0, y: 2)
```

- `holdTopText`：cancelling → `"松开手指，取消发送"`；noSpeechDetected → `"没听到声音，请说话"`；否则 → `"正在听…"`。
- `holdBarText`：cancelling → `"松开 取消"`；否则 → `"松手发送，上移取消"`。
- `holdBarColor`：cancelling → `Color(red: 0.55, green: 0.56, blue: 0.58)`；否则 → `Color(red: 0.96, green: 0.26, blue: 0.26)`。
- 过渡动画：保持现有 `.transition(.opacity.combined(with: .scale(scale: 0.98)))` 与三件套 `.opacity(speechRecording ? 0.0 : 1.0)`。
- 删除旧 redHoldBar（其文本"松开 发送"被新文案取代）。

### 5b. 【卡死修复保护项——绝对不许回退】

以下为上一版已完成并验收的修复，**一字不许改**：

1. `holdGesture` 全文逻辑：holdInitiated 守卫、onChanged 仅一次 startHold、-80pt 上滑取消可滑回、
   onEnded 按 cancelling/translation 走 cancelHold/stopHold、极轻点 DispatchQueue.main.async 兜底。
2. **手势宿主**：`recordingHoldView` 必须 `.gesture(holdGesture)`（整块红容器是录音态手势宿主，
   松手/上滑/滑回落在任意处都触发 onEnded → isRecording 必然复位）。
3. 三件套 HStack 中：`addAttachButton` 与 `textField` 保持 `.allowsHitTesting(!speechRecording)`；
   **rightButton（麦克风）不得加 allowsHitTesting(false)**——录音期间正在进行的拖拽必须持续收事件。
4. `onChange(of: speech.isRecording)` 看门狗（录音但无手指按压 → cancelHold）保留。

### 5c. noSpeechDetected 定时器迁移

- 把 `voiceStatusBar` 内 `onReceive(Timer 1s)` 的计数逻辑（transcriptSnap/silentSeconds/noSpeechDetected，
  guard kind == .listening || .silent 的等价语义）**原样迁移**到 recordingHoldView 上：
  guard 录音中才计数（speech.isRecording true），非录音清零。
- `voiceStatusSection`：删除 isRecording 分支下的 listening / cancelling / noSpeechDetected 三个
  `voiceStatusBar` 调用（已并入 recordingHoldView）；**保留** calibrating / emptyRecording / asrFailed / unavailable 分支。
- VoiceStatusKind 中不再被调用的 case 可保留（不影响编译）或删除，随实现者判断，不得破坏其他分支编译。

## 6. 文字输入态——不改

有字后右侧蓝色 `arrow.up.circle.fill` 30pt（vhs.send）现状已对齐豆包，保持不动。

## 7. 保留要素（用户已拍板，不许删）

- ＋添加资料入口与 AttachmentPanelView（不动）
- 顶栏红/蓝连接状态点（ConnectionDotView / TopBarDot 逻辑与测试不动）
- 多会话默认隐藏列表 + 左上会话入口（不动）
- 消息行无🔔🔊（现状已满足）、无"云电脑/技能"按钮（现状已满足）

## 8. 版本（RootView 执行者负责）

- `VoxSign/App/Info.plist`：`CFBundleShortVersionString` 2.6 → **4.0**；`CFBundleVersion` 9 → **10**。
- 不得改 Info.plist 其他任何键。SettingsView 自动显示 "4.0 (build 10)"，无需改代码。

## 9. 执行纪律（双方共同）

- 只改各自清单内的文件，**不得**动其他文件（AppModel/SessionStore/其他 Views/测试）。
- **不构建、不跑测试、不 git add/commit**（构建与验证由后续执行者统一做；git 操作由主代理收尾）。
- 不引入新第三方依赖；不引入新文件（常量定义在现有文件内完成）。
- 完成后自查：逐项对照本规格 §1-§8 报告"改了什么/对应哪一条"，并确认没有改动保护项（§5b）。
