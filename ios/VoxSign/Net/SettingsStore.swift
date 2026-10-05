//
//  SettingsStore.swift
//  VoxSign
//
//  豆包式设置（T3）：双连接模式——
//  - 云道（默认）：零配置，仅 Google 登录；base/token 指向 VoxSign 云端（与服务器列表无关）。
//  - 自建：默认空列表，用户自己添加服务器（IP 地址 / 机器码 两种方式）；
//    条目记录绑定方式与连接方式（直连 / 云端转发）。
//  下游（APIClient/SSEClient）继续用 base/token 计算属性，无需改动。
//

import Foundation

/// 连接模式：云道（默认）/ 自建。
enum ConnectionMode: String, Codable, CaseIterable {
    case cloud        // 云道 · 默认：Google 登录即用，零配置
    case selfHosted   // 自建：自己添加服务器
}

/// 一台可连接的 Harness 服务器（自建）。
struct ServerConfig: Identifiable, Codable, Equatable {
    var id: String
    var name: String
    var base: String
    var token: String
    /// 机器码绑定（非 nil = 通过机器码添加，地址由云道定位带入）。
    var machineCode: String?
    /// 云端转发（内网服务器经云道中转；nil = 直连）。
    var viaRelay: Bool?

    var isMachineBound: Bool { machineCode != nil }
    var usesRelay: Bool { viaRelay ?? false }
}

/// Google 登录态（云道模式：租户 = Google sub）。
struct GoogleAuthState: Codable, Equatable {
    var email: String
    var tenant: String
    var tier: String
    var quotaUsed: Int?
    var quotaLimit: Int?
    var resetsAt: String?
    var trialUntil: String?
    /// 会话 JWT（云道模式下作为 Bearer token；老版本数据可能缺失 → optional 兼容）。
    var token: String?
    /// 兼容保留（v2.1 曾记录服务器地址；云道模式不再依赖它）。
    var serverBase: String
}

/// 观测式设置仓库：App 启动时读 UserDefaults，保存时回写。
final class SettingsStore: ObservableObject {
    static let shared = SettingsStore()

    private let defaults = UserDefaults.standard
    private let serversKey = "vhs-ios-servers"
    private let activeKey = "vhs-ios-active"
    private let modeKey = "vhs-ios-mode"

    /// 连接模式（云道默认 / 自建），持久化。
    @Published var mode: ConnectionMode = .cloud

    /// 自建服务器列表（默认空，用户自己添加），持久化。
    /// 注：不挂 didSet（init 阶段赋值会触发 didSet 且此时 self 未完整初始化会编译报错），
    /// 持久化统一在变更方法里显式 persist()。
    @Published var servers: [ServerConfig] = []
    /// 当前活动服务器 id（自建模式），持久化。
    @Published var activeServerID: String = ""

    /// 云道地址（正式域名 voxsign.ai，nginx 转发 /v1 → 云端 harness 8898）。
    var cloudBase: String { "https://voxsign.ai" }

    /// 当前生效的 base（下游兼容用）。
    var base: String {
        switch mode {
        case .cloud: return cloudBase
        case .selfHosted: return activeServer()?.base ?? ""
        }
    }
    /// 当前生效的 token（下游兼容用）。
    var token: String {
        switch mode {
        case .cloud: return googleAuth?.token ?? ""
        case .selfHosted: return activeServer()?.token ?? ""
        }
    }

    init() {
        if let raw = defaults.string(forKey: modeKey), let m = ConnectionMode(rawValue: raw) {
            mode = m
        }
        if let data = defaults.data(forKey: serversKey),
           let list = try? JSONDecoder().decode([ServerConfig].self, from: data),
           !list.isEmpty {
            servers = list
        }
        // 自建默认空：不再预置任何服务器；老用户升级后保留原有列表。
        let saved = defaults.string(forKey: activeKey)
        if let saved, servers.contains(where: { $0.id == saved }) {
            activeServerID = saved
        }
        loadAuth()
    }

    // MARK: - 模式

    func setMode(_ m: ConnectionMode) {
        mode = m
        defaults.set(m.rawValue, forKey: modeKey)
    }

    // MARK: - 自建服务器管理

