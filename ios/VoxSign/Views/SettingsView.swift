//
//  SettingsView.swift
//  VoxSign
//
//  豆包式设置（T3，UI 重设计 v3）：
//  - 双连接模式：「云道 · 默认」（零配置，仅 Google 登录）/「自建」（默认空列表，自加服务器）
//  - 自建添加服务器两种方式：IP 地址（内网检测 + 云端转发开关 + 保存前必检连通）/
//    机器码（云道定位机器 → 同网直连 / 异网转发 → 检测后保存）
//  - 连接状态：当前生效端点实时探测（绿/黄/灰）+ 立即重探
//  - 语音：朗读回复开关 + 语速；后台：离线队列补投
//

import SwiftUI
#if canImport(Speech)
import Speech
#endif

struct SettingsView: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var settings: SettingsStore
    #if canImport(Speech)
    @EnvironmentObject var speech: SpeechRecognizer
    #endif
    /// T2 连接感知：实时状态 + 最近错误（设置页排障不黑盒）。
    @ObservedObject var conn = ConnectivityService.shared
    /// T3 语音朗读设置。
    @ObservedObject var tts = VoiceOutputService.shared
    @Environment(\.dismiss) private var dismiss

    // P1 云道：Google 登录状态。
    @State private var googleBusy = false
    @State private var googleError = ""
    @State private var showGoogleAlert = false

    // 添加服务器弹层（方式选择 → IP / 机器码）。
    @State private var showAdd = false
    @State private var addStep: AddStep = .method
    // IP 地址表单
    @State private var ipName = ""
    @State private var ipBase = ""
    @State private var ipToken = ""
    @State private var ipViaRelay = false
    @State private var ipBusy = false
    @State private var ipError = ""
    // 机器码表单
    @State private var mcCode = ""
    @State private var mcBusy = false
    @State private var mcInfo: MachineInfo?
    @State private var mcRelay = false
    @State private var mcError = ""

    // UniFusion 组织部署条目 添加/编辑弹层
    @State private var showUniFusionAdd = false
    @State private var unifusionEditID: String?

    private enum AddStep { case method, ip, machine }

    /// 机器码粘贴净化：从粘贴文本中提取 XXXX-XXXX-XXXX 形态的机器码（忽略大小写，统一大写），丢弃其余文字。
    private static func extractedMachineCode(from text: String) -> String? {
        let pattern = #"\b[A-Za-z0-9]{4}-[A-Za-z0-9]{4}-[A-Za-z0-9]{4}\b"#
        guard let range = text.range(of: pattern, options: .regularExpression) else { return nil }
        return String(text[range]).uppercased()
    }

    var body: some View {
        NavigationStack {
            Form {
                // —— 连接模式：云道（默认）/ 自建 ——
                Section {
                    Picker("连接模式", selection: $settings.mode) {
                        Text("云道 · 默认").tag(ConnectionMode.cloud)
                        Text("自建").tag(ConnectionMode.selfHosted)
                    }
                    .pickerStyle(.segmented)
                    .onChange(of: settings.mode) { _ in
                        conn.probe()
                    }
                }

                if settings.mode == .cloud {
                    cloudSection
                } else {
                    selfHostedSection
                }

                // —— 连接状态 ——
                Section("连接状态") {
                    HStack {
                        // 云道模式未登录：显示「未登录（云道）」灰点，不再误报「已连接」。
                        // 「已连接」仅代表服务器连通（health 无认证），不代表登录态有效。
                        let signedIn = settings.mode != .cloud || settings.googleAuth != nil
                        if !signedIn {
                            Circle().fill(Color.gray)
                                .frame(width: 8, height: 8)
                            Text("未登录（云道）")
                                .font(.system(size: 14))
                                .foregroundColor(.secondary)
                            Spacer()
                        } else {
                            Circle().fill(conn.state == .online ? Color.green : (conn.state == .reconnecting ? Color.yellow : Color.gray))
                                .frame(width: 8, height: 8)
                            Text(conn.state == .online ? "已连接" : (conn.state == .reconnecting ? "正在重连…" : "离线"))
                            Spacer()
                            // UI v3：延迟小字（豆包式 12.5pt 灰字，如「延迟 42ms」）。
                            if conn.state == .online && conn.latencyMs > 0 {
                                Text("延迟 \(conn.latencyMs)ms")
                                    .font(.system(size: 12))
                                    .foregroundColor(.secondary)
                            }
                        }
                    }
                    Button("立即重探") { conn.probe() }
                    if !conn.lastError.isEmpty {
                        Text(conn.lastError).font(.system(size: 11)).foregroundColor(.secondary)
                    }
                }

                // —— 语音（豆包式 TTS）——
                Section("语音朗读") {
                    Toggle("朗读回复（TTS）", isOn: $tts.enabled)
                    HStack {
                        Text("语速").font(.system(size: 13))
                        Slider(value: $tts.rate, in: 0.4...0.6, step: 0.05)
                        Text(String(format: "%.2f", tts.rate)).font(.system(size: 11)).foregroundColor(.secondary)
                    }
                    Button("试听") { tts.speak("你好，我是 VoxSign 语音助手。") }
                }

                // —— 后台能力 ——
                #if canImport(Speech)
                Section("后台能力") {
                    Button("补投离线队列") {
                        Task { _ = await model.flushQueue() }
                    }
                    Text("离线队列 \(DeliveryQueue.shared.count) 条待投递 · 通知已授权=\(NotificationService.shared.authorized ? "是" : "否")")
                        .font(.system(size: 11)).foregroundColor(.secondary)
                }
                #endif

                Section {
                    Text("云道：默认模式，Google 登录即用。自建：添加自己的 Harness 服务器（IP 地址或机器码）。").font(.system(size: 11)).foregroundColor(.secondary)
                }

                // 版本号（每次更新递增，便于用户确认是否装到最新版）。
                Section {
                    HStack {
                        Text("版本")
                        Spacer()
                        Text(appVersionLabel())
                            .font(.system(size: 12, weight: .medium))
                            .foregroundColor(.secondary)
                    }
                }
            }
            .navigationTitle("设置")
            .toolbar {
                Button("完成") { dismiss() }
            }
            .sheet(isPresented: $showAdd) { addServerSheet }
            .sheet(isPresented: $showUniFusionAdd) {
                UniFusionEditView(mode: .add)
                    .environmentObject(settings)
            }
            .sheet(item: Binding(
                get: { unifusionEditID.map { UniFusionIdentifiableID(id: $0) } },
                set: { unifusionEditID = $0?.id }
            )) { item in
                UniFusionEditView(mode: .edit(item.id))
                    .environmentObject(settings)
            }
            .alert("Google 登录失败", isPresented: $showGoogleAlert) {
                Button("好", role: .cancel) {}
            } message: {
                Text(googleError.isEmpty ? "未知错误" : googleError)
            }
        }
    }

    // MARK: - 云道视图（零配置，仅 Google 登录）

    private var cloudSection: some View {
        Section {
            if let auth = settings.googleAuth {
                LabeledContent("账户") { Text(auth.email).font(.system(size: 13)) }
                LabeledContent("档位") { Text(tierLabel(auth.tier)).font(.system(size: 13)) }
                if let u = auth.quotaUsed, let l = auth.quotaLimit {
                    LabeledContent("今日额度") { Text("\(u) / \(l)").font(.system(size: 13)) }
                }
                if let t = auth.trialUntil, !t.isEmpty {
                    LabeledContent("体验会员") { Text("至 \(t)").font(.system(size: 13)) }
                }
                HStack {
                    Button("刷新登录态") { refreshMe() }
                    Button("退出登录", role: .destructive) { settings.logoutGoogle() }
                }
                if !googleError.isEmpty {
                    Text(googleError).font(.system(size: 11)).foregroundColor(.red)
                }
            } else {
                Button {
                    loginGoogle()
                } label: {
                    if googleBusy {
                        ProgressView().frame(maxWidth: .infinity)
                    } else {
                        // UI v3：Google 官方四色 G（自绘）+ 登录文案（豆包式居中白按钮）。
                        HStack(spacing: 8) {
                            GoogleLogo()
                            Text("使用 Google 登录")
                                .font(.system(size: 15, weight: .medium))
                                .foregroundColor(.black.opacity(0.85))
                        }
                        .frame(maxWidth: .infinity, minHeight: 50)
                        .background(Color.white)
                        .cornerRadius(10)
                        .overlay(
                            RoundedRectangle(cornerRadius: 10)
                                .stroke(Color.black.opacity(0.1), lineWidth: 0.5)
                        )
                    }
                }
                .disabled(googleBusy)
                if !googleError.isEmpty {
                    Text(googleError).font(.system(size: 11)).foregroundColor(.red)
                }
                Text("云道模式零配置：Google 一键登录，自动连接 VoxSign 云端。").font(.system(size: 11)).foregroundColor(.secondary)
            }
        } header: {
            Text("云道")
        } footer: {
            Text("无需服务器地址、无需 Token")
        }
    }

    // MARK: - 自建视图（默认空列表 + 添加）

    private var selfHostedSection: some View {
        Group {
            // —— 普通自建机器（不含 UniFusion 组织条目）——
            Section("服务器（自建）") {
                if settings.selfHostedServers.isEmpty {
                    VStack(spacing: 6) {
                        Text("还没有自建服务器")
                            .font(.system(size: 14, weight: .medium))
                            .padding(.top, 6)
                        Text("添加后即可连接 · 支持 IP 地址 / 机器码两种方式")
                            .font(.system(size: 11)).foregroundColor(.secondary)
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.bottom, 6)
                } else {
                    ForEach(settings.selfHostedServers) { srv in
                        ServerRowView(srv: srv, settings: settings, conn: conn)
                    }
                }
                Button {
                    openAddSheet()
                } label: {
                    Label("添加服务器", systemImage: "plus.circle")
                }
                .accessibilityIdentifier("vhs.selfhost.add")
            }

            // —— 独立部署（按客户/部署实例，一台客户一条目）——
            Section {
                if settings.unifusionServers.isEmpty {
                    VStack(spacing: 6) {
                        Text("尚未配置企业部署")
                            .font(.system(size: 14, weight: .medium))
                            .padding(.top, 6)
                        Text("添加客户/部署实例的私有化部署地址")
                            .font(.system(size: 11)).foregroundColor(.secondary)
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.bottom, 6)
                } else {
                    ForEach(settings.unifusionServers) { srv in
                        UniFusionRowView(srv: srv, settings: settings) {
                            unifusionEditID = srv.id
                        }
                    }
                }
                Button {
                    showUniFusionAdd = true
                } label: {
                    Label("添加企业部署地址", systemImage: "building.2")
                }
                .accessibilityIdentifier("vhs.unifusion.add")
            } header: {
                Text("独立部署")
            } footer: {
                Text("企业私有化部署入口 · 按客户/部署实例提供。")
            }

            // 当前活动自建服务器的快捷编辑（仅普通自建机器；组织条目走 UniFusionEditView）。
            if let active = settings.selfHostedServers.first(where: { $0.id == settings.activeServerID }) {
                Section("当前：\(active.name)") {
                    TextField("名称", text: Binding(
                        get: { active.name },
                        set: { v in
                            guard let i = settings.servers.firstIndex(where: { $0.id == settings.activeServerID }) else { return }
                            settings.servers[i].name = v
                        }))
                    TextField("地址 http://…", text: Binding(
                        get: { active.base },
                        set: { v in
                            guard let i = settings.servers.firstIndex(where: { $0.id == settings.activeServerID }) else { return }
                            settings.servers[i].base = v
                        }))
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                    SecureField("Bearer Token", text: Binding(
                        get: { active.token },
                        set: { v in
                            guard let i = settings.servers.firstIndex(where: { $0.id == settings.activeServerID }) else { return }
                            settings.servers[i].token = v
                        }))
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                    if active.isMachineBound || active.usesRelay {
                        Text([active.isMachineBound ? "机器码绑定" : nil,
                              active.usesRelay ? "云端转发" : nil]
                            .compactMap { $0 }.joined(separator: " · "))
                            .font(.system(size: 11)).foregroundColor(.blue)
                    }
                    HStack {
                        Button("测试连接") { model.testConnection() }
                        Button("立即重探") { conn.probe() }
                    }
                    if !model.statusLine.isEmpty {
                        Text(model.statusLine).font(.system(size: 12))
                    }
                }
            }
        }
    }

    // MARK: - 添加服务器弹层（方式选择 → IP / 机器码）

    private var addServerSheet: some View {
        NavigationStack {
            Group {
                switch addStep {
                case .method:
                    Form {
                        Section("选择添加方式") {
                            Button {
                                addStep = .ip
                                ipName = ""; ipBase = ""; ipToken = ""; ipViaRelay = false; ipError = ""
                            } label: {
                                HStack {
                                    Label("IP 地址", systemImage: "network")
                                    Spacer()
                                    Text("输入 IP + 端口 + Token").font(.caption).foregroundColor(.secondary)
                                }
                            }
                            Button {
                                addStep = .machine
                                mcCode = ""; mcInfo = nil; mcRelay = false; mcError = ""
                            } label: {
                                HStack {
                                    Label("机器码", systemImage: "number")
                                    Spacer()
                                    Text("装机机器码，自动关联").font(.caption).foregroundColor(.secondary)
                                }
                            }
                        }
                        Section {
                            Text("自建默认空列表，无任何预置服务器。").font(.system(size: 11)).foregroundColor(.secondary)
                        }
                    }
                case .ip:
                    ipAddForm
                case .machine:
                    machineAddForm
                }
            }
            .navigationTitle(addStep == .method ? "添加服务器" : (addStep == .ip ? "IP 地址" : "机器码"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { showAdd = false; addStep = .method }
                }
                if addStep != .method {
                    ToolbarItem(placement: .navigation) {
                        Button("返回") { addStep = .method }
                    }
                }
            }
        }
        .presentationDetents([.medium, .large])
    }

    /// IP 地址方式：内网检测 → 云端转发开关 → 保存前必检连通。
    private var ipAddForm: some View {
        Form {
            Section("服务器") {
                TextField("名称（如：办公室 Mac）", text: $ipName)
                TextField("http://192.168.x.x:8897", text: $ipBase)
                    .keyboardType(.URL)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                SecureField("Bearer Token（可选）", text: $ipToken)
            }
            if isLanIP(ipBase) {
                Section {
                    Toggle("通过云端转发访问", isOn: $ipViaRelay)
                } footer: {
                    Text("检测到内网地址：外网手机不可直达，由云道中转（需内网端开启转发）。")
                }
            }
            Section {
                Button {
                    saveIP()
                } label: {
                    if ipBusy {
                        HStack {
                            ProgressView().frame(width: 16, height: 16)
                            Text("正在检测连接…").frame(maxWidth: .infinity)
                        }
                    } else {
                        Text("保存并检测连接").frame(maxWidth: .infinity)
                    }
                }
                .disabled(ipBusy || ipBase.isEmpty)
                if !ipError.isEmpty {
                    Text(ipError).font(.system(size: 11)).foregroundColor(.red)
                }
            }
        }
    }

    /// 机器码方式：查询 → 云道定位 → 直连/转发判定 → 检测后保存。
    private var machineAddForm: some View {
        Form {
            Section("机器码") {
                TextField("AB12-CD34-EF56", text: $mcCode)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.characters)
                    .autocapitalization(.allCharacters)
                    // 粘贴净化：从粘贴文本提取机器码（XXXX-XXXX-XXXX），丢弃其余文字，统一大写。
                    .onChange(of: mcCode) { newValue in
                        if let code = Self.extractedMachineCode(from: newValue), code != newValue {
                            mcCode = code
                        }
                    }
                Text("输入 Harness 装机生成的机器码，云道自动定位该机器。")
                    .font(.system(size: 11)).foregroundColor(.secondary)
            }
            if let info = mcInfo {
                Section {
                    LabeledContent("机器") { Text(info.name).font(.system(size: 13)) }
                    LabeledContent("地址") { Text(info.base).font(.system(size: 13)) }
                    LabeledContent("连接方式") {
                        Text(mcRelay ? "云道转发（异网）" : "同网直连")
                            .font(.system(size: 13))
                            .foregroundColor(mcRelay ? .blue : .green)
                    }
                }
                Section {
                    Button {
                        saveMachine(info)
                    } label: {
                        if mcBusy {
                            HStack {
                                ProgressView().frame(width: 16, height: 16)
                                Text("正在检测…").frame(maxWidth: .infinity)
                            }
                        } else {
                            Text("保存").frame(maxWidth: .infinity)
                        }
                    }
                    .disabled(mcBusy)
                }
            } else {
                Section {
                    Button {
                        lookupMachine()
                    } label: {
                        if mcBusy {
                            HStack {
                                ProgressView().frame(width: 16, height: 16)
                                Text("正在查询机器码…").frame(maxWidth: .infinity)
                            }
                        } else {
                            Text("查询并绑定").frame(maxWidth: .infinity)
                        }
                    }
                    .disabled(mcBusy || mcCode.isEmpty)
                }
            }
            if !mcError.isEmpty {
                Text(mcError).font(.system(size: 11)).foregroundColor(.red)
            }
        }
    }

    // MARK: - 动作

    /// 保存 IP 方式：先检测（同网直连；开启转发则云道可用也算通），通过才保存。
    private func saveIP() {
        ipBusy = true
        ipError = ""
        Task {
            let directOK = await APIClient.shared.healthCheck(base: ipBase)
            let relayOK = ipViaRelay ? await APIClient.shared.healthCheck(base: settings.cloudBase) : false
            ipBusy = false
            guard directOK || relayOK else {
                ipError = ipViaRelay
                    ? "无法连接：内网地址与云道转发均不可达（确认服务器在线、内网端已开启转发）"
                    : "无法连接该地址（确认服务器在线且地址正确）"
                return
            }
            settings.addServer(name: ipName.isEmpty ? "服务器" : ipName,
                               base: ipBase,
                               token: ipToken,
                               viaRelay: ipViaRelay)
            conn.probe()
            showAdd = false
            addStep = .method
        }
    }

    /// 机器码查询：云道定位 → 直连探测 → 不通则标记异网转发。
    private func lookupMachine() {
        mcBusy = true
        mcError = ""
        mcInfo = nil
        Task {
            do {
                let info = try await APIClient.shared.lookupMachine(code: mcCode)
                let direct = await APIClient.shared.healthCheck(base: info.base)
                mcInfo = info
                mcRelay = !direct
                if !direct {
                    mcError = "异网：将保存为「云道转发」模式，按需中转（用完即断）。"
                }
            } catch {
                mcError = error.localizedDescription
            }
            mcBusy = false
        }
    }

    /// 云道转发地址：https://voxsign.ai/relay/<机器码>（云端按机器码路由到该机反向连接；
    /// 端口只按服务分不按机器分，转发请求全部复用 443）。
    private func relayBase(code: String) -> String {
        let cloud = settings.cloudBase.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        return "\(cloud)/relay/\(code)"
    }

    /// 保存机器码方式：检测（直连 or 转发通道）通过才保存。
    private func saveMachine(_ info: MachineInfo) {
        mcBusy = true
        mcError = ""
        Task {
            let saveBase = mcRelay ? relayBase(code: mcCode) : info.base
            let ok = await APIClient.shared.healthCheck(base: saveBase)
            mcBusy = false
            guard ok else {
                mcError = "无法连接（确认机器在线\(mcRelay ? "、云道转发可用" : "")）"
                return
            }
            settings.addServer(name: info.name,
                               base: saveBase,
                               token: info.token,
                               machineCode: mcCode,
                               viaRelay: mcRelay)
            conn.probe()
            showAdd = false
            addStep = .method
        }
    }

    /// 发起 Google 授权（PKCE）→ 换会话 JWT → 云道登录。
    private func loginGoogle() {
        googleBusy = true
        googleError = ""
        Task {
            defer { googleBusy = false }
            do {
                let (code, verifier) = try await AuthService.shared.authorize(base: settings.base)
                let idToken = try await AuthService.shared.exchangeIDToken(code: code, verifier: verifier)
                let result = try await APIClient.shared.loginGoogleIDToken(idToken)
                settings.setGoogleLogin(result, base: settings.base)
                conn.probe()
                // 云道登录成功后拉取用户所属组织目录（UniFusion 自动出现在机器列表）。
                Task { await settings.refreshUniFusionOrgs() }
            } catch {
                googleError = error.localizedDescription
                showGoogleAlert = true
            }
        }
    }

    /// 刷新登录态（GET /v1/me）。
    /// 401 = 会话失效/未登录：自动引导 Google 重新登录，避免红字卡死。
    private func refreshMe() {
        googleError = ""
        Task {
            do {
                let me = try await APIClient.shared.me()
                settings.refreshAuth(me)
            } catch let APIError.http(code, body) {
                if code == 401 {
                    googleError = "登录态已失效，正在引导重新登录…"
                    loginGoogle()
                } else {
                    googleError = "HTTP \(code): \(body)"
                }
            } catch {
                googleError = error.localizedDescription
            }
        }
    }

    private func openAddSheet() {
        addStep = .method
        showAdd = true
    }

    private func tierLabel(_ tier: String) -> String {
        switch tier {
        case "free": return "免费（每日额度内）"
        case "prime": return "Prime"
        case "enterprise": return "Enterprise"
        default: return tier
        }
    }

    private func isLanIP(_ s: String) -> Bool {
        s.range(of: #"https?://(192\.168\.|10\.|172\.(1[6-9]|2\d|3[01])\.)"#,
                options: .regularExpression) != nil
    }

    /// 版本号：Info.plist CFBundleShortVersionString + CFBundleVersion。
    /// 每次发布递增 build，用户可在设置页最底部确认是否已更新到最新版。
    private func appVersionLabel() -> String {
        let ver = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "?"
        let build = Bundle.main.infoDictionary?["CFBundleVersion"] as? String ?? "?"
        return "\(ver) (build \(build))"
    }
}


