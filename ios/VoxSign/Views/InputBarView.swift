//
//  InputBarView.swift
//  V4 方案B（用户最终确认稿 2026-10-05）：豆包式底部输入条
//  - 默认态（语音模式）：[圆形＋] | [🎤 按住说话 深色胶囊 flex:1, 56pt] | [圆形⌨]
//  - 点 ⌨ → 文字模式：输入框 + 发送箭头（有文本时出现）；再点 ⌨ 切回语音
//  - 按住大按钮 → 满底波形界面：深色 #1a1a1a 矩形（无弧线/无圆角、向下延展贴屏幕底沿）
//    ① 顶部「正在听…」② 中部 20 根 #ff3b30 竖形声波条（随 meterLevel 起伏+相位呼吸）
//    ③ 底部加粗「松手发送 · 上移取消」
//  - 上滑 -80pt → 取消态（底部换「松开 取消」，可滑回继续录音）；松手发送
//  - 录音期间输入条淡出但保留在层级中（mic 手势宿主不移除/不禁用 hit-testing）
//  - 输入区上方状态条仅保留 校准中 / 空录音重说 / 识别失败 / 语音不可用 四态
//  - 【卡死修复保护项（提交 5f54837）不得回退】holdGesture 双宿主 / holdInitiated 防重入 /
//    极轻点竞态兜底 / isRecording 置 true 无按压看门狗
//

import SwiftUI

struct InputBarView: View {
    @EnvironmentObject var model: AppModel
    #if canImport(Speech)
    @EnvironmentObject var speech: SpeechRecognizer
    #endif

    // 附件面板（＋ → Sheet）
    @State private var showAttachPanel: Bool = false

    // V4 方案B：文字输入模式（默认 false=语音大按钮模式；点 ⌨ 切换）
    @State private var useTextInput: Bool = false

    // v2.1 I14：3s 无识别提示（不显示逐字期间的补偿反馈）
    @State private var transcriptSnap: String = ""
    @State private var silentSeconds: Int = 0
    @State private var noSpeechDetected: Bool = false
    // UI v3：上滑取消态（-80pt 阈值，可滑回继续录音——豆包同款手感）
    @State private var cancelling: Bool = false
    // 【加固1】本轮按压是否由本视图手势发起：快速连按/双宿主重复按压时据此挡住二次 start，
    // 堵住 SpeechRecognizer 异步重入窗口被 start() 内 defer 架空的竞态（R2）。
    @State private var holdInitiated: Bool = false

    private var isTyping: Bool { !model.inputText.isEmpty }

    var body: some View {
        VStack(spacing: 0) {
            #if canImport(Speech)
            // 语音状态条（校准中/空录音/识别失败/不可用 四态，按住/松手期间显示在输入区上方）。
            voiceStatusSection
            #endif

            inputRow
        }
        .background(.ultraThinMaterial)
        // 输入条顶部 0.5pt 极淡描边（豆包式）。
        .overlay(alignment: .top) {
            Rectangle()
                .fill(Color.black.opacity(0.08))
                .frame(height: 0.5)
        }
        .sheet(isPresented: $showAttachPanel) {
            AttachmentPanelView()
                .environmentObject(model)
        }
    }

    // MARK: - 输入行（方案B：语音大按钮 / 文字模式 / 录音态满底波形）

    private var inputRow: some View {
        // ZStack 叠层：录音态时深色波形面板盖在整行之上；普通输入行不随录音移除——
        // 【卡死修复】大按钮 voiceMainButton 必须留在层级中（opacity 0 淡出）且不加
        // allowsHitTesting(false)：已开始的 DragGesture 依赖该视图存活持续收事件，
        // onEnded 必然触发 → stopHold/cancelHold 执行 → isRecording 复位，绝不冻结。
        ZStack {
            normalInputRow
                .opacity(speechRecording ? 0.0 : 1.0)

            if speechRecording {
                recordingHoldView
                    .transition(.opacity)
            }
        }
        #if canImport(Speech)
        // 【加固3-看门狗】isRecording 变 true 但本轮无手指按压（如首次授权弹窗后 start() 补启动，
        // 或任何竞态残留）→ 立即 cancelHold 复位，绝不冻结。正常按压时 holdInitiated 在
        // onChanged 同步置位、先于异步置位 → 不会误取消；cancelHold 内 isRecording=false 不循环触发。
        .onChange(of: speech.isRecording) { recording in
            if recording && !holdInitiated { speech.cancelHold() }
        }
        #endif
        .animation(.easeOut(duration: 0.12), value: speechRecording)
    }

