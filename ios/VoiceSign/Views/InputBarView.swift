//
//  InputBarView.swift
//  VoiceSign
//
//  豆包式底部输入条（T3，99% 复刻豆包交互）：
//  - 默认"按住说话"：按住大圆钮 → 立即变红 + 顶部出现"松开发送"红色状态条 + 实时识别文字
//  - 松手 → 自动识别完毕 → 自动发送执行（不需要任何确认键）
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

    var body: some View {
        VStack(spacing: 0) {
            // 豆包式"松开发送"录音状态条：按住时出现。
            // v3 决策依据①：按住中**不显示逐字识别**（中间态会误导英文名/专业词），
            // 说完（松手 final）一次性在用户气泡里显示最终句，可校准。
            // v3 决策依据③：提示"上滑取消"（豆包同款手势）。
            // v2.1 I14：加"正在听…"波形动画；3s 无识别 → "请说话"提示（说完一次性显示的补偿反馈）。
            #if canImport(Speech)
            if speech.calibrating && !keyboardMode {
                // v2.4 ASR 校准：松手后先到平台千问校准，校准完才一次性出文字（用户原话"先到ASR服务器校准，校准完再丢出来"）。
                HStack(spacing: 8) {
                    ProgressView().tint(.white)
                    Text("正在校准…")
                        .font(.system(size: 14, weight: .semibold))
                        .foregroundColor(.white)
                    Spacer()
                    Text("识别完成后自动发送")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.white.opacity(0.85))
                }
                .padding(.horizontal, 14).padding(.vertical, 10)
                .background(VSColor.blue)
                .cornerRadius(12)
                .padding(.horizontal, 12).padding(.top, 6)
                .transition(.move(edge: .top).combined(with: .opacity))
            } else if speech.asrFailed && !keyboardMode {
                // v2.4：ASR 服务器校准失败 → 明确提示重说（不回退本地识别）。
                HStack(spacing: 8) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .font(.system(size: 14))
                        .foregroundColor(.white)
                    Text("识别失败，请再按一次")
                        .font(.system(size: 14, weight: .semibold))
                        .foregroundColor(.white)
                    Spacer()
                }
                .padding(.horizontal, 14).padding(.vertical, 10)
                .background(Color.orange)
                .cornerRadius(12)
                .padding(.horizontal, 12).padding(.top, 6)
                .transition(.move(edge: .top).combined(with: .opacity))
            } else if speech.isRecording && !keyboardMode {
                VStack(spacing: 3) {
                    HStack(spacing: 8) {
                        // "正在听"波形（按住中持续呼吸，补偿"不显示逐字"的空窗期）
                        WaveView()
                            .colorScheme(.light)
                        Text(noSpeechDetected ? "没听到声音，请说话" : "正在听…")
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundColor(.white)
                        Spacer()
                        Text("松开 发送 · 上滑取消")
                            .font(.system(size: 12, weight: .medium))
                            .foregroundColor(.white.opacity(0.85))
                    }
                }
                .padding(.horizontal, 14).padding(.vertical, 10)
                .background(noSpeechDetected ? Color.orange : Color.red)
                .cornerRadius(12)
                .padding(.horizontal, 12).padding(.top, 6)
                .transition(.move(edge: .top).combined(with: .opacity))
                .onReceive(Timer.publish(every: 1, on: .main, in: .common).autoconnect()) { _ in
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
                }
            } else if speech.unavailable && !keyboardMode {
                Text("语音不可用：请在 系统设置→VoiceSign 中允许 麦克风 与 语音识别")
                    .font(.system(size: 12))
                    .foregroundColor(.white)
                    .padding(.horizontal, 12).padding(.vertical, 6)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color.gray.opacity(0.7))
                    .cornerRadius(8)
                    .padding(.horizontal, 12).padding(.top, 6)
            }
            #endif

            HStack(spacing: 12) {
                // 键盘/语音切换（豆包式：左侧小图标切换两种输入）
                Button {
                    #if canImport(Speech)
                    if speech.isRecording { speech.stopHold() }
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
                    // 按住说话大圆钮（豆包式主交互）
                    holdToTalkButton
                }
            }
            .padding(.horizontal, 16).padding(.vertical, 10)
        }
        .background(.ultraThinMaterial)
    }

    /// 按住说话：按下立即变红（不等引擎就绪），松手 endAudio → final → 自动提交。
    private var holdToTalkButton: some View {
        #if canImport(Speech)
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
                // 按住说话（豆包同款）：按下录音；松手发送；上滑取消（translation.height < -80）。
                .gesture(
                    DragGesture(minimumDistance: 0)
                        .onChanged { _ in
                            if !speech.isRecording { speech.startHold() }
                        }
                        .onEnded { v in
                            if speech.isRecording {
                                if v.translation.height < -80 {
                                    speech.cancelHold()   // 上滑取消：不发送
                                } else {
                                    speech.stopHold()     // 松手：识别完自动发送
                                }
                            }
                        }
                )
        }
        .frame(height: 64)
        #else
        Button { model.send() } label: {
            Image(systemName: "arrow.up.circle.fill")
                .font(.system(size: 30))
                .foregroundColor(.blue)
        }
        #endif
    }
}
