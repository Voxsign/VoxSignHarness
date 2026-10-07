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
//  「独立部署」（v3.x）：企业私有化部署按「客户/部署实例」呈现——
//  分区名=「独立部署」；每属于一个客户/组织，机器列表就多一台该客户的部署条目
//  （条目名=客户名本身，不加品牌前缀，如「UniFusion」「PeterZou」平级排列），
//  条目自带 base/token/直连或云道转发。
//  实现上与自建机器共用 ServerConfig，仅以 orgId/orgName 标记区分，
//  add/switch/remove/连通探测全部复用既有 servers 机制，base/token 零改动。
//
//  组织目录来源：后端 /v1/orgs（唯一权威）。后端返回空 / 零权限 / 未登录401 / 拉取失败
//  → 一律视为「当前用户无可用独立部署」，界面落到空态文案「暂无可用独立部署」，
//  绝不崩溃、不再内置任何硬编码占位客户。手动添加降级为空态内的兜底入口（manual-* 前缀）。
//  orgId/orgName 语义=客户/部署实例，字段名不改。

import Foundation

/// 连接模式：云道（默认）/ 自建（含 UniFusion 组织条目——二者都走 activeServer 取 base/token）。
enum ConnectionMode: String, Codable, CaseIterable {
    case cloud        // 云道 · 默认：Google 登录即用，零配置
    case selfHosted   // 自建 + UniFusion 组织条目：自己/组织添加服务器
}

/// 一台可连接的 Harness 服务器（自建机器 / UniFusion 组织条目共用）。
struct ServerConfig: Identifiable, Codable, Equatable {
    var id: String
    var name: String
    var base: String
    var token: String
    /// 机器码绑定（非 nil = 通过机器码添加，地址由云道定位带入）。
    var machineCode: String?
    /// 云端转发（内网服务器经云道中转；nil = 直连）。
    var viaRelay: Bool?
    /// UniFusion 组织条目标记：非 nil = 该条目是某组织的私有化部署入口；
    /// nil = 普通自建机器。老版本数据无这两个字段 → 解码为 nil（向后兼容，老用户零影响）。
    var orgId: String?
    var orgName: String?

    var isMachineBound: Bool { machineCode != nil }
    var usesRelay: Bool { viaRelay ?? false }
    /// 是否为 UniFusion 组织条目（机器列表据此分区渲染）。
    var isUniFusion: Bool { orgId != nil }
}

/// 一个用户所属组织的 UniFusion 部署目录项（占位数据源）。
/// 机器切换面板按组织归属渲染：属于几个组织就出现几台 UniFusion 机器。
struct UniFusionOrg: Identifiable, Equatable {
    let orgId: String
    let orgName: String
    /// 该组织私有化部署前端的默认地址（添加时预填，用户可改）。
    let suggestedBase: String
    var id: String { orgId }
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

    private let defaults: UserDefaults
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

    /// 从后端 /v1/orgs 拉到的组织目录（权威来源；空 = 未拉到/无归属）。
    /// UniFusion 区按此渲染并自动同步为机器条目；手动添加兜底始终可用。
    @Published var backendOrgs: [UniFusionOrg] = []

    /// 云道地址（正式域名 voxsign.ai，nginx 转发 /v1 → 云端 harness 8898）。
    var cloudBase: String { "https://voxsign.ai" }

    // MARK: - 独立部署（组织目录：后端 /v1/orgs 唯一权威）

    /// 用户所属组织的部署目录（唯一权威 = 后端 /v1/orgs）。
    /// 登录成功 / 切换自建机器后由 refreshUniFusionOrgs() 填充；
    /// 拉取失败 / 401 / 空列表 → 返回空数组，界面落到空态文案，保留手动添加兜底。
    /// （build 13 起移除硬编码占位客户目录——任何用户不再先看到 UniFusion/PeterZou 占位。）
    var availableUniFusionOrgs: [UniFusionOrg] {
        backendOrgs
    }

    /// 已是机器条目的组织 id（避免「添加组织」重复添加同一组织）。
    var existingUniFusionOrgIDs: Set<String> {
        Set(servers.compactMap { $0.orgId })
    }

    /// 尚未添加的可选组织目录项（「添加组织部署」下拉用）。
    var availableOrgsToAdd: [UniFusionOrg] {
        availableUniFusionOrgs.filter { !existingUniFusionOrgIDs.contains($0.orgId) }
    }

    /// 当前「独立部署」条目（机器切换面板对应分区渲染用）。
    var unifusionServers: [ServerConfig] {
        servers.filter { $0.isUniFusion }
    }