    /// 默认输入行（语音大按钮模式 / 文字模式）。录音时整行 opacity 0 但仍在层级中。
    private var normalInputRow: some View {
        HStack(spacing: 10) {
            addAttachButton
                .allowsHitTesting(!speechRecording)
            if useTextInput {
                textField
                    .allowsHitTesting(!speechRecording)
                if isTyping {
                    sendButton
                        .allowsHitTesting(!speechRecording)
                } else {
                    keyboardToggleButton
                        .allowsHitTesting(!speechRecording)
                }
            } else {
                // 语音大按钮：永不禁用 hit-testing（手势宿主，录音中透明命中维持拖拽连续）
                voiceMainButton
                keyboardToggleButton
                    .allowsHitTesting(!speechRecording)
            }
        }
        .padding(.horizontal, 12).padding(.vertical, 10)
    }

    /// 左：圆形 ＋ 按钮（30×30，浅灰圆底，加号 16pt secondary），弹出附件面板。
    private var addAttachButton: some View {
        Button {
            showAttachPanel = true
        } label: {
            Image(systemName: "plus")
                .font(.system(size: 16, weight: .medium))
                .foregroundColor(.secondary)
                .frame(width: 30, height: 30)
                .background(Circle().fill(Color.black.opacity(0.05)))
        }
        .accessibilityIdentifier("vhs.attach")
    }

    /// 输入框（文字模式）：占位符「发消息或语音指令…」。
    private var textField: some View {
        TextField("发消息或语音指令…", text: $model.inputText, axis: .vertical)
            .lineLimit(...4)
            .font(.system(size: 15))
            .padding(.horizontal, 12).padding(.vertical, 9)
            .background(Color(.secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 18))
            .accessibilityIdentifier("vhs.input")
    }

    /// 中：超大「🎤 按住说话」深色胶囊主按钮（#1a1a1a、高 56pt、胶囊圆角=高度一半、占满剩余宽度）。
    /// 【卡死修复-手势宿主】录音中淡出但不禁用 hit-testing——拖拽起始于此、录音期间持续收事件。
    private var voiceMainButton: some View {
        Text("🎤 按住说话")
            .font(.system(size: 17, weight: .semibold))
            .foregroundColor(.white)
            .frame(maxWidth: .infinity)
            .frame(height: 56)
            .background(Color(red: 0.102, green: 0.102, blue: 0.102)) // #1a1a1a
            .clipShape(Capsule())
            .contentShape(Capsule())
            .accessibilityIdentifier("vhs.mic")
            .gesture(holdGesture)
    }

    /// 右：圆形 ⌨ 按钮（30×30，浅灰圆底，键盘图标 16pt secondary），切换文字输入模式。
    private var keyboardToggleButton: some View {
        Button {
            useTextInput.toggle()
        } label: {
            Image(systemName: "keyboard")
                .font(.system(size: 16, weight: .medium))
                .foregroundColor(.secondary)
                .frame(width: 30, height: 30)
                .background(Circle().fill(Color.black.opacity(0.05)))
        }
        .accessibilityIdentifier("vhs.keyboard")
    }

    /// 发送箭头（文字模式有文本时）。
    private var sendButton: some View {
        Button {
            model.send()
        } label: {
            Image(systemName: "arrow.up.circle.fill")
                .font(.system(size: 30))
                .foregroundColor(model.decision != nil ? .gray : VSColor.blue)
        }
        .accessibilityIdentifier("vhs.send")
        .disabled(model.decision != nil)
    }

