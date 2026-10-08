//
//  VoiceOutputService.swift
//  VoxSign
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
    /// 云端 TTS 音频播放器（阿里云 CosyVoice，经 tts.voxsign.ai 网关）。
    private var currentPlayer: AVAudioPlayer?
    private var currentDataTask: URLSessionDataTask?
    /// 云端 TTS 端点（本地优先链路中的云端引擎；阿语/中文均由 CosyVoice 合成）。
    private let ttsEndpoint = URL(string: "https://tts.voxsign.ai/v1/tts")!

    private init() {
        enabled = defaults.object(forKey: enabledKey) as? Bool ?? true
        let r = defaults.object(forKey: rateKey) as? Float ?? 0.5
        rate = min(max(r, 0.4), 0.6)
    }

    /// 朗读一段文本（中英阿自适应）。enabled 关闭 / 文本为空时不动作。
    /// 主链路：POST 到云端 TTS 网关（阿里云 CosyVoice 合成 WAV）→ AVAudioPlayer 播放；
    /// 失败（网络/非 200）自动回退 iOS 系统朗读（AVSpeechSynthesizer）。
    func speak(_ text: String) {
        guard enabled else { return }
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !t.isEmpty else { return }
        // 【无声修复 T4】录音 endRecordingSession 会把 AVAudioSession setActive(false)，
        // 且 App 从未设 playback 类别（默认 ambient 受静音拨片影响）→ 朗读完全无声。
        // 朗读前强制切 playback（扬声器、无视静音拨片）+ 激活会话；失败不阻断朗读（继续尝试）。
        do {
            let session = AVAudioSession.sharedInstance()
            try session.setCategory(.playback, mode: .spokenAudio, options: [.duckOthers])
            try session.setActive(true, options: [])
        } catch {
            DiagLogger.shared.log("TTS", "audio session 设置失败：\(error.localizedDescription)")
        }
        // 打断上一段（连续回复时只说最新的）。
        currentPlayer?.stop()
        currentPlayer = nil
        currentDataTask?.cancel()
        if synthesizer.isSpeaking { synthesizer.stopSpeaking(at: .immediate) }
        // 云端 TTS 主链路。
        var req = URLRequest(url: ttsEndpoint)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.timeoutInterval = 30
        req.httpBody = try? JSONSerialization.data(withJSONObject: ["text": t])
        currentDataTask = URLSession.shared.dataTask(with: req) { [weak self] data, resp, err in
            guard let self = self else { return }
            guard err == nil,
                  let data = data,
                  let http = resp as? HTTPURLResponse, http.statusCode == 200,
                  let wav = try? AVAudioPlayer(data: data) else {
                // 云端失败 → 回退 iOS 系统朗读（中英阿自适应）。
                DiagLogger.shared.log("TTS", "云端合成失败，回退系统朗读：\(err?.localizedDescription ?? "non-200")")
                DispatchQueue.main.async { self._fallbackSpeak(t) }
                return
            }
            DispatchQueue.main.async {
                wav.prepareToPlay()
                self.currentPlayer = wav
                self.currentPlayer?.play()
            }
        }
        currentDataTask?.resume()
    }

    /// 回退：iOS 系统朗读（云端不可达时兜底，中英阿自适应）。
    private func _fallbackSpeak(_ t: String) {
        let utterance = AVSpeechUtterance(string: t)
        let hasArabic = t.range(of: #"[\u0600-\u06FF]"#, options: .regularExpression) != nil
        utterance.voice = AVSpeechSynthesisVoice(language: hasArabic ? "ar-SA" : "zh-CN")
        utterance.rate = rate
        utterance.pitchMultiplier = 1.0
        synthesizer.speak(utterance)
    }

    /// 立即停止朗读（用户再次按住说话 / 切后台时可调）。
    func stop() {
        currentDataTask?.cancel()
        currentPlayer?.stop()
        currentPlayer = nil
        if synthesizer.isSpeaking {
            synthesizer.stopSpeaking(at: .immediate)
        }
    }
}
