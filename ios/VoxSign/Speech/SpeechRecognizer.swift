//
//  SpeechRecognizer.swift
//  VoxSign
//
//  麦克风输入：v3 纯录音版（2026-10-04 用户定：本地离线识别全部移除）。
//  链路：按住录音（AVAudioEngine → WAV）→ 松手 → POST Harness /v1/asr
//        → ASR 服务器（平台千问 + 个性化热词）校准 → 校准文本一次性显示 → 自动提交。
//  校准失败 → 明确提示「识别失败，请再按一次」，**不回退本地识别**。
//

import Foundation
import AVFoundation
import Combine

final class SpeechRecognizer: ObservableObject {
    static let shared = SpeechRecognizer()

    @Published var transcript: String = ""
    @Published var isRecording: Bool = false
    /// 识别不可用（无权限/不支持）→ UI 回退键盘。
    @Published var unavailable: Bool = false
    /// 豆包式"按住说话"模式：按住录音、松手上传 ASR 服务器校准 → 自动提交。
    private var holdMode = false
    /// ASR 校准态：松手 → "正在校准…"（不出文字）→ 平台千问校准完成才出文字。
    @Published var calibrating: Bool = false
    /// 校准失败态：显示「识别失败，请再按一次」（不回退本地识别）。
    @Published var asrFailed: Bool = false
    /// 松手后 WAV 为空（没录到声音）→ 提示重说（与 asrFailed 分开，给用户明确原因）。
    @Published var emptyRecording: Bool = false
    /// 按住录音时的实时振幅（0~1），驱动声波条动画（豆包式"按住有反应"）。
    @Published var meterLevel: Float = 0

    /// UI v3：按住录音累计秒数（语音气泡时长显示，如 "3″"）。startHold 清零，stopHold/cancelHold 定格。
    private(set) var lastHoldSeconds: Int = 0
    private var holdTimer: Timer?

    private let engine = AVAudioEngine()
    /// 录音 WAV 文件（平台校准用）与对应 URL。
    private var audioFile: AVAudioFile?
    private var audioURL: URL?
    /// v2.3 重入防护：isRecording 是异步置位，快速连按时 start() 可能被重复调用，
    /// 导致 installTap 二次注册崩溃。启动中/录音中直接忽略。
    private var starting = false

    private init() {}

    /// 请求麦克风授权（首次会弹系统权限框）。
    func requestAuthorization() {
        AVAudioSession.sharedInstance().requestRecordPermission { [weak self] ok in
            DispatchQueue.main.async { if !ok { self?.unavailable = true } }
        }
    }

    func toggle() {
        isRecording ? stop() : start()
    }

    // MARK: - 豆包式"按住说话"（主交互）

    /// 按住开始录音（纯录音，不喂任何本地识别）。
    func startHold() {
        holdMode = true
        // UI v3：按住录音计时（语音气泡时长）。
        lastHoldSeconds = 0
        holdTimer?.invalidate()
        holdTimer = Timer.scheduledTimer(withTimeInterval: 1.0, repeats: true) { [weak self] _ in
            guard let self = self, self.isRecording else { return }
            self.lastHoldSeconds += 1
        }
        // 用户要说话，先停掉上一段回复朗读（录音 session 也会切走 playback）。
        VoiceOutputService.shared.stop()
        start()
    }

    /// 松手（ASR 校准）：停引擎/关 WAV → 上传 ASR 服务器（平台千问 + 热词）校准。
    /// 校准成功 → 一次性出文字 → 自动提交；失败 → asrFailed 提示（无本地兜底）。
    func stopHold() {
        holdMode = false
        holdTimer?.invalidate()
        holdTimer = nil
        endRecordingSession()
        startCalibration()
    }

    /// 上滑取消（豆包同款手势）：直接清理录音，**不发送**。
    func cancelHold() {
        print("[ASR] cancelHold（上滑取消）")
        DiagLogger.shared.log("ASR", "cancelHold（上滑取消）")
        holdMode = false
        holdTimer?.invalidate()
        holdTimer = nil
        calibrating = false
        asrFailed = false
        emptyRecording = false
        audioFile = nil
        audioURL = nil
        engine.inputNode.removeTap(onBus: 0)
        if engine.isRunning { engine.stop() }
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        isRecording = false
        transcript = ""
    }

