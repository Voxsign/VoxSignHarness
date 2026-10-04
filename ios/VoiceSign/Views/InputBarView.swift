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

    var body: some View {
        VStack(spacing: 0) {
            // 豆包式"松开发送"录音状态条：按住时出现（红底白字 + 实时识别文字）
            #if canImport(Speech)
            if speech.isRecording && !keyboardMode {
                VStack(spacing: 4) {
                    HStack(spacing: 6) {
                        Circle().fill(Color.white).frame(width: 8, height: 8)
                        Text("松开 发送")
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundColor(.white)
                        Spacer()
                    }
                    if !speech.transcript.isEmpty {
                        Text(speech.transcript)
                            .font(.system(size: 14))
                            .foregroundColor(.white.opacity(0.95))
                            .lineLimit(2)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
                .padding(.horizontal, 14).padding(.vertical, 8)
                .background(Color.red)
                .cornerRadius(12)
                .padding(.horizontal, 12).padding(.top, 6)
                .transition(.move(edge: .top).combined(with: .opacity))
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
                .background(speech.isRecording ? Color.red : VSColor.blue)
                .clipShape(Circle())
                .contentShape(Circle())
                .scaleEffect(speech.isRecording ? 1.05 : 1.0)
                .animation(.easeOut(duration: 0.12), value: speech.isRecording)
                .accessibilityIdentifier("vhs.hold")
                // 按住说话：DragGesture(minimumDistance:0) 按下即触发、松手即结束（豆包同款）
                .gesture(
                    DragGesture(minimumDistance: 0)
                        .onChanged { _ in
                            if !speech.isRecording { speech.startHold() }
                        }
                        .onEnded { _ in
                            if speech.isRecording { speech.stopHold() }
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