    // MARK: - 按住态：满底波形界面（方案B 用户确认稿）

    /// 几何（硬性）：矩形填满宽度、四角无圆角无弧线；背景 .ignoresSafeArea(edges:.bottom)
    /// 向下延展贴屏幕底沿（沉到 home indicator 之下）；总高 160pt；条组高 70pt；
    /// 条组下方留 ~22pt 再放底部文案；底色 #1a1a1a，红条/红字突出。
    /// 结构：① 顶部小字「正在听…」② 中部 20 根 #ff3b30 竖形声波条 ③ 底部加粗「松手发送 · 上移取消」。
    /// 【卡死修复-双宿主】整块波形区域即录音态手势宿主（.gesture(holdGesture)）：
    /// 松手/上滑/滑回落在任意处都触发 onEnded（cancelling→cancelHold 否则 stopHold），isRecording 必然复位。
    private var recordingHoldView: some View {
        VStack(spacing: 0) {
            Text(holdTopText)
                .font(.system(size: 13))
                .foregroundColor(Color.white.opacity(0.75))
                .padding(.top, 10)

            #if canImport(Speech)
            HoldWaveBars(meterLevel: speech.meterLevel)
                .frame(height: 70)
                .padding(.top, 8)
            #else
            HoldWaveBars(meterLevel: 0)
                .frame(height: 70)
                .padding(.top, 8)
            #endif

            Text(holdBarText)
                .font(.system(size: 16, weight: .semibold))
                .foregroundColor(HoldWaveBars.barRed)
                .padding(.top, 22)          // 条组下方留 ~22pt 再放底部文案
                .padding(.bottom, 14)
        }
        .frame(maxWidth: .infinity)
        .frame(height: 160)
        // 深色矩形：无圆角/无弧线，四角都是直角
        .background(HoldWaveBars.panelDark)
        // 向下延展：背景沉到 home indicator 之下，整体贴底不留空隙
        .ignoresSafeArea(edges: .bottom)
        .animation(.easeOut(duration: 0.12), value: cancelling)
        // 【卡死修复-双宿主】手势宿主：录音中重新按压波形区域任意处也可接管手势；
        // -80pt 阈值与可滑回逻辑由 holdGesture.onChanged/onEnded 内部语义保证，此处不变。
        .gesture(holdGesture)
        // noSpeechDetected 计数定时器（原样迁移）。录音中每秒对比 transcript 快照累计静默秒数；非录音清零。
        .onReceive(Timer.publish(every: 1, on: .main, in: .common).autoconnect()) { _ in
            #if canImport(Speech)
            if speech.isRecording {
                if speech.transcript != transcriptSnap {
                    transcriptSnap = speech.transcript
                    silentSeconds = 0
                } else {
                    silentSeconds += 1
                }
                noSpeechDetected = silentSeconds >= 3
            } else {
                noSpeechDetected = false
                silentSeconds = 0
                transcriptSnap = ""
            }
            #endif
        }
    }

    /// 顶部文案：无语音（≥3s）→ “没听到声音，请说话”；否则 → “正在听…”。
    private var holdTopText: String {
        if noSpeechDetected { return "没听到声音，请说话" }
        return "正在听…"
    }

    /// 底部整行文案：取消 → “松开 取消”；否则 → “松手发送 · 上移取消”。
    private var holdBarText: String {
        cancelling ? "松开 取消" : "松手发送 · 上移取消"
    }

    // MARK: - 按住态竖形声波条组（方案B 新建于本文件内，不动 MessageViews 的 WaveView）

