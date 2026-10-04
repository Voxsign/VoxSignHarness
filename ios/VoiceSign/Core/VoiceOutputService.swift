//
//  VoiceOutputService.swift
//  VoiceSign
//
//  T3 豆包式交互·语音朗读（TTS）：Harness 的回复/决策点问题到达即朗读，
//  用户不用看屏（豆包 App 同款体验）。端侧 AVSpeechSynthesizer，中文 zh-CN。
//

import Foundation
import AVFoundation

final class VoiceOutputService: ObservableObject {
    static let shared = VoiceOutputService()

    /// 是否启用朗读（豆包默认开；设置页可关）。
    @Published var enabled: Bool {
        didSet { defaults.set(enabled, forKey: enabledKey) }
    }
    /// 朗读语速（0.4~0.6，中文舒适区间）。
    @Published var rate: Float {
        didSet { defaults.set(rate, forKey: rateKey) }
    }

    private let defaults = UserDefaults.standard
    private let enabledKey = "vhs-ios-tts-enabled"
    private let rateKey = "vhs-ios-tts-rate"
    private let synthesizer = AVSpeechSynthesizer()

    private init() {
        enabled = defaults.object(forKey: enabledKey) as? Bool ?? true
        let r = defaults.object(forKey: rateKey) as? Float ?? 0.5
        rate = min(max(r, 0.4), 0.6)
    }

    /// 朗读一段文本（中文）。enabled 关闭 / 文本为空时不动作。
    func speak(_ text: String) {
        guard enabled else { return }
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !t.isEmpty else { return }
        // 打断上一段（连续回复时只说最新的）。
        if synthesizer.isSpeaking { synthesizer.stopSpeaking(at: .immediate) }
        let utterance = AVSpeechUtterance(string: t)
        utterance.voice = AVSpeechSynthesisVoice(language: "zh-CN")
        utterance.rate = rate
        utterance.pitchMultiplier = 1.0
        synthesizer.speak(utterance)
    }

    /// 立即停止朗读（用户再次按住说话 / 切后台时可调）。
    func stop() {
        if synthesizer.isSpeaking {
            synthesizer.stopSpeaking(at: .immediate)
        }
    }
}
