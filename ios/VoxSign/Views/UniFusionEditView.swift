//
//  UniFusionEditView.swift
//  VoxSign
//
//  独立部署条目「添加 / 编辑」表单（企业私有化部署，按客户/部署实例）：
//  - 添加：从目录选一个客户 → 预填名称/地址 → 补 Token → 保存前必检连通
//  - 编辑：改已有条目的名称 / 地址 / Token / 直连或云道转发
//  - 保存并检测连接：复用 APIClient.healthCheck（直连；开启云道转发则同时探 cloudBase）
//  - 失败给中文报错文案，绝不静默崩溃；token 不入日志/注释。
//
//  注：目录来自 SettingsStore.availableUniFusionOrgs（后端 /v1/orgs 权威，失败用内置占位兜底）。

import SwiftUI

struct UniFusionEditView: View {
    @EnvironmentObject var settings: SettingsStore
    @Environment(\.dismiss) private var dismiss

    /// .add 新建组织条目；.edit(id) 编辑已有 ServerConfig。
    enum Mode {
        case add
        case edit(String)
    }
    let mode: Mode

    // 表单字段
    @State private var orgID: String = ""
    @State private var orgName: String = ""
    @State private var name: String = ""
    @State private var base: String = ""
    @State private var token: String = ""
    @State private var viaRelay: Bool = false
    @State private var busy = false
    @State private var error: String = ""

    private var isEdit: Bool {
        if case .edit = mode { return true }
        return false
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    if case .add = mode, !settings.availableOrgsToAdd.isEmpty {
                        Picker("组织", selection: $orgID) {
                            ForEach(settings.availableOrgsToAdd) { org in
                                Text(org.orgName).tag(org.orgId)
                            }
                        }
                        .onChange(of: orgID) { _ in applyOrgPrefill() }
                        .accessibilityIdentifier("vhs.unifusion.form.orgpicker")
                    }
                    TextField("名称", text: $name)
                        .accessibilityIdentifier("vhs.unifusion.form.name")
                    TextField("企业地址 https://…", text: $base)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                        .accessibilityIdentifier("vhs.unifusion.form.base")
                    SecureField("Token（企业分配）", text: $token)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                        .accessibilityIdentifier("vhs.unifusion.form.token")
                    if case .add = mode, settings.availableOrgsToAdd.isEmpty {
                        Text("未从后端获取到组织目录，可手动填写企业部署地址。")
                            .font(.system(size: 11)).foregroundColor(.secondary)
                    }
                } header: {
                    Text("组织部署")
                }
                Section {
                    Toggle("经云道转发访问", isOn: $viaRelay)
                        .accessibilityIdentifier("vhs.unifusion.form.relay")
                } footer: {
                    Text("开启后经 VoxSign 云端中转到企业部署；直连则手机直接访问企业地址。")
                }
                Section {
                    Button {
                        save()
                    } label: {
                        if busy {
                            HStack {
                                ProgressView().frame(width: 16, height: 16)
                                Text("正在检测连接…").frame(maxWidth: .infinity)
                            }
                        } else {
                            Text("保存并检测连接").frame(maxWidth: .infinity)
                        }
                    }
                    .disabled(busy || base.isEmpty)
                    .accessibilityIdentifier("vhs.unifusion.form.save")
                    if !error.isEmpty {
                        Text(error)
                            .font(.system(size: 11))
                            .foregroundColor(.red)
                            .accessibilityIdentifier("vhs.unifusion.form.error")
                    }
                }
                if case .edit = mode {
                    Section {
                        Button(role: .destructive) {
                            if case .edit(let id) = mode {
                                settings.removeServer(id)
                            }
                            dismiss()
                        } label: {
                            Text("删除此组织部署").frame(maxWidth: .infinity)
                        }
                        .accessibilityIdentifier("vhs.unifusion.form.delete")
                    }
                }
            }
            .navigationTitle(isEdit ? "编辑组织部署" : "添加组织部署")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { dismiss() }
                }
            }
            .onAppear(perform: setup)
        }
        .presentationDetents([.medium, .large])
    }

    private func setup() {
        switch mode {
        case .add:
            if let first = settings.availableOrgsToAdd.first {
                orgID = first.orgId
                applyOrgPrefill()
            }
        case .edit(let id):
            guard let srv = settings.servers.first(where: { $0.id == id }) else { return }
            orgID = srv.orgId ?? ""
            orgName = srv.orgName ?? ""
            name = srv.name
            base = srv.base
            token = srv.token
            viaRelay = srv.usesRelay
        }
    }

    /// 切换组织时预填名称/默认地址（用户可再改）。
    private func applyOrgPrefill() {
        guard let org = settings.availableUniFusionOrgs.first(where: { $0.orgId == orgID }) else { return }
        orgName = org.orgName
        name = org.orgName
        base = org.suggestedBase
    }

    /// 保存：先探测（直连；开启转发则云道可达也算通），通过才落库。
    private func save() {
        busy = true
        error = ""
        Task {
            let directOK = await APIClient.shared.healthCheck(base: base)
            let relayOK = viaRelay ? await APIClient.shared.healthCheck(base: settings.cloudBase) : false
            busy = false
            guard directOK || relayOK else {
                error = viaRelay
                    ? "无法连接：企业地址与云道转发均不可达（确认部署在线、网络可达）"
                    : "无法连接该企业地址（确认部署在线且地址正确）"
                return
            }
            switch mode {
            case .add:
                // 若用户从目录选了组织则用其 orgId/orgName；否则视为手动兜底（生成独立 orgId）。
                let finalOrgID = orgID.isEmpty ? "manual-\(UUID().uuidString.prefix(8))" : orgID
                let finalOrgName = orgName.isEmpty ? name : orgName
                settings.addServer(name: name.isEmpty ? "UniFusion" : name,
                                   base: base, token: token,
                                   viaRelay: viaRelay,
                                   orgId: finalOrgID, orgName: finalOrgName)
            case .edit(let id):
                settings.updateUniFusion(id: id, name: name, base: base, token: token, viaRelay: viaRelay)
            }
            dismiss()
        }
    }
}