// MARK: - Google 官方四色 G（UI v3 自绘，非图片）

/// 豆包式登录按钮用的 Google 品牌标识：四色环 + 白色 G 字形。
struct GoogleLogo: View {
    var body: some View {
        ZStack {
            // 蓝底圆环
            Circle().fill(Color(red: 0.259, green: 0.522, blue: 0.957)) // #4285F4
            // 黄（左上 90° 扇形）
            sector(start: .degrees(180), end: .degrees(270))
                .fill(Color(red: 0.988, green: 0.737, blue: 0.031))     // #FBBC05
            // 绿（左下 90° 扇形）
            sector(start: .degrees(90), end: .degrees(180))
                .fill(Color(red: 0.204, green: 0.659, blue: 0.325))     // #34A853
            // 红（右下 90° 扇形）
            sector(start: .degrees(0), end: .degrees(90))
                .fill(Color(red: 0.918, green: 0.263, blue: 0.208))     // #EA4335
            // 白色 G 字形（semibold，视觉对齐官方 G 的位置）
            Text("G")
                .font(.system(size: 17, weight: .heavy))
                .foregroundColor(.white)
                .offset(x: -1, y: 0)
        }
        .frame(width: 20, height: 20)
    }

    private func sector(start: Angle, end: Angle) -> some Shape {
        // 圆心在 (10,10)，半径 10，画 90° 扇形（SwiftUI Path 角度从 3 点方向起、顺时针为正）。
        Path { p in
            p.move(to: CGPoint(x: 10, y: 10))
            p.addArc(center: CGPoint(x: 10, y: 10),
                     radius: 10,
                     startAngle: start,
                     endAngle: end,
                     clockwise: false)
            p.closeSubpath()
        }
    }
}


