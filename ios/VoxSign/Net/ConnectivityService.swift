//
//  ConnectivityService.swift
//  VoxSign
//
//  T2 豆包式交互·连接感知层（Connection-Aware Client 范式）：
//  ---------------------------------------------------------------
//  核心思想：客户端不再"盲目重连"，而是感知连接三态并驱动 UI 与队列：
//    unknown      → 启动初态，立即探测
//    online       → 服务可达（/v1/status 心跳通过）
//    offline      → 网络断 / 服务不可达（语音指令先进离线队列，不丢）
//    reconnecting → 路径恢复但心跳未过（短暂过渡态，UI 显示"正在重连…"）
//
//  触发源（三个，互不依赖）：
//    1. NWPathMonitor    网络路径变化（Wi-Fi/蜂窝/飞行模式）→ 立即重探
//    2. 心跳定时器       每 30s GET /v1/status（慢速自愈，服务重启后自动上线）
//    3. 主动探活         提交前 / 补投前主动 probe()，online 才走网络
//
//  与 DeliveryQueue 的协作（离线不丢语音）：
//    offline 时 submit → 直接入队（不等 15s 超时）；状态回到 online → 自动 flushQueue。
//
//  对用户可见的效果（豆包式体验）：
//    - 顶部状态胶囊：绿=已连接 · 灰=离线排队中 · 黄=重连中，不遮挡对话流
//    - 断网不再"卡死"：提交前探测，offline 立即给出"已入离线队列"反馈
//    - 切网自动恢复：Wi-Fi→蜂窝 / 飞行模式→恢复，数秒内自动补投
//

import Foundation
import Network
import Combine

/// 连接状态（三态 + 过渡态）。
enum ConnectionState: Equatable {
    case unknown       // 初态：尚未探测
    case online        // 服务可达
    case offline       // 服务不可达（网络断 / 服务器没起）
    case reconnecting  // 路径恢复，心跳未过
}

/// 连接感知服务（单例）。对外只发布 connectionState + 探活回调。
final class ConnectivityService: ObservableObject {
    static let shared = ConnectivityService()

    /// 当前连接状态（UI 顶部胶囊 + 提交/补投决策共用）。
    @Published private(set) var state: ConnectionState = .unknown

    /// 最近一次探测错误描述（设置页排障用）。
    @Published private(set) var lastError: String = ""

    /// UI v3：最近一次成功探测的延迟（毫秒），设置页连接状态行显示。
    @Published private(set) var latencyMs: Int = 0

    // 探测参数（可调；真机实测后按需收紧）。
    private let heartbeatInterval: TimeInterval = 30
    private let probeTimeout: TimeInterval = 5

    private let pathMonitor = NWPathMonitor()
    private let monitorQueue = DispatchQueue(label: "vhs.connectivity.monitor")
    private var heartbeatTimer: Timer?
    private var isProbing = false

    private init() {
        // 1) 网络路径监控：路径变化 → 立即重探（不再等 30s 心跳）。
        pathMonitor.pathUpdateHandler = { [weak self] path in
            DispatchQueue.main.async {
                guard let self = self else { return }
                if path.status == .satisfied {
                    // 路径恢复：若当前非 online，进入 reconnecting 并立即探活。
                    if self.state != .online {
                        self.setState(.reconnecting)
                        self.probe()
                    }
                } else {
                    // 路径断开：直接 offline（心跳也不用白等）。
                    self.setState(.offline)
                    self.lastError = "网络路径不可用（Wi-Fi/蜂窝断开）"
                }
            }
        }
        pathMonitor.start(queue: monitorQueue)

        // 2) 心跳定时器：周期探活（服务重启后 30s 内自动发现）。
        let timer = Timer(timeInterval: heartbeatInterval, repeats: true) { [weak self] _ in
            DispatchQueue.main.async {
                self?.probe()
            }
        }
        RunLoop.main.add(timer, forMode: .common)
        heartbeatTimer = timer
    }

    /// 启动探测（App onAppear 调用一次，触发初态 unknown → online/offline）。
    func start() {
        probe()
    }

    /// 服务器/机器码绑定变更后重置探测：清掉旧连接状态与错误，回到 unknown 并立即重探当前 base。
    func reset() {
        lastError = ""
        latencyMs = 0
        setState(.unknown)
        probe()
    }

    /// 探活：GET /v1/status，5s 超时。成功 → online；失败 → offline（并记错误）。
    /// UI v3：成功时记录延迟毫秒（设置页连接状态行显示"延迟 Xms"）。
    func probe() {
        guard !isProbing else { return }
        isProbing = true
        let started = Date()
        Task {
            defer { isProbing = false }
            do {
                _ = try await withThrowingTaskGroup(of: StatusResponse.self) { group in
                    group.addTask { try await APIClient.shared.status() }
                    group.addTask {
                        try await Task.sleep(nanoseconds: UInt64(self.probeTimeout * 1_000_000_000))
                        throw APIError.transport("心跳超时（\(Int(self.probeTimeout))s）")
                    }
                    let first = try await group.next()!
                    group.cancelAll()
                    return first
                }
                let latency = Int(Date().timeIntervalSince(started) * 1000)
                await MainActor.run {
                    self.setState(.online)
                    self.lastError = ""
                    self.latencyMs = latency
                }
            } catch {
                await MainActor.run {
                    self.setState(.offline)
                    self.lastError = (error as? APIError)?.errorDescription ?? error.localizedDescription
                }
            }
        }
    }

    /// 提交/补投前的主动探活：快速确认 online 才继续走网络（避免 15s 傻等）。
    /// - Returns: true=服务可达，false=不可达（调用方应入队或提示）。
    func isReachable() async -> Bool {
        // 缓存态 online 时直接放行（心跳已 30s 内验证过，省一次网络往返）。
        if state == .online { return true }
        // 非 online：主动探一次再裁决。
        probe()
        return state == .online
    }

    /// 网络恢复回调（AppModel 订阅：恢复 → flushQueue）。
    func onOnline(_ action: @escaping () -> Void) -> AnyCancellable {
        $state
            .filter { $0 == .online }
            .dropFirst()          // 跳过启动时的首次 online（由 AppModel 自行决定 flush 时机）
            .sink { _ in action() }
    }

    private func setState(_ s: ConnectionState) {
        if state != s {
            DiagLogger.shared.log("CONN", "状态 \(state) → \(s)")
        }
        state = s
    }
}