    /// 结束录音会话：停 tap/引擎/会话，关 WAV 文件。
    private func endRecordingSession() {
        engine.inputNode.removeTap(onBus: 0)
        if engine.isRunning { engine.stop() }
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        audioFile = nil   // close AVAudioFile（flush WAV）
        isRecording = false
    }

    /// ASR 校准：松手后把 WAV 上传 /v1/asr → ASR 服务器（平台千问 + 个性化热词）校准。
    /// 成功 → 一次性出文字 + 自动提交；失败 → asrFailed（提示重说，不回退本地识别）。
    private func startCalibration() {
        calibrating = true
        asrFailed = false
        emptyRecording = false
        guard let url = audioURL, FileManager.default.fileExists(atPath: url.path),
              let audioData = try? Data(contentsOf: url), !audioData.isEmpty else {
            // 无有效录音（没说话/录音失败）→ 明确提示"没录到声音"，与识别失败区分。
            calibrating = false
            emptyRecording = true
            return
        }
        // WAV 有文件但 data chunk 为空（如转换失败）→ 同样按"没录到声音"处理。
        let wavDataLen = wavDataChunkLength(audioData)
        if wavDataLen == 0 {
            print("[ASR] WAV data chunk empty (bytes=\(audioData.count))")
            DiagLogger.shared.log("ASR", "WAV data empty bytes=\(audioData.count)")
            calibrating = false
            emptyRecording = true
            return
        }
        print("[ASR] WAV bytes=\(audioData.count) dataLen=\(wavDataLen) at \(url.lastPathComponent)")
        DiagLogger.shared.log("ASR", "WAV bytes=\(audioData.count) dataLen=\(wavDataLen)")
        uploadAudio(audioData) { [weak self] text, ok in
            DispatchQueue.main.async {
                guard let self = self else { return }
                self.calibrating = false
                if ok, let text = text, !text.isEmpty {
                    // ASR 服务器校准成功：一次性出文字 + 自动提交（豆包式）。
                    self.transcript = text
                    self.asrFailed = false
                    self.onFinalSegment?(text)
                } else {
                    // ASR 服务器未就绪/失败：明确提示重说，绝不用本地识别兜底。
                    self.asrFailed = true
                }
            }
        }
    }

    /// T2 豆包式交互：校准得到完整文本时回调文本。
    /// AppModel 设置后即"开口即达"——校准完自动提交，无需再按发送键。
    var onFinalSegment: ((String) -> Void)?

    /// 上传 WAV → harness /v1/asr（multipart file=）→ ASR 服务器校准。20s 超时。
    private func uploadAudio(_ data: Data, completion: @escaping (String?, Bool) -> Void) {
        let base = SettingsStore.shared.base
        guard !base.isEmpty, let endpoint = URL(string: base + "/v1/asr") else {
            completion(nil, false); return
        }
        var req = URLRequest(url: endpoint)
        req.httpMethod = "POST"
        req.timeoutInterval = 45
        let tok = SettingsStore.shared.token
        if !tok.isEmpty {
            req.setValue("Bearer " + tok, forHTTPHeaderField: "Authorization")
        }
        let boundary = "vhs-\(UUID().uuidString)"
        req.setValue("multipart/form-data; boundary=\(boundary)", forHTTPHeaderField: "Content-Type")
        var body = Data()
        body.append("--\(boundary)\r\n".data(using: .utf8)!)
        body.append("Content-Disposition: form-data; name=\"file\"; filename=\"voice.wav\"\r\n".data(using: .utf8)!)
        body.append("Content-Type: audio/wav\r\n\r\n".data(using: .utf8)!)
        body.append(data)
        body.append("\r\n--\(boundary)--\r\n".data(using: .utf8)!)
        req.httpBody = body
        URLSession.shared.dataTask(with: req) { data, _, err in
            guard err == nil, let data = data,
                  let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let ok = obj["ok"] as? Bool, ok,
                  let text = obj["text"] as? String, !text.isEmpty else {
                completion(nil, false)
                return
            }
            completion(text, true)
        }.resume()
    }

