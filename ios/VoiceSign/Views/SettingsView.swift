//
//  SettingsView.swift
//  VoiceSign
//
//  豆包式设置（T3）：
//  - 服务器管理：多台（我的 Mac / 云 / 备机）列表 + 添加 + 切换 + 删除（豆包"连哪台设备"）
//  - 连接状态：当前服务器实时探测（绿/黄/灰）+ 立即重探
//  - 语音：朗读回复开关 + 语速
//  - 后台：常听模式开关 + 离线队列补投
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

    @State private var editingServer: ServerConfig?
    @State private var showAdd = false
    @State private var newName = ""
    @State private var newBase = ""
    @State private var newToken = ""
    // P1 云端模式：Google 登录状态。
    @State private var googleBusy = false
    @State private var googleError = ""

    var body: some View {
        NavigationStack {
            Form {
                // —— 服务器管理（豆包式：连哪台电脑/连云，多台可切换）——
                Section("服务器（可多台切换）") {
                    ForEach(settings.servers) { srv in
                        ServerRowView(srv: srv, settings: settings, conn: conn)
                    }
                    Button {
                        showAdd = true
                        newName = ""; newBase = ""; newToken = ""
                    } label: {
                        Label("添加服务器", systemImage: "plus.circle")
                    }
                }

                // —— Google 登录（云端模式：租户 = 谷歌账户）——
                Section("Google 登录") {
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
                                Label("使用 Google 登录", systemImage: "person.crop.circle.badge.checkmark")
                                    .frame(maxWidth: .infinity)
                            }
                        }
                        .disabled(googleBusy)
                        if !googleError.isEmpty {
                            Text(googleError).font(.system(size: 11)).foregroundColor(.red)
                        }
                        Text("登录后自动写入当前服务器的 Bearer token；云端地址需已指向 VoxSign 云端 harness。")
                            .font(.system(size: 11)).foregroundColor(.secondary)
                    }
                }

                // —— 当前服务器编辑 ——
                if let active = settings.servers.first(where: { $0.id == settings.activeServerID }) {
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
                        HStack {
                            Button("测试连接") { model.testConnection() }
                            Button("立即重探") { conn.probe() }
                        }
                        if !model.statusLine.isEmpty {
                            Text(model.statusLine).font(.system(size: 12))
                        }
                    }
                }

                // —— 连接状态 ——
                Section("连接状态") {
                    HStack {
                        Circle().fill(conn.state == .online ? Color.green : (conn.state == .reconnecting ? Color.yellow : Color.gray))
                            .frame(width: 8, height: 8)
                        Text(conn.state == .online ? "已连接" : (conn.state == .reconnecting ? "正在重连…" : "离线"))
                        Spacer()
                    }
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
                    Button("试听") { tts.speak("你好，我是 VoiceSign 语音助手。") }
                }

                // —— 后台能力 ——
                #if canImport(Speech)
                Section("后台能力") {
                    // v2.4：本地识别全部移除，语音走 ASR 服务器校准（按住说话，无常听模式）。
                    Button("补投离线队列") {
                        Task { _ = await model.flushQueue() }
                    }
                    Text("离线队列 \(DeliveryQueue.shared.count) 条待投递 · 通知已授权=\(NotificationService.shared.authorized ? "是" : "否")")
                        .font(.system(size: 11)).foregroundColor(.secondary)
                }
                #endif

                Section {
                    Text("默认按住说话，说完松手自动执行；可点左侧键盘图标打字。Harness 跑在电脑上，iPhone 同 Wi-Fi 即可连接。").font(.system(size: 11)).foregroundColor(.secondary)
                }
            }
            .navigationTitle("设置")
            .toolbar {
                Button("完成") { dismiss() }
            }
            .sheet(isPresented: $showAdd) {
                addServerSheet
            }
        }
    }

    // MARK: - Google 登录（云端模式）

    /// 发起 Google 授权（PKCE）→ 换会话 JWT → 写入当前服务器。
    private func loginGoogle() {
        googleBusy = true
        googleError = ""
        Task {
            defer { googleBusy = false }
            do {
                let (code, verifier) = try await AuthService.shared.authorize(base: settings.base)
                let result = try await APIClient.shared.loginGoogle(code: code, verifier: verifier)
                settings.setGoogleLogin(result, base: settings.base)
                conn.probe()
            } catch {
                googleError = error.localizedDescription
            }
        }
    }

    /// 刷新登录态（GET /v1/me）。
    private func refreshMe() {
        googleError = ""
        Task {
            do {
                let me = try await APIClient.shared.me()
                settings.refreshAuth(me)
            } catch {
                googleError = error.localizedDescription
            }
        }
    }

    private func tierLabel(_ tier: String) -> String {
        switch tier {
        case "free": return "免费（每日额度内）"
        case "prime": return "Prime"
        case "enterprise": return "Enterprise"
        default: return tier
        }
    }

    /// 添加服务器表单（豆包式：给新设备起名 + 地址 + token）。
    private var addServerSheet: some View {        NavigationStack {
            Form {
                Section("新服务器") {
                    TextField("名称（如：我的 Mac / 云服务器）", text: $newName)
                    TextField("地址 http://192.168.x.x:8897", text: $newBase)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                    SecureField("Bearer Token（可选）", text: $newToken)
                }
            }
            .navigationTitle("添加服务器")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { showAdd = false }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("添加并连接") {
                        let base = newBase.trimmingCharacters(in: CharacterSet(charactersIn: " "))
                        if !base.isEmpty {
                            settings.addServer(name: newName.isEmpty ? "服务器" : newName,
                                               base: base,
                                               token: newToken)
                            conn.probe()
                        }
                        showAdd = false
                    }
                }
            }
        }
        .presentationDetents([.height(320)])
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
            }
            Spacer()
            if srv.id == settings.activeServerID {
                Text("当前").font(.system(size: 11)).foregroundColor(.green)
            }
        }
        .contentShape(Rectangle())
        .onTapGesture { settings.switchServer(srv.id); conn.probe() }
        .swipeActions {
            if settings.servers.count > 1 {
                Button("删除", role: .destructive) { settings.removeServer(srv.id) }
            }
        }
    }
}
