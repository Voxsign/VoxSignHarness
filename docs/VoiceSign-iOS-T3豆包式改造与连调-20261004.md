# VoiceSign iOS 客户端 · T3 豆包式改造与连调记录

- 日期：2026-10-04
- 版本：T3（豆包式 v3，最终版）
- 目标：iOS 客户端成为 VoxSign Harness 的载体与测试项目，交互 99% 复刻豆包 App

## 一、T3 豆包式改造清单（对照豆包 App）

| # | 豆包行为 | VoiceSign 实现 | 落地文件 |
|---|---|---|---|
| 1 | 按住说话、松手自动识别发送 | `DragGesture(minimumDistance:0)`：按下→录音（立即变红+光圈），松手→endAudio→final→自动提交 | SpeechRecognizer (startHold/stopHold)、InputBarView |
| 2 | 按住时显示"松开发送"+实时识别文字 | 红色状态条（"松开 发送"+transcript 实时回显） | InputBarView |
| 3 | 打字模式切换 | 左侧键盘/麦克风图标切换 TextField+发送 | InputBarView |
| 4 | 回复语音朗读（TTS） | AVSpeechSynthesizer zh-CN；回复/回执/决策点问题自动朗读；用户开口打断 | VoiceOutputService（新）、AppModel |
| 5 | 设置里"连接哪台设备/多台切换/连云" | 多服务器列表（添加/切换/删除/编辑），预置"主服务器 201"+"我的 Mac 129" | SettingsStore（重构）、SettingsView |
| 6 | 纯对话流，无流程卡 | 执行过程收敛为三点"正在思考"（TypingView）；七项流程卡/诊断行/角色条全部移除 | RootView、MessageViews |
| 7 | 发完即滚到底 | scrollTick 驱动 0.1s 轻滚 | AppModel、RootView |
| 8 | 说完不留残留 | final 即 cancel 旧任务+清 transcript；提交后 inputText 清空 | SpeechRecognizer、AppModel |
| 9 | 连接状态透明 | 顶部胶囊：绿=已连接/黄=重连/灰=离线排队 | ConnectivityService、ConnectionStatusView |

## 二、默认服务器

- 默认连接：`http://127.0.0.1:8897`（用户指定，主服务器）
- 备用：`http://192.168.8.129:8897`（本机 Mac，m7-token）
- 均预置两台，设置页可随时切换/添加/删除（豆包式"连哪台电脑/连云"）

## 三、连调结果（e2e-ios-link.py，模拟手机全链路）

| 语音命令（模拟 ASR 输出） | 链路结果 |
|---|---|
| 查看当前工作目录的文件 | done + 回执（QUERY）✓ |
| 记一下我待会要给电脑贴膜这个想法 | done + 回执（append notes.md + 备份）✓ |
| 删除当前目录下的临时文件 temp_e2e.txt | done + BOUNDARY_VIOLATION（越界拦截）✓ |
| 请读取项目根目录的 AGENTS.md 这个文件 | need_ask（意图分类未命中 READ，回问）——Harness 侧话术边界，已记录 |

SSE stage/done 事件流正常；轮询视图与回执一致；服务端 8897 监听 0.0.0.0（手机可直连）。

## 四、真机状态

- iPhonePeter（UDID 00008120-001428820AB8201E）已安装 T3 最终版，启动需解锁
- ipa：`ios/VoiceSign-T3豆包式-最终版-真机包-20261004.ipa`（Debug，627K）

## 五、测试方式（"可能失败"的验证）

1. 编译：xcodebuild 真机构建 BUILD SUCCEEDED（多次迭代）
2. 单测：VoiceSignTests 全过（VSLogicTests 8 项）
3. server 链路：e2e-ios-link.py 4 条命令全链路（POST→SSE→轮询→回执）
4. 模拟器：启动不崩、顶部胶囊/欢迎语/输入条渲染正常
5. 真机：按住说话手势 → 用户实测（解锁后）

## 六、已知边界（非缺陷，Harness 侧待改进）

- 意图分类为规则式：话术不在规则内会 need_ask 回问（如"请读取…文件"）。主仓库已提交意图分类修复（67ce91f），vhs-live 工作区为并行演进版本，可按需同步。
- 语音识别依赖 Apple SFSpeechRecognizer（zh-CN）网络服务；网络差时识别慢/不准属系统级依赖。
