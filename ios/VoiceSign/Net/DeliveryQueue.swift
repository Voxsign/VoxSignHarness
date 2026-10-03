//
//  DeliveryQueue.swift
//  VoiceSign
//
//  T1 后台能力 · 投递队列（DeliveryChannel）：
//  断网/后台/提交失败时把「文本+mode+request_id」持久化入队，网络恢复后按序补投。
//  幂等由 request_id 保证（服务端 POST /v1/tasks 同 request_id → 200 deduped）。
//  零第三方依赖：文件持久化（Application Support）+ URLSession（复用 APIClient）。
//

import Foundation

/// 队列中的一条待投递任务（可持久化编码）。
struct PendingSubmission: Codable, Equatable {
    let requestId: String
    let text: String
    let mode: String       // voice | text
    let createdAt: Date
    var retryCount: Int

    init(requestId: String = UUID().uuidString, text: String, mode: String, createdAt: Date = Date(), retryCount: Int = 0) {
        self.requestId = requestId
        self.text = text
        self.mode = mode
        self.createdAt = createdAt
        self.retryCount = retryCount
    }
}

/// 投递队列：FIFO + 持久化 + 退避重试。
/// - 线程安全：所有操作在主 actor / 串行访问下调用（AppModel 已是 @MainActor）。
/// - 持久化：Application Support/DeliveryQueue.jsonl；写失败不丢内存态。
/// - 上限：maxStored 条（防无限膨胀）；入队超限丢弃最旧。
final class DeliveryQueue {
    static let shared = DeliveryQueue()

    /// 存储根目录（测试可注入）。默认 Application Support。
    var storageDirectory: URL? {
        didSet { reload() }
    }
    /// 提交闭包（测试可注入；默认走 APIClient.submitTask）。
    var submitter: ((PendingSubmission) async throws -> Void)?
    /// 最大退避秒数（指数退避 2^n，封顶）。
    var maxBackoffSeconds: Int = 300
    /// 入队上限。
    var maxStored: Int = 100

    /// 当前队列（供 UI/测试读取）。
    private(set) var pending: [PendingSubmission] = []
    /// 最后一次提交失败时间（退避计算）。
    private(set) var lastFailureAt: Date?

    private let queueURL = "DeliveryQueue.jsonl"

    init() {
        submitter = { [weak self] item in
            _ = try await APIClient.shared.submitTask(text: item.text, requestId: item.requestId)
        }
        reload()
    }

    // MARK: - 持久化

    private func queueFile() -> URL? {
        guard let dir = storageDirectory else {
            guard let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first else {
                return nil
            }
            let dir = support.appendingPathComponent("VoiceSign", isDirectory: true)
            try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
            return dir.appendingPathComponent(queueURL)
        }
        return dir.appendingPathComponent(queueURL)
    }

    private func reload() {
        guard let file = queueFile(), let data = try? Data(contentsOf: file) else { return }
        let lines = data.split(separator: 0x0A).compactMap { line -> PendingSubmission? in
            guard let d = String(data: Data(line), encoding: .utf8)?.data(using: .utf8) else { return nil }
            return try? JSONDecoder().decode(PendingSubmission.self, from: d)
        }
        pending = lines
    }

    private func persist() {
        guard let file = queueFile() else { return }
        let lines = pending.compactMap { item -> Data? in try? JSONEncoder().encode(item) }
        var data = Data()
        for line in lines { data.append(line); data.append(0x0A) }
        try? data.write(to: file, options: .atomic)
    }

    // MARK: - 队列操作

    /// 入队（断网/失败时）。返回是否入队成功。
    @discardableResult
    func enqueue(_ item: PendingSubmission) -> Bool {
        if pending.count >= maxStored {
            if pending.isEmpty { return false } // 队列已满且为空（maxStored=0）
            pending.removeFirst()               // 超限丢最旧
        }
        pending.append(item)
        persist()
        return true
    }

    /// 队列是否为空。
    var isEmpty: Bool { pending.isEmpty }

    /// 待投递条数。
    var count: Int { pending.count }

    /// 清空（投递成功后全清）。
    func clearAll() {
        pending.removeAll()
        lastFailureAt = nil
        persist()
    }

    // MARK: - 投递

    /// 尝试补投全部：逐条提交；失败即停（保序 + 退避），成功一条移出一条。
    /// - Returns: 本次成功投递的条数。
    @discardableResult
    func flush() async -> Int {
        guard !pending.isEmpty else { return 0 }
        // 退避：距上次失败不足 2^retry 秒则整轮等待，避免无意义风暴。
        if let failAt = lastFailureAt {
            let first = pending[0]
            let backoff = min(maxBackoffSeconds, 1 << min(first.retryCount, 9))
            if Date().timeIntervalSince(failAt) < Double(backoff) {
                return 0
            }
        }
        var delivered = 0
        var idx = 0
        while idx < pending.count {
            let item = pending[idx]
            do {
                if let submitter = submitter {
                    try await submitter(item)
                }
                pending.remove(at: idx)          // 成功即移出
                delivered += 1
                lastFailureAt = nil
            } catch {
                pending[idx].retryCount += 1     // 失败计数 +1（下次退避更长）
                lastFailureAt = Date()
                persist()
                break                             // 保序：头阻塞则停
            }
        }
        persist()
        return delivered
    }
}