// MARK: - 服务器行（拆分自 ForEach，规避 Swift 类型检查超时）

private struct ServerRowView: View {
    let srv: ServerConfig
    @ObservedObject var settings: SettingsStore
    @ObservedObject var conn: ConnectivityService

    var body: some View {
        HStack {
            Image(systemName: srv.id == settings.activeServerID ? "checkmark.circle.fill" : "circle")
                .foregroundColor(srv.id == settings.activeServerID ? .green : .gray)
            VStack(alignment: .leading, spacing: 2) {
                Text(srv.name).font(.system(size: 14, weight: .medium))
                Text(srv.base).font(.system(size: 11)).foregroundColor(.secondary).lineLimit(1)
                // UI v3：连接方式 chips——机器码 / 同网直连 / 云端转发（豆包式小标签）。
                if srv.isMachineBound || srv.usesRelay {
                    HStack(spacing: 4) {
                        if srv.isMachineBound {
                            chip("机器码", .blue)
                        }
                        chip(srv.usesRelay ? "云端转发" : "同网直连", srv.usesRelay ? .blue : .green)
                    }
                }
            }
            Spacer()
            if srv.id == settings.activeServerID {
                Text("当前").font(.system(size: 11)).foregroundColor(.green)
            }
        }
        .contentShape(Rectangle())
        .onTapGesture { settings.switchServer(srv.id); conn.probe() }
        .swipeActions {
            // 解除机器码绑定：清除 machineCode，服务器条目保留（按普通自建使用）。
            if srv.isMachineBound {
                Button("解绑") { settings.unbindMachine(srv.id) }
            }
            Button("删除", role: .destructive) { settings.removeServer(srv.id) }
        }
    }

