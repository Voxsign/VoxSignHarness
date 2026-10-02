//
//  InputBarView.swift
//  VoiceSign
//
//  底部麦克风输入条：麦克风按钮（真语音→文本可改）+ 文本输入 + 发送。
//  语音识别不可用（无权限/模拟器）时自动回退键盘输入。
//

import SwiftUI

struct InputBarView: View {
    @EnvironmentObject var model: AppModel
    #if canImport(Speech)
    @EnvironmentObject var speech: SpeechRecognizer
    #endif

    var body: some View {
        HStack(spacing: 10) {
            // 麦克风按钮
            Button {
                #if canImport(Speech)
                speech.toggle()
                #endif
            } label: {
                Image(systemName: micIcon)
                    .font(.system(size: 18))
                    .foregroundColor(micTint)
                    .frame(width: 34, height: 34)
                    .background(micBg)
                    .clipShape(Circle())
            }

            // 文本输入（语音识别结果回显可改；final 结果整段替换，不拼接）
            TextField("说点什么，或打字…", text: $model.inputText)
                .textFieldStyle(.roundedBorder)
                .accessibilityIdentifier("vhs.input")
                .disabled(model.decision != nil)
                .onChange(of: transcript) { newValue in
                    // P1：一轮一清——final/增量结果整段替换输入框，绝不与旧文本拼接。
                    if !newValue.isEmpty { model.inputText = newValue }
                }

            // 发送（决策点出现时禁用，兑现"一屏一个决策点"）
            Button {
                model.send()
                // P1：发送后同时清掉语音识别缓冲，下一轮重新开始（不带上一轮残留）。
                #if canImport(Speech)
                speech.resetRound()
                #endif
            } label: {
                Image(systemName: "arrow.up.circle.fill")
                    .font(.system(size: 28))
                    .foregroundColor(model.decision != nil ? .gray : VSColor.blue)
            }
            .accessibilityIdentifier("vhs.send")
            .disabled(model.decision != nil || model.inputText.isEmpty)
        }
        .padding(.horizontal, 12).padding(.vertical, 8)
        .background(.ultraThinMaterial)
    }

    private var transcript: String {
        #if canImport(Speech)
        speech.transcript
        #else
        ""
        #endif
    }

    private var micIcon: String {
        #if canImport(Speech)
        speech.isRecording ? "waveform" : "mic.fill"
        #else
        "mic"
        #endif
    }

    private var micTint: Color {
        #if canImport(Speech)
        speech.isRecording ? .red : VSColor.blue
        #else
        .gray
        #endif
    }

    private var micBg: Color {
        #if canImport(Speech)
        speech.isRecording ? Color.red.opacity(0.15) : Color.gray.opacity(0.1)
        #else
        Color.gray.opacity(0.1)
        #endif
    }
}