    func addServer(name: String, base: String, token: String,
                   machineCode: String? = nil, viaRelay: Bool = false) {
        let cfg = ServerConfig(id: UUID().uuidString, name: name, base: base, token: token,
                               machineCode: machineCode, viaRelay: viaRelay)
        servers.append(cfg)
        activeServerID = cfg.id   // 新加的即切换过去（豆包式：添加即连接）
        persist()
    }

    func switchServer(_ id: String) {
        guard servers.contains(where: { $0.id == id }) else { return }
        activeServerID = id
        persist()
        // v2.1 I16：切换即触发一次即时连通性探测（胶囊立即反馈，不等 30s 心跳）。
        Task { await ConnectivityService.shared.probe() }
    }

    func removeServer(_ id: String) {
        servers.removeAll { $0.id == id }
        if activeServerID == id {
            activeServerID = servers.first?.id ?? ""
        }
        // 允许删到 0 台（不再"至少保留一台"）；删空且处于自建模式时回落云道默认模式，界面自洽。
        if servers.isEmpty && mode == .selfHosted {
            setMode(.cloud)
        }
        persist()
        // 重置探测：清掉旧服务器连接状态，立即对当前 base 重探。
        ConnectivityService.shared.reset()
    }

    /// 解除机器码绑定：清除 machineCode（保留服务器条目与地址/token/转发方式），
    /// 之后该服务器按普通自建服务器使用（可改名/改地址/删除）。
    func unbindMachine(_ id: String) {
        guard let idx = servers.firstIndex(where: { $0.id == id }) else { return }
        servers[idx].machineCode = nil
        persist()
        ConnectivityService.shared.reset()
    }

    /// 更新当前活动服务器的地址/token（设置页编辑）。
    func updateActive(base: String, token: String) {
        guard let idx = servers.firstIndex(where: { $0.id == activeServerID }) else { return }
        servers[idx].base = base
        servers[idx].token = token
        persist()
    }

    // MARK: - Google 登录态（云道模式）

    private let authKey = "vhs-ios-google-auth"

    /// 登录态：租户邮箱/档位/额度/试用期 + 会话 JWT（持久化）。
    @Published var googleAuth: GoogleAuthState?

    func setGoogleLogin(_ result: GoogleLoginResult, base: String) {
        googleAuth = GoogleAuthState(email: result.email,
                                     tenant: result.tenant,
                                     tier: result.tier,
                                     quotaUsed: result.quota?.used,
                                     quotaLimit: result.quota?.limit,
                                     resetsAt: result.quota?.resetsAt,
                                     trialUntil: result.trialUntil,
                                     token: result.token,
                                     serverBase: base)
        persistAuth()
    }

    /// 用 /v1/me 结果刷新登录态（档位/额度可能变化）。
    func refreshAuth(_ me: MeResult) {
        guard var a = googleAuth else { return }
        a.tier = me.tier
        a.quotaUsed = me.quota?.used
        a.quotaLimit = me.quota?.limit
        a.resetsAt = me.quota?.resetsAt
        a.trialUntil = me.trialUntil
        googleAuth = a
        persistAuth()
    }

    func logoutGoogle() {
        googleAuth = nil
        persistAuth()
    }

    private func loadAuth() {
        guard let data = defaults.data(forKey: authKey),
              let auth = try? JSONDecoder().decode(GoogleAuthState.self, from: data) else { return }
        googleAuth = auth
    }

    private func persistAuth() {
        if let data = try? JSONEncoder().encode(googleAuth) {
            defaults.set(data, forKey: authKey)
        } else {
            defaults.removeObject(forKey: authKey)
        }
    }

    private func activeServer() -> ServerConfig? {
        servers.first { $0.id == activeServerID } ?? servers.first
    }

    /// 去掉末尾斜杠，拼出绝对 URL（path 以 / 开头）。
    func url(_ path: String) -> URL? {
        let clean = base.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        return URL(string: clean + path)
    }

    private func persist() {
        if let data = try? JSONEncoder().encode(servers) {
            defaults.set(data, forKey: serversKey)
        }
        defaults.set(activeServerID, forKey: activeKey)
        defaults.set(mode.rawValue, forKey: modeKey)
    }
}