    /// 20 根细竖条（宽 5pt、圆角 2pt、#ff3b30），参差基准高度形成自然声波轮廓。
    /// TimelineView(.animation) 每帧重算：
    ///   条高[i] = 基准[i] × (0.35 + 0.65 × 当帧系数[i])
    ///   当帧系数[i] = meterLevel(0~1)×0.7 + 每根独立相位呼吸 sin(t×4 + i×0.55)×0.3
    /// 低电平时系数 ≈ 0~0.3 → 条高保持基准的 35%~55% 轻微呼吸（不至于静止）；
    /// 说话时 meterLevel→1 → 条高冲到接近基准（上限对齐条组 70pt）。
    private struct HoldWaveBars: View {
        var meterLevel: Float

        /// #ff3b30 波形红（条与底部文案共用）。
        static let barRed = Color(red: 0.996, green: 0.231, blue: 0.188)
        /// #1a1a1a 深色面板底（与主胶囊一致）。
        static let panelDark = Color(red: 0.102, green: 0.102, blue: 0.102)

        /// 20 个参差基准高度（pt）：中间高两侧低的自然声波轮廓，峰值 64pt 对齐条组 70pt。
        private static let baselines: [CGFloat] = [
            14, 26, 40, 54, 36, 58, 44, 64, 30, 48,
            48, 30, 64, 44, 58, 36, 54, 40, 26, 14
        ]

        var body: some View {
            TimelineView(.animation) { timeline in
                let t = timeline.date.timeIntervalSinceReferenceDate
                let lvl = Double(meterLevel)
                HStack(alignment: .center, spacing: 3) {
                    ForEach(Array(Self.baselines.enumerated()), id: \.offset) { i, base in
                        // 每根条独立相位：sin(t×速率 + i×相位差) → [0,1] 呼吸值
                        let breath = 0.5 + 0.5 * sin(t * 4.0 + Double(i) * 0.55)
                        let coeff = min(1.0, lvl * 0.7 + breath * 0.3)
                        let h = base * (0.35 + 0.65 * coeff)
                        RoundedRectangle(cornerRadius: 2)
                            .fill(Self.barRed)
                            .frame(width: 5, height: h)
                    }
                }
            }
        }
    }

    // MARK: - 语音状态（4 态）段

