//
//  InputBarView.swift
//  VoxSign
//
//  豆包式底部输入条（聊天面视图 T3 改造）：
//  - 三件套：[＋ 添加资料] | [输入框] | [🎤 语音 / ⬆ 发送]
//  - 输入框常显（打字即用，无键盘/语音切换）；去掉云电脑/项目/技能等默认项入口
//  - 右侧按钮双态：空文本=麦克风（按住说话热区）；有文本=蓝色发送箭头
//  - 按住说话 → 输入条整体变为红色「松开 发送」条（白字红底，豆包同款）；
//    上滑 -80pt →「松开 取消」（可滑回继续录音）；松手发送
//  - 5 态语音状态条（正在听 / 没听到声音 / 取消 / 校准 / 失败）保留在输入区上方
//  - AI 消息操作行不朗读/喇叭按钮（收进 MessageViews 的「…」菜单）
//

import SwiftUI

struct InputBarView: View {
    @EnvironmentObject var model: AppModel
    #if canImport(Speech)
    @EnvironmentObject var speech: SpeechRecognizer
    #endif

    // 附件面板（＋ 添加资料 → Sheet）
    @State private var showAttachPanel: Bool = false

    // v2.1 I14：3s 无识别提示（不显示逐字期间的补偿反馈）
    @State private var transcriptSnap: String = ""
    @State private var silentSeconds: Int = 0
    @State private var noSpeechDetected: Bool = false
    // UI v3：上滑取消态（-80pt 阈值，可滑回继续录音——豆包同款手感）
    @State private var cancelling: Bool = false

    private var isTyping: Bool { !model.inputText.isEmpty }

    var body: some View {
        VStack(spacing: 0) {
            #if canImport(Speech)
            // UI v3 豆包式浅色语音状态条（5 态，按住/松手期间显示在输入区上方）。
            voiceStatusSection
            #endif

            inputRow
        }
        .background(.ultraThinMaterial)
        // UI v3：输入条顶部 0.5pt 极淡描边（豆包式）。
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

    // MARK: - 输入行（三件套 / 录音态红色条）

    private var inputRow: some View {
        HStack(spacing: 10) {
            // 录音中：整行被红色「松开 发送」条覆盖（手势宿主仍为下方按钮，保证拖拽连续）。
            if speechRecording {
                redHoldBar
                    .transition(.opacity.combined(with: .scale(scale: 0.98)))
            }

            // 录音态下三件套淡出但保留在层级中（右侧按钮=手势宿主，按住拖拽不中断）。
            HStack(spacing: 10) {
                addAttachButton
                textField
                rightButton
            }
            .opacity(speechRecording ? 0.0 : 1.0)
            .allowsHitTesting(!speechRecording)
        }
        .padding(.horizontal, 12).padding(.vertical, 10)
        .animation(.easeOut(duration: 0.12), value: speechRecording)
    }

    /// ＋ 添加资料（紧凑豆包式），弹出附件面板。
    private var addAttachButton: some View {
        Button {
            showAttachPanel = true
        } label: {
            HStack(spacing: 3) {
                Image(systemName: "plus.circle")
                    .font(.system(size: 18))
                Text("添加资料")
                    .font(.system(size: 13, weight: .medium))
            }
            .foregroundColor(.secondary)
        }
        .accessibilityIdentifier("vhs.attach")
    }

    /// 输入框：常显，占位符「发消息或语音指令…」。
    private var textField: some View {
        TextField("发消息或语音指令…", text: $model.inputText, axis: .vertical)
            .lineLimit(...4)
            .font(.system(size: 15))
            .padding(.horizontal, 12).padding(.vertical, 8)
            .background(Color(.secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 18))
            .accessibilityIdentifier("vhs.input")
    }

    /// 右侧双态按钮：空文本=麦克风（按住说话热区）；有文本=蓝色发送箭头。
    @ViewBuilder
    private var rightButton: some View {
        if isTyping {
            Button {
                model.send()
            } label: {
                Image(systemName: "arrow.up.circle.fill")
                    .font(.system(size: 30))
                    .foregroundColor(model.decision != nil ? .gray : VSColor.blue)
            }
            .accessibilityIdentifier("vhs.send")
            .disabled(model.decision != nil)
        } else {
            // 麦克风：按住说话热区（手势宿主，录音期间保持在层级中以维持拖拽连续）。
            Image(systemName: "mic.fill")
                .font(.system(size: 20))
                .foregroundColor(.secondary)
                .frame(width: 40, height: 40)
                .contentShape(Circle())
                .accessibilityIdentifier("vhs.mic")
                .gesture(holdGesture)
        }
    }

    /// 豆包同款红色「松开 发送」条：白字红底，整行覆盖；上滑取消时变灰「松开 取消」。
    private var redHoldBar: some View {
        HStack {
            Spacer()
            Text(cancelling ? "松开 取消" : "松开 发送")
                .font(.system(size: 15, weight: .semibold))
                .foregroundColor(.white)
            Spacer()
        }
        .frame(height: 40)
        .frame(maxWidth: .infinity)
        .background(cancelling
                    ? Color(red: 0.55, green: 0.56, blue: 0.58)
                    : Color(red: 0.96, green: 0.26, blue: 0.26))
        .clipShape(RoundedRectangle(cornerRadius: 20))
        .shadow(color: (cancelling ? Color.gray : Color.red).opacity(0.3), radius: 6, x: 0, y: 2)
        .animation(.easeOut(duration: 0.12), value: cancelling)
    }

    // MARK: - 语音状态（5 态）段

    @ViewBuilder
    private var voiceStatusSection: some View {
        if speech.calibrating {
            voiceStatusBar(kind: .calibrating, primary: "正在校准…", secondary: "识别完成后自动发送")
        } else if speech.emptyRecording {
            voiceStatusBar(kind: .silent, primary: "没录到声音，请重说", secondary: "录音是空的，这次没有发送")
        } else if speech.asrFailed {
            voiceStatusBar(kind: .failed, primary: "识别失败，请再按一次", secondary: "没有听清，这次没有发送")
        } else if speech.isRecording {
            if cancelling {
                voiceStatusBar(kind: .cancelling, primary: "松开手指，取消发送", secondary: "手指移回下方可继续录音")
            } else if noSpeechDetected {
                voiceStatusBar(kind: .silent, primary: "没听到声音，请说话", secondary: "再靠近一点，或松开手指取消")
            } else {
                voiceStatusBar(kind: .listening, primary: "正在听…", secondary: "松开 发送 · 上滑取消")
            }
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

    // MARK: - 豆包式浅色语音状态条（5 态）

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

    // MARK: - 按住说话手势（按下录音 / 上滑取消可滑回 / 松手发送）

    #if canImport(Speech)
    /// 麦克风按钮手势——按下录音；dy < -80pt 进取消态（可滑回继续）；松手按态发送。
    private var holdGesture: some Gesture {
        DragGesture(minimumDistance: 0)
            .onChanged { v in
                if !speech.isRecording {
                    cancelling = false
                    speech.startHold()
                }
                // 上滑 -80pt → 取消态；滑回 -80pt 以上 → 恢复录音（豆包同款可逆手感）。
                let c = v.translation.height < -80
                if c != cancelling { cancelling = c }
            }
            .onEnded { v in
                if speech.isRecording {
                    if cancelling || v.translation.height < -80 {
                        speech.cancelHold()   // 上滑取消：不发送
                    } else {
                        speech.stopHold()     // 松手：识别完自动发送
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
