# VoxSign iOS V4 · 方案 B 最终设计规格（用户已确认 2026-10-06）

本文件是 V4 实现的唯一设计依据。用户已对方案 B 修正稿回复「ok」。

## 0. 背景与基准

- 工程：/Users/zouyongming/vhs-ui-v3/ios，分支 ui-v3-doubao，VoxSign.xcodeproj / scheme VoxSign
- 按住态视觉基准（用户 iOS 实拍）：`docs/v4-ref-doubao-holdstate.png`
- 聊天参考图：`docs/v4-ref-doubao-chat.png`；改造前现状：`docs/v4-cur-state.png`
- 版本：Info.plist 4.0 (build 10)；bundle id = ai.voxsign.ios

## 1. 必须保留（不可回退）

1. **按住说话卡死修复**（提交 `5f54837`，逐字节不得改动）：
   - holdGesture 双宿主（红条/波形区域本身挂手势，麦克风宿主不再被 allowsHitTesting(false) 禁用）
   - holdInitiated 防重入
   - 极轻点竞态兜底（onEnded 后下一拍补 stopHold）
   - isRecording 置 true 但无按压时看门狗 cancelHold
   - 涉及：InputBarView.swift + SpeechRecognizer.swift 的调用链
2. 顶栏豆包式：居中主标题 VoxSign + 副标题 VoxSign·metasystem（旧 V4 代理已改，保留）
3. 消息气泡/时间戳豆包式、空态极简、AI 消息行收…菜单（去🔔🔊）、无云电脑/技能按钮
4. ＋添加资料四类（文本/URL/图片/文件），AttachmentPanelView 逻辑保留

## 2. 默认态输入条（聊天页底部，方案 B）

一行三元素，浅灰圆角容器（仿豆包聊天页）：

```
[＋]  [ 🎤 按住说话（黑色大按钮，flex:1） ]  [⌨]
```

- ＋：约 30×30 圆形浅灰底，点击弹添加资料面板（四类）
- **主按钮：flex:1 占满宽度主体，深黑底（#1A1A1A）白字，高约 48pt，圆角约 24pt，居中「🎤 按住说话」（14–15pt 粗体）——按住即录音**
- ⌨：约 30×30 圆形浅灰底键盘图标，点击切文字输入（出键盘+输入框）；再点回语音模式
- 空态文案指向明确：**「按住🎤说话，或点⌨打字」**（不得再出现「说点什么，或按住下方按钮说话」等指向不明文案）

## 3. 按住态（满底波形界面）— 本次核心新设计

按住主按钮后，输入条区域切换为**满底波形界面**（用户已确认，区别于旧版红条/蓝条）：

- **波形**：一组竖条（约 18–20 根，宽 5–6pt，圆角 2–3pt，红色 #FF3B30），高度各异（约 12–64pt），**按住时随语音音量起伏动画（动图感）**——由 SpeechRecognizer meterLevel 驱动（WaveView 已有，接好即可）
- 波形上方小字「正在听…」
- 波形下方居中粗体「**松手发送 · 上移取消**」
- **区域形状：矩形、无弧线、无圆角边框、填满容器宽度；向下延展：波形区域贴近屏幕底部/容器底部，下方留更大空间**（相对豆包实拍更满、更向下）
- 交互保留：松手发送；上滑（约 -80pt）取消

## 4. 验收清单（全部通过才算完成）

1. 构建（命令原样，**不传任何签名覆盖参数**）：
   `xcodebuild -project VoxSign.xcodeproj -scheme VoxSign -configuration Debug -destination 'generic/platform=iOS' -derivedDataPath /tmp/vhs-v4-dd build`
2. 42 单测全绿
3. 模拟器三态实图（干净、无系统弹窗）：默认态 / 按住态（录音态）
   - **UI 测试需命令行环境变量 `TEST_TARGET_NAME=VoxSign`**（否则 Code=108）
   - 通知授权弹窗在 UI 测试内点掉（或预写通知授权状态）
   - 模拟器按住态截图可行：SpeechRecognizer.start() 按下瞬间置 isRecording=true（UI 立即反馈）
4. 真机 ipa V4（ai.voxsign.ios，约 1.18MB），codesign TeamID 6ASMXVQHKK
5. git commit 到 ui-v3-doubao（不 push）；**harness-output/ 在途数据禁碰禁提交**；用户并行流程可能随时提交改动（用 reflog/git log 核对，避免重复提交）

## 5. 交付

- 模拟器实图：默认态、按住态（PNG，路径清晰）
- 真机 ipa 路径 + 校验信息（bid/文案/签名）
- commit 信息（hash + message）