    private func chip(_ text: String, _ color: Color) -> some View {
        Text(text)
            .font(.system(size: 10, weight: .medium))
            .padding(.horizontal, 5).padding(.vertical, 1.5)
            .background(color.opacity(0.12))
            .foregroundColor(color)
            .cornerRadius(4)
    }
}


// MARK: - UniFusion 组织部署行（设置页）

/// 一个组织一台的 UniFusion 条目：显示组织名/地址 + 「当前」标记 + 编辑入口。
struct UniFusionRowView: View {
    let srv: ServerConfig
    @ObservedObject var settings: SettingsStore
    /// 点「编辑」回调（由父视图弹 UniFusionEditView）。
    let onEdit: () -> Void

    var body: some View {
        HStack {
            Image(systemName: "building.2")
                .foregroundColor(.purple)
            VStack(alignment: .leading, spacing: 2) {
                Text(srv.name).font(.system(size: 14, weight: .medium))
                Text(srv.base).font(.system(size: 11)).foregroundColor(.secondary).lineLimit(1)
                HStack(spacing: 4) {
                    chip(srv.orgName ?? "组织", .purple)
                    chip(srv.usesRelay ? "云道转发" : "直连", srv.usesRelay ? .blue : .green)
                }
            }
            Spacer()
            if srv.id == settings.activeServerID {
                Text("当前").font(.system(size: 11)).foregroundColor(.green)
            }
            Button {
                onEdit()
            } label: {
                Label("编辑", systemImage: "pencil")
            }
            .accessibilityIdentifier("vhs.unifusion.edit.\(srv.orgId ?? srv.id)")
        }
        .swipeActions {
            Button(role: .destructive) {
                settings.removeServer(srv.id)
            } label: {
                Label("删除", systemImage: "trash")
            }
            .accessibilityIdentifier("vhs.unifusion.delete.\(srv.orgId ?? srv.id)")
        }
        .accessibilityIdentifier("vhs.unifusion.row.\(srv.orgId ?? srv.id)")
    }

    private func chip(_ text: String, _ color: Color) -> some View {
        Text(text)
            .font(.system(size: 10, weight: .medium))
            .padding(.horizontal, 5).padding(.vertical, 1.5)
            .background(color.opacity(0.12))
            .foregroundColor(color)
            .cornerRadius(4)
    }
}

/// 包裹 String 以适配 sheet(item:) 的 Identifiable。
private struct UniFusionIdentifiableID: Identifiable {
    let id: String
}
