//
//  InputBarView.swift
//  VoiceSign
//
//  豆包式底部输入条（T3，99% 复刻豆包交互；UI v3 对齐设计稿）：
//  - 默认"按住说话"：中间「按住 说话」胶囊 + 右侧大圆钮同为热区
//  - 按住 → 立即变红 + 顶部浅色语音状态条（5 态：正在听 / 没听到声音 / 取消 / 校准 / 失败）
//  - 上滑超过 -80pt → 取消态（可滑回继续录音）；松手 → 自动校准 → 自动发送
//  - 左侧键盘图标切换打字模式：TextField + 发送
//  - 录音时 TTS 暂停；提交后输入区立即清空（无残留）
//

import SwiftUI

struct InputBarView: View {
    @EnvironmentObject var model: AppModel
    #if canImport(Speech)
    @EnvironmentObject var speech: SpeechRecognizer
    #endif

    @State private var keyboardMode: Bool = false
    // v2.1 I14：3s 无识别提示（不显示逐字期间的补偿反馈）
    @State private var transcriptSnap: String = ""
    @State private var silentSeconds: Int = 0
    @State private var noSpeechDetected: Bool = false
    // UI v3：上滑取消态（-80pt 阈值，可滑回继续录音——豆包同款手感）
    @State private var cancelling: Bool = false

    var body: some View {
        VStack(spacing: 0) {
            #if canImport(Speech)
            // UI v3 豆包式浅色语音状态条（5 态，按住/松手期间显示在输入区上方）。
            if !keyboardMode {
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
                    Text("语音不可用：请在 系统设置→VoiceSign 中允许 麦克风 与 语音识别")
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 12).padding(.vertical, 8)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(Color.gray.opacity(0.12))
                        .cornerRadius(10)
                        .padding(.horizontal, 12).padding(.top, 6)
                }
            }
            #endif

