//
//  SpeechRecognizer.swift
//  VoiceSign
//
//  麦克风输入：iOS Speech framework（SFSpeechRecognizer + AVAudioEngine）基础实现。
//  【如实标注】语音识别需真机授权（麦克风 + 语音识别权限）；模拟器/未授权时自动回退键盘输入。
//  权限项需在 Info.plist 配置 NSMicrophoneUsageDescription 与 NSSpeechRecognitionUsageDescription。
//

import Foundation
import Speech
import AVFoundation
import Combine

@available(iOS 13.0, *)
final class SpeechRecognizer: ObservableObject {
    static let shared = SpeechRecognizer()

    @Published var transcript: String = ""
    @Published var isRecording: Bool = false
    /// 识别不可用（无权限/不支持）→ UI 回退键盘。
    @Published var unavailable: Bool = false
    /// 常听模式（T1）：开启后录音会话常驻后台（UIBackgroundModes: audio），
    /// 识别到 final 不自动停、保持引擎运行，随时说话随时识别。默认按需关闭。
    @Published var alwaysOn: Bool = false {
        didSet {
            if alwaysOn {
                if !isRecording { start() }
            } else if isRecording {
                stop()
            }
        }
    }

    private var recognizer: SFSpeechRecognizer?
    private var request: SFSpeechAudioBufferRecognitionRequest?
    private var task: SFSpeechRecognitionTask?
    private let engine = AVAudioEngine()

    private init() {
        // 中文简体识别器（与 web zh-CN 对齐）。
        recognizer = SFSpeechRecognizer(locale: Locale(identifier: "zh-CN"))
    }

    /// 请求授权（首次会弹系统权限框）。
    func requestAuthorization() {
        SFSpeechRecognizer.requestAuthorization { [weak self] status in
            DispatchQueue.main.async {
                switch status {
                case .authorized: break
                default: self?.unavailable = true
                }
            }
        }
        AVAudioSession.sharedInstance().requestRecordPermission { [weak self] ok in
            DispatchQueue.main.async { if !ok { self?.unavailable = true } }
        }
    }

    func toggle() {
        isRecording ? stop() : start()
    }

    func start() {
        guard let recognizer = recognizer, recognizer.isAvailable else {
            unavailable = true
            return
        }
        do {
            let session = AVAudioSession.sharedInstance()
            // T1 常听模式：playAndRecord + background 支持后台持续采集（配合 UIBackgroundModes: audio）。
            // 按需模式保持原 .record 行为，互不干扰。
            if alwaysOn {
                try session.setCategory(.playAndRecord, mode: .measurement,
                                        options: [.duckOthers, .allowBluetooth, .defaultToSpeaker])
            } else {
                try session.setCategory(.record, mode: .measurement, options: .duckOthers)
            }
            try session.setActive(true, options: .notifyOthersOnDeactivation)

            let req = SFSpeechAudioBufferRecognitionRequest()
            req.shouldReportPartialResults = true
            // ASR 标点全自动：显式开启，不依赖系统"设置→键盘→自动标点符号"开关；
            // 即便系统开关关闭，iPhonePeter（iOS 16+）也由本请求级属性强制出标点。
            if #available(iOS 16.0, *) {
                req.addsPunctuation = true
            }
            self.request = req

            let node = engine.inputNode
            let format = node.outputFormat(forBus: 0)
            node.installTap(onBus: 0, bufferSize: 1024, format: format) { [weak self] buffer, _ in
                self?.request?.append(buffer)
            }
            engine.prepare()
            try engine.start()

            task = recognizer.recognitionTask(with: req) { [weak self] result, error in
                guard let self = self else { return }
                if let result = result {
                    DispatchQueue.main.async {
                        self.transcript = result.bestTranscription.formattedString
                    }
                }
                // T1 常听：识别到 final（停顿）只复位 request，不停止引擎，继续下一段；
                // 按需模式保持原行为（final/错误即停）。
                if error != nil {
                    DispatchQueue.main.async {
                        self.stop()
                    }
                } else if result?.isFinal ?? false {
                    if self.alwaysOn {
                        // 一段结束：丢弃本次 request，重启一段（引擎与会话保持运行）。
                        self.request?.endAudio()
                        self.request = nil
                        self.task = nil
                        if self.isRecording {
                            self.beginNewSegment()
                        }
                    } else {
                        self.stop()
                    }
                }
            }
            isRecording = true
            transcript = ""
        } catch {
            unavailable = true
            stop()
        }
    }

    /// 常听模式：一段识别结束后原地开新段（复用同一引擎/会话）。
    private func beginNewSegment() {
        guard let recognizer = recognizer, recognizer.isAvailable else { return }
        let req = SFSpeechAudioBufferRecognitionRequest()
        req.shouldReportPartialResults = true
        if #available(iOS 16.0, *) {
            req.addsPunctuation = true
        }
        self.request = req
        task = recognizer.recognitionTask(with: req) { [weak self] result, error in
            guard let self = self else { return }
            if let result = result {
                DispatchQueue.main.async {
                    self.transcript = result.bestTranscription.formattedString
                }
            }
            if error != nil {
                DispatchQueue.main.async { self.stop() }
            } else if result?.isFinal ?? false, self.alwaysOn {
                self.request?.endAudio()
                self.request = nil
                self.task = nil
                if self.isRecording { self.beginNewSegment() }
            } else if result?.isFinal ?? false {
                self.stop()
            }
        }
    }

    func stop() {
        engine.inputNode.removeTap(onBus: 0)
        if engine.isRunning { engine.stop() }
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        request?.endAudio()
        task?.cancel()
        request = nil
        task = nil
        isRecording = false
    }

    /// P1 一轮一清：发送后清空识别缓冲，下一轮从空白开始（不带上一轮残留）。
    /// 调用时机：send 之后（此时识别已 final/手动 stop），start() 也会再清一次作双保险。
    func resetRound() {
        transcript = ""
    }
}
