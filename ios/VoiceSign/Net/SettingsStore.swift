//
//  SettingsStore.swift
//  VoiceSign
//
//  豆包式设置（T3）：多服务器管理——可保存多台"我的电脑/云服务器"，
//  随时切换；当前活动服务器决定 base/token。等价豆包"连接哪台设备/云"。
//  下游（APIClient/SSEClient）继续用 base/token 计算属性，无需改动。
//

import Foundation

/// 一台可连接的 Harness 服务器（电脑或云）。
struct ServerConfig: Identifiable, Codable, Equatable {
    var id: String
    var name: String
    var base: String
    var token: String
}

/// Google 登录态（云端模式：租户 = Google sub）。
struct GoogleAuthState: Codable, Equatable {
    var email: String
    var tenant: String
    var tier: String
    var quotaUsed: Int?
    var quotaLimit: Int?
    var resetsAt: String?
    var trialUntil: String?
    var serverBase: String
}

/// 观测式设置仓库：App 启动时读 UserDefaults，保存时回写。
final class SettingsStore: ObservableObject {
    static let shared = SettingsStore()

    private let defaults = UserDefaults.standard
    private let serversKey = "vhs-ios-servers"
    private let activeKey = "vhs-ios-active"

    /// 服务器列表（多台：局域网电脑 / 云 / 备机），持久化。
    /// 注：不挂 didSet（init 阶段赋值会触发 didSet 且此时 self 未完整初始化会编译报错），
    /// 持久化统一在变更方法里显式 persist()。
    @Published var servers: [ServerConfig] = []
    /// 当前活动服务器 id，持久化。
    @Published var activeServerID: String = ""

    /// 当前活动服务器的 base（下游兼容用）。
    var base: String { activeServer()?.base ?? "" }
    /// 当前活动服务器的 token（下游兼容用）。
    var token: String { activeServer()?.token ?? "" }

    init() {
        if let data = defaults.data(forKey: serversKey),
           let list = try? JSONDecoder().decode([ServerConfig].self, from: data),
           !list.isEmpty {
            servers = list
        } else {
            // 首次启动：预置三台（豆包式"可连多台"）——
            // 186:8897 = 本机 Mac 本地后台（默认连接）；186:8898 = 本机云端模拟（Google 登录，token 登录后自动写入）；
            // 201:8897 = 主服务器（备用）。
            servers = [
                ServerConfig(id: UUID().uuidString,
                             name: "我的 Mac（本地）",
                             base: "http://192.168.8.186:8897",
                             token: "m7-token"),
                ServerConfig(id: UUID().uuidString,
                             name: "云端模拟（Google）",
                             base: "http://192.168.8.186:8898",
                             token: ""),
                ServerConfig(id: UUID().uuidString,
                             name: "主服务器",
                             base: "http://192.168.8.201:8897",
                             token: "m7-token")
            ]
        }
        let saved = defaults.string(forKey: activeKey)
        if let saved, servers.contains(where: { $0.id == saved }) {
            activeServerID = saved
        } else {
            activeServerID = servers[0].id
        }
        loadAuth()
    }

    // MARK: - 服务器管理

    func addServer(name: String, base: String, token: String) {
        let cfg = ServerConfig(id: UUID().uuidString, name: name, base: base, token: token)
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
        guard servers.count > 1 else { return }   // 至少保留一台
        servers.removeAll { $0.id == id }
        if activeServerID == id {
            activeServerID = servers[0].id
        }
        persist()
    }

    /// 更新当前活动服务器的地址/token（设置页编辑）。
    func updateActive(base: String, token: String) {
        guard let idx = servers.firstIndex(where: { $0.id == activeServerID }) else { return }
        servers[idx].base = base
        servers[idx].token = token
        persist()
    }

    // MARK: - Google 登录态（云端模式）

    private let authKey = "vhs-ios-google-auth"

    /// 登录态：租户邮箱/档位/额度/试用期（持久化；token 存于对应服务器 token 字段）。
    @Published var googleAuth: GoogleAuthState?

    func setGoogleLogin(_ result: GoogleLoginResult, base: String) {
        updateActive(base: base, token: result.token)
        googleAuth = GoogleAuthState(email: result.email,
                                     tenant: result.tenant,
                                     tier: result.tier,
                                     quotaUsed: result.quota?.used,
                                     quotaLimit: result.quota?.limit,
                                     resetsAt: result.quota?.resetsAt,
                                     trialUntil: result.trialUntil,
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
        if let idx = servers.firstIndex(where: { $0.id == activeServerID }) {
            servers[idx].token = ""
        }
        googleAuth = nil
        persist()
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
    }
}