            HStack(spacing: 12) {
                // 键盘/语音切换（豆包式：左侧小图标切换两种输入）
                Button {
                    #if canImport(Speech)
                    if speech.isRecording { speech.cancelHold() }
                    cancelling = false
                    #endif
                    withAnimation { keyboardMode.toggle() }
                } label: {
                    Image(systemName: keyboardMode ? "mic.fill" : "keyboard")
                        .font(.system(size: 17))
                        .foregroundColor(.secondary)
                        .frame(width: 30, height: 30)
                }
                .accessibilityIdentifier("vhs.mode")

                if keyboardMode {
                    // 打字模式（豆包式：输入框 + 蓝色发送）
                    TextField("说点什么，或打字…", text: $model.inputText)
                        .textFieldStyle(.roundedBorder)
                        .accessibilityIdentifier("vhs.input")
                        .disabled(model.decision != nil)
                    Button {
                        model.send()
                    } label: {
                        Image(systemName: "arrow.up.circle.fill")
                            .font(.system(size: 30))
                            .foregroundColor(model.decision != nil || model.inputText.isEmpty ? .gray : VSColor.blue)
                    }
                    .accessibilityIdentifier("vhs.send")
                    .disabled(model.decision != nil || model.inputText.isEmpty)
                } else {
                    // UI v3：中间「按住 说话」胶囊（也是按住说话热区，扩大可用面积，豆包同款）
                    holdToTalkCapsule
                    // 按住说话大圆钮（豆包式主交互）
                    holdToTalkButton
                }
            }
            .padding(.horizontal, 16).padding(.vertical, 10)
        }
        .background(.ultraThinMaterial)
        // UI v3：输入条顶部 0.5pt 极淡描边（豆包式）。
        .overlay(alignment: .top) {
            Rectangle()
                .fill(Color.black.opacity(0.08))
                .frame(height: 0.5)
        }
    }

    // MARK: - UI v3 豆包式浅色语音状态条（5 态）

    private enum VoiceStatusKind {
        case listening, silent, cancelling, calibrating, failed
    }

    /// 浅底深字豆包式状态条（#FFECEB 红 / #FFF4E5 橙 / #EDEDF0 灰 / #EAF0FF 蓝）。
    @ViewBuilder
    private func voiceStatusBar(kind: VoiceStatusKind, primary: String, secondary: String) -> some View {
        let (bg, fg) = statusColors(kind)
        HStack(spacing: 8) {
            if kind == .listening {
                WaveView().colorScheme(.light)
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
            // 红底深红字（豆包录音态）
            return (Color(red: 1.0, green: 0.925, blue: 0.922), Color(red: 0.776, green: 0.184, blue: 0.149))
        case .silent, .failed:
            // 橙底深橙字
            return (Color(red: 1.0, green: 0.957, blue: 0.898), Color(red: 0.702, green: 0.416, blue: 0.0))
        case .cancelling:
            // 灰底深灰字
            return (Color(red: 0.929, green: 0.929, blue: 0.941), Color(red: 0.333, green: 0.333, blue: 0.361))
        case .calibrating:
            // 蓝底深蓝字
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

    // MARK: - 按住说话（豆包同款手势：按下录音 / 上滑取消可滑回 / 松手发送）

    #if canImport(Speech)
    /// 中间「按住 说话」胶囊：38pt 高浅灰底，按压变红；与右侧大圆钮共用同一手势逻辑。
    private var holdToTalkCapsule: some View {
        Capsule()
            .fill(capsuleFill)
            .frame(height: 38)
            .overlay(
                Text(speech.isRecording ? (cancelling ? "松开 取消" : "松开 发送") : "按住 说话")
                    .font(.system(size: 14, weight: .medium))
                    .foregroundColor(capsuleTextColor)
            )
            .contentShape(Capsule())
            .gesture(holdGesture)
            .animation(.easeOut(duration: 0.12), value: speech.isRecording)
            .animation(.easeOut(duration: 0.12), value: cancelling)
            .accessibilityIdentifier("vhs.hold.capsule")
    }

    private var capsuleFill: Color {
        if speech.isRecording {
            return cancelling ? Color(red: 0.776, green: 0.184, blue: 0.149).opacity(0.9) : Color.red
        }
        return Color.black.opacity(0.06)
    }

    private var capsuleTextColor: Color {
        if speech.isRecording {
            return .white
        }
        return Color(red: 0.557, green: 0.557, blue: 0.576)
    }

    /// 右侧大圆钮：46pt 蓝紫渐变，按下变红呼吸（豆包式主交互）。
    private var holdToTalkButton: some View {
        ZStack {
            // 录音时的呼吸光圈（豆包式：按下有明确视觉反馈）
            if speech.isRecording {
                Circle()
                    .fill(Color.red.opacity(0.15))
                    .frame(width: 76, height: 76)
                    .transition(.scale)
            }
            Image(systemName: speech.isRecording ? "waveform" : "mic.fill")
                .font(.system(size: 26, weight: .medium))
                .foregroundColor(.white)
                .frame(width: 64, height: 64)
                .background(speech.isRecording
                            ? AnyShapeStyle(LinearGradient(colors: [Color(red: 1.0, green: 0.24, blue: 0.24), Color(red: 0.85, green: 0.15, blue: 0.30)], startPoint: .top, endPoint: .bottom))
                            : AnyShapeStyle(VSColor.brandGradient))
                .clipShape(Circle())
                .shadow(color: speech.isRecording ? Color.red.opacity(0.35) : VSColor.purple.opacity(0.3), radius: 8, x: 0, y: 3)
                .contentShape(Circle())
                .scaleEffect(speech.isRecording ? 1.05 : 1.0)
                .animation(.easeOut(duration: 0.12), value: speech.isRecording)
                .accessibilityIdentifier("vhs.hold")
                .gesture(holdGesture)
        }
        .frame(height: 64)
    }

    /// UI v3：胶囊与大圆钮共用手势——按下录音；dy < -80pt 进取消态（可滑回继续）；松手按态发送。
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
    #endif
}