    func start() {
        guard !isRecording, !starting else {
            print("[ASR] start 忽略（已在录音/启动中）")
            return
        }
        starting = true
        defer { starting = false }
        // 豆包式：按下瞬间立即进录音态（UI 先反馈，不等引擎就绪）。
        DispatchQueue.main.async {
            self.transcript = ""
            self.isRecording = true
            self.calibrating = false
            self.asrFailed = false
        }
        print("[ASR] start: mic=\(micStatus()) hold=\(holdMode)")
        DiagLogger.shared.log("ASR", "start mic=\(micStatus())")
        // 【闪退修复 T3】权限前置检查：麦克风未授权时，AVAudioEngine 的
        // installTap/engine.start 会抛 NSException（Swift do-catch 捕不到）→ 直接闪退。
        let mic = AVAudioSession.sharedInstance().recordPermission
        switch mic {
        case .granted:
            break
        case .undetermined:
            print("[ASR] mic undetermined → 请求授权")
            AVAudioSession.sharedInstance().requestRecordPermission { [weak self] ok in
                DispatchQueue.main.async {
                    guard let self = self else { return }
                    print("[ASR] mic 授权结果 ok=\(ok)")
                    DiagLogger.shared.log("ASR", "mic 授权 ok=\(ok)")
                    if ok {
                        self.start()   // 授权成功：立即补启动（用户不用再按一次）
                    } else {
                        self.unavailable = true
                        self.isRecording = false
                    }
                }
            }
            return
        case .denied:
            print("[ASR] mic denied → unavailable")
            DispatchQueue.main.async {
                self.unavailable = true
                self.isRecording = false
            }
            return
        @unknown default:
            return
        }
        do {
            let session = AVAudioSession.sharedInstance()
            try session.setCategory(.record, mode: .measurement, options: .duckOthers)
            try session.setActive(true, options: .notifyOthersOnDeactivation)
            print("[ASR] session 已激活 (record)")
            DiagLogger.shared.log("ASR", "session 已激活")
            // v2.3 崩溃修复：AVAudioEngine 已有 tap 时再次 installTap 会抛 ObjC NSException。
            // installTap 前**幂等清理**：引擎还在跑就停、已有 tap 就移除，再开新 tap。
            if engine.isRunning { engine.stop() }
            engine.inputNode.removeTap(onBus: 0)

            let node = engine.inputNode
            let hardwareFormat = node.outputFormat(forBus: 0)
            print("[ASR] installTap format=\(hardwareFormat.sampleRate)Hz ch=\(hardwareFormat.channelCount)")
            // v2.5 录音修复：AVAudioFile 统一写 16k Int16 单声道。
            // 1) 旧版强制 channels=1 写文件，但 tap buffer 用硬件格式（可能 2ch），
            //    af.write 因声道不匹配静默失败 → WAV data chunk 为空 → 平台 422 asr_audio_empty。
            // 2) 16k Int16 是平台 ASR 标准输入格式（已实测 200）：文件体积比 48k Float32 小 12 倍，
            //    上传更快、平台预处理更省，端到端延迟更稳。
            // tap 按硬件格式收 buffer，用 AVAudioConverter 转成目标格式再写文件。
            let fileURL = FileManager.default.temporaryDirectory
                .appendingPathComponent("vhs-voice-\(Int(Date().timeIntervalSince1970 * 1000)).wav")
            let targetFormat = AVAudioFormat(commonFormat: .pcmFormatInt16,
                                             sampleRate: 16000, channels: 1, interleaved: false)!
            do {
                audioFile = try AVAudioFile(forWriting: fileURL,
                                            settings: targetFormat.settings,
                                            commonFormat: .pcmFormatInt16, interleaved: false)
            } catch {
                print("[ASR] AVAudioFile create FAIL: \(error.localizedDescription)")
                DiagLogger.shared.log("ASR", "AVAudioFile FAIL: \(error.localizedDescription)")
                audioFile = nil
            }
            audioURL = fileURL
            let converter = AVAudioConverter(from: hardwareFormat, to: targetFormat)
            node.installTap(onBus: 0, bufferSize: 1024, format: hardwareFormat) { [weak self] buffer, _ in
                guard let self = self else { return }
                guard let af = self.audioFile else { return }
                guard let cv = converter else { return }
                let ratio = targetFormat.sampleRate / hardwareFormat.sampleRate
                let cap = AVAudioFrameCount(Double(buffer.frameLength) * ratio) + 1024
                guard let outBuf = AVAudioPCMBuffer(pcmFormat: targetFormat, frameCapacity: cap) else { return }
                var convErr: NSError?
                let status = cv.convert(to: outBuf, error: &convErr) { _, outStatus in
                    outStatus.pointee = .haveData
                    return buffer
                }
                if convErr != nil {
                    print("[ASR] convert FAIL: \(convErr!.localizedDescription)")
                    return
                }
                if status == .haveData || status == .inputRanDry {
                    if outBuf.frameLength > 0 {
                        do {
                            try af.write(from: outBuf)
                        } catch {
                            print("[ASR] tap write FAIL: \(error.localizedDescription)")
                            DiagLogger.shared.log("ASR", "tap write FAIL: \(error.localizedDescription)")
                        }
                    }
                }
                // 按住时的实时振幅反馈：从原始 buffer 算峰值（硬件格式通常 float32），
                // 低通平滑后驱动 UI 声波条（豆包式"按住有反应"）。
                if let ch = buffer.floatChannelData, buffer.format.commonFormat == .pcmFormatFloat32 {
                    var peak: Float = 0
                    let n = Int(buffer.frameLength)
                    let stride = buffer.stride
                    for i in 0..<n {
                        let v = abs(ch[0][i * stride])
                        if v > peak { peak = v }
                    }
                    // 用双声道峰值
                    if buffer.format.channelCount > 1, let ch1 = buffer.floatChannelData?[1] {
                        for i in 0..<n {
                            let v = abs(ch1[i * stride])
                            if v > peak { peak = v }
                        }
                    }
                    let lvl = min(1, peak * 2.5)
                    DispatchQueue.main.async { [weak self] in
                        guard let self = self else { return }
                        self.meterLevel = self.meterLevel * 0.65 + lvl * 0.35
                    }
                }
            }
            engine.prepare()
            try engine.start()
            print("[ASR] engine.start OK")
            DiagLogger.shared.log("ASR", "engine.start OK")
            DispatchQueue.main.async {
                self.isRecording = true
                self.transcript = ""
            }
        } catch {
            print("[ASR] 启动异常(可捕): \(error.localizedDescription)")
            DiagLogger.shared.log("ASR", "启动异常: \(error.localizedDescription)")
            DispatchQueue.main.async { self.isRecording = false }
            unavailable = true
            stop()
        }
    }