    @ViewBuilder
    private var voiceStatusSection: some View {
        if speech.calibrating {
            voiceStatusBar(kind: .calibrating, primary: "正在校准…", secondary: "识别完成后自动发送")
        } else if speech.emptyRecording {
            voiceStatusBar(kind: .silent, primary: "没录到声音，请重说", secondary: "录音是空的，这次没有发送")
        } else if speech.asrFailed {
            voiceStatusBar(kind: .failed, primary: "识别失败，请再按一次", secondary: "没有听清，这次没有发送")
        } else if speech.unavailable {
            Text("语音不可用：请在 系统设置→VoxSign 中允许 麦克风 与 语音识别")
                .font(.system(size: 12))
                .foregroundColor(.secondary)
                .padding(.horizontal, 12).padding(.vertical, 8)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Color.gray.opacity(0.12))
                .cornerRadius(10)
                .padding(.horizontal, 12).padding(.top, 6)
        }
    }

    private var speechRecording: Bool {
        #if canImport(Speech)
        return speech.isRecording
        #else
        return false
        #endif
    }

    // MARK: - 豆包式浅色语音状态条

    private enum VoiceStatusKind {
        case listening, silent, cancelling, calibrating, failed
    }

    /// 浅底深字豆包式状态条（#FFECEB 红 / #FFF4E5 橙 / #EDEDF0 灰 / #EAF0FF 蓝）。
    @ViewBuilder
    private func voiceStatusBar(kind: VoiceStatusKind, primary: String, secondary: String) -> some View {
        let (bg, fg) = statusColors(kind)
        HStack(spacing: 8) {
            if kind == .listening {
                WaveView(meterLevel: speech.meterLevel).colorScheme(.light)
            } else {
                Image(systemName: icon(kind))
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(fg)
            }
            VStack(alignment: .leading, spacing: 1) {
                Text(primary)
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(fg)
                Text(secondary)
                    .font(.system(size: 11, weight: .medium))
                    .foregroundColor(fg.opacity(0.85))
            }
            Spacer()
        }
        .padding(.horizontal, 12).padding(.vertical, 8)
        .background(bg)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(fg.opacity(0.18), lineWidth: 0.5)
        )
        .cornerRadius(12)
        .padding(.horizontal, 12).padding(.top, 6)
        .transition(.move(edge: .top).combined(with: .opacity))
        .onReceive(Timer.publish(every: 1, on: .main, in: .common).autoconnect()) { _ in
            guard kind == .listening || kind == .silent else { return }
            #if canImport(Speech)
            if speech.isRecording {
                if speech.transcript != transcriptSnap {
                    transcriptSnap = speech.transcript
                    silentSeconds = 0
                } else {
                    silentSeconds += 1
                }
                noSpeechDetected = silentSeconds >= 3
            } else {
                noSpeechDetected = false
                silentSeconds = 0
                transcriptSnap = ""
            }
            #endif
        }
    }

    private func statusColors(_ kind: VoiceStatusKind) -> (Color, Color) {
        switch kind {
        case .listening:
            return (Color(red: 1.0, green: 0.925, blue: 0.922), Color(red: 0.776, green: 0.184, blue: 0.149))
        case .silent, .failed:
            return (Color(red: 1.0, green: 0.957, blue: 0.898), Color(red: 0.702, green: 0.416, blue: 0.0))
        case .cancelling:
            return (Color(red: 0.929, green: 0.929, blue: 0.941), Color(red: 0.333, green: 0.333, blue: 0.361))
        case .calibrating:
            return (Color(red: 0.918, green: 0.941, blue: 1.0), Color(red: 0.137, green: 0.333, blue: 0.78))
        }
    }

    private func icon(_ kind: VoiceStatusKind) -> String {
        switch kind {
        case .silent: return "speaker.slash.fill"
        case .cancelling: return "xmark.circle.fill"
        case .calibrating: return "waveform"
        case .failed: return "exclamationmark.triangle.fill"
        case .listening: return "waveform"
        }
    }

    // MARK: - 按住说话手势（按下录音 / 上滑取消可滑回 / 松手发送）——语义原样迁移

    #if canImport(Speech)
    /// 麦克风按钮手势——按下录音；dy < -80pt 进取消态（可滑回继续）；松手按态发送。
    private var holdGesture: some Gesture {
        DragGesture(minimumDistance: 0)
            .onChanged { v in
                // 【加固1】仅在本轮尚无发起者时 start 一次；录音中或已发起过不重复 start（R2）。
                if !speech.isRecording && !holdInitiated {
                    holdInitiated = true
                    cancelling = false
                    speech.startHold()
                }
                // 上滑 -80pt → 取消态；滑回 -80pt 以上 → 恢复录音（豆包同款可逆手感）。
                let c = v.translation.height < -80
                if c != cancelling { cancelling = c }
            }
            .onEnded { v in
                // 【加固1/2】先记下本轮是否发起者，再复位标记。
                let wasInitiator = holdInitiated
                holdInitiated = false
                if speech.isRecording {
                    if cancelling || v.translation.height < -80 {
                        speech.cancelHold()   // 上滑取消：不发送
                    } else {
                        speech.stopHold()     // 松手：识别完自动发送
                    }
                } else if wasInitiator {
                    // 【加固2-R1 极轻点竞态兜底】isRecording 是 start() 里 DispatchQueue.main.async
                    // 下一拍才置 true；手指先松时按旧逻辑什么都不做 → 随后置位 → 无手指却卡录音态。
                    // 主队列 FIFO 保证此块排在置位块之后执行：若已置位则立即收尾（此时 installTap
                    // 已同步完成，stopHold 安全）。
                    DispatchQueue.main.async {
                        if self.speech.isRecording { self.speech.stopHold() }
                    }
                }
                cancelling = false
            }
    }
    #else
    private var holdGesture: some Gesture {
        DragGesture(minimumDistance: 0)
    }
    #endif
}