    /// 普通自建机器条目（不含 UniFusion 组织条目）。
    var selfHostedServers: [ServerConfig] {
        servers.filter { !$0.isUniFusion }
    }

    /// 当前活动服务器（自建模式；云道 / 无选中 → nil）。
    var activeServerConfig: ServerConfig? { activeServer() }

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

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
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
        // 独立部署条目只来自后端 refresh upsert + 用户手动添加；全新安装不播种占位行
        // （否则任何用户都会先看到 fallback 占位，破坏按客户/部署实例的隔离）。
        loadAuth()
    }

    // MARK: - 独立部署（组织目录后端联动）

    /// 从活动 base 拉取组织目录（GET /v1/orgs）并按 orgId 同步机器条目（后端权威）：
    /// - 成功且返回非空：已存在条目 → 更新 orgName/base/viaRelay（保留用户已填 token）；
    ///   新组织 → 自动新增；后端不再返回的组织条目 → 删除（用户被移出组织/换账号时条目消失）。
    ///   manual-* 手动条目不受后端清理影响。
    /// - 空列表（零权限 / 未登录401 / 端点未支持404 / 拉取失败）：同样按后端权威处理——
    ///   liveIDs 为空 → 清除全部非 manual-* 组织条目，界面落到空态「暂无可用独立部署」。
    ///   getOrgs() 内部绝不抛错，故此处永不因网络问题崩溃。
    func refreshUniFusionOrgs() async {
        let entries = await APIClient.shared.getOrgs()
        backendOrgs = entries.map {
            UniFusionOrg(orgId: $0.orgId, orgName: $0.orgName, suggestedBase: $0.base)
        }
        // 空列表时 liveIDs=空集：清理掉所有非 manual-* 的组织条目（手动兜底保留）。
        let liveIDs = Set(entries.map { $0.orgId })
        for e in entries {
            if let idx = servers.firstIndex(where: { $0.orgId == e.orgId }) {
                servers[idx].orgName = e.orgName
                servers[idx].name = e.orgName
                servers[idx].base = e.base
                servers[idx].viaRelay = e.viaRelay
            } else {
                servers.append(ServerConfig(id: UUID().uuidString,
                                            name: e.orgName, base: e.base, token: "",
                                            machineCode: nil, viaRelay: e.viaRelay ?? false,
                                            orgId: e.orgId, orgName: e.orgName))
            }
        }
        // 后端权威清理：删除后端不再返回的组织条目（保留 manual-* 手动条目）。
        servers.removeAll { srv in
            guard let oid = srv.orgId else { return false }
            return !oid.hasPrefix("manual-") && !liveIDs.contains(oid)
        }
        persist()
    }

    // MARK: - 模式

    func setMode(_ m: ConnectionMode) {
        mode = m
        defaults.set(m.rawValue, forKey: modeKey)
        // V6.3 切换即重探：状态点与机器名必须跟随"实际连接目标"。
        // 否则会出现"设置点了自建、实际还连云端，名字却已变"的错位。
        ConnectivityService.shared.reset()
        // 切换连接目标后，重新拉取该 base 下用户所属组织目录。
        Task { await refreshUniFusionOrgs() }
    }

    // MARK: - 自建服务器管理

    func addServer(name: String, base: String, token: String,
                   machineCode: String? = nil, viaRelay: Bool = false,
                   orgId: String? = nil, orgName: String? = nil) {
        let cfg = ServerConfig(id: UUID().uuidString, name: name, base: base, token: token,
                               machineCode: machineCode, viaRelay: viaRelay,
                               orgId: orgId, orgName: orgName)
        servers.append(cfg)
        activeServerID = cfg.id   // 新加的即切换过去（豆包式：添加即连接）
        mode = .selfHosted        // UniFusion 条目也走自建通道取 base/token
        persist()
        ConnectivityService.shared.reset()
        Task { await refreshUniFusionOrgs() }
    }

    /// 更新某个 UniFusion 组织条目的地址/token/转发方式（设置页编辑表单保存用）。
    func updateUniFusion(id: String, name: String, base: String, token: String, viaRelay: Bool) {
        guard let idx = servers.firstIndex(where: { $0.id == id }) else { return }
        servers[idx].name = name
        servers[idx].base = base
        servers[idx].token = token
        servers[idx].viaRelay = viaRelay
        persist()
        ConnectivityService.shared.reset()
    }

    func switchServer(_ id: String) {
        guard servers.contains(where: { $0.id == id }) else { return }
        activeServerID = id
        persist()
        // v2.1 I16：切换即触发一次即时连通性探测（胶囊立即反馈，不等 30s 心跳）。
        Task { await ConnectivityService.shared.probe() }
        Task { await refreshUniFusionOrgs() }
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