    func stop() {
        print("[ASR] stop")
        audioFile = nil
        engine.inputNode.removeTap(onBus: 0)
        if engine.isRunning { engine.stop() }
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        isRecording = false
        meterLevel = 0
    }

    // MARK: - 日志辅助

    private func micStatus() -> String {
        switch AVAudioSession.sharedInstance().recordPermission {
        case .granted: return "granted"
        case .denied: return "denied"
        case .undetermined: return "undetermined"
        @unknown default: return "unknown"
        }
    }

    /// 解析 WAV 头里 data chunk 的字节长度（4 字节小端）。解析失败返回 -1。
    private func wavDataChunkLength(_ data: Data) -> Int {
        guard data.count >= 12, data[0] == 0x52, data[1] == 0x49, data[2] == 0x46, data[3] == 0x46 else {
            return -1
        }
        var offset = 12
        while offset + 8 <= data.count {
            let id = String(data: data.subdata(in: offset..<offset + 4), encoding: .ascii) ?? ""
            let len = data.withUnsafeBytes { $0.loadUnaligned(fromByteOffset: offset + 4, as: UInt32.self) }
            if id == "data" { return Int(len) }
            offset += 8 + Int(len)
        }
        return -1
    }

    /// P1 一轮一清：发送后清空识别缓冲，下一轮从空白开始。
    func resetRound() {
        transcript = ""
    }
}
