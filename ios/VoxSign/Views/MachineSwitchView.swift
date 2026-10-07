//
//  MachineSwitchView.swift
//  VoxSign
//
//  机器切换面板（点顶栏机器名打开）：
//  - 云道（默认）：零配置，点击即切换
//  - 自建服务器列表：点击切换（机器码/直连/云端转发均可）
//  - UniFusion 独立部署：按用户所属组织各一台（机器名=组织名），点击切换，可编辑/删除
//  - 右上「管理」→ 设置页（添加/管理服务器）
//

import SwiftUI

struct MachinePickerView: View {
    @EnvironmentObject var model: AppModel
    @ObservedObject private var settings = SettingsStore.shared
    @ObservedObject private var conn = ConnectivityService.shared
    @Environment(\.dismiss) private var dismiss

    // UniFusion 添加/编辑弹层
    @State private var showUniFusionAdd = false
    @State private var unifusionEditID: String?

    var body: some View {
        NavigationStack {
            List {
                // 云端（默认）
                Section {
                    Button {
                        settings.setMode(.cloud)
                        dismiss()
                    } label: {
                        HStack {
                            Label("VoxSign 云端", systemImage: "cloud")
                                .font(.system(size: 15))
                            Spacer()
                            if conn.state == .online
                                && settings.mode == .cloud {
                                Image(systemName: "checkmark")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(VSColor.blue)
                            }
                        }
                    }
                    .accessibilityIdentifier("vhs.machine.cloud")
                } header: {
                    Text("云端")
                } footer: {
                    Text("零配置，走 VoxSign 云端服务。")
                }

                // 自建机器（不含 UniFusion 组织条目）
                Section {
                    if settings.selfHostedServers.isEmpty {
                        Text("暂无自建机器，点右上「管理」添加")
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                    } else {
                        ForEach(settings.selfHostedServers) { s in
                            Button {
                                settings.setMode(.selfHosted)
                                settings.switchServer(s.id)
                                dismiss()
                            } label: {
                                HStack {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(s.name.isEmpty ? "未命名机器" : s.name)
                                            .font(.system(size: 15))
                                            .foregroundColor(.primary)
                                        Text(s.base)
                                            .font(.system(size: 11))
                                            .foregroundColor(.secondary)
                                            .lineLimit(1)
                                    }
                                    Spacer()
                                    if conn.state == .online
                                        && settings.mode == .selfHosted
                                        && settings.activeServerID == s.id {
                                        Image(systemName: "checkmark")
                                            .font(.system(size: 13, weight: .semibold))
                                            .foregroundColor(VSColor.blue)
                                    }
                                }
                            }
                            .accessibilityIdentifier("vhs.selfhost.row.\(s.id)")
                        }
                    }
                } header: {
                    Text("自建机器")
                }

                // UniFusion 独立部署（按用户组织归属，一台组织一台）
                Section {
                    if settings.unifusionServers.isEmpty {
                        Button {
                            showUniFusionAdd = true
                        } label: {
                            Label("添加企业部署地址", systemImage: "building.2")
                                .font(.system(size: 15))
                        }
                        .accessibilityIdentifier("vhs.unifusion.add")
                    } else {
                        ForEach(settings.unifusionServers) { s in
                            Button {
                                settings.setMode(.selfHosted)
                                settings.switchServer(s.id)
                                dismiss()
                            } label: {
                                HStack {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(s.name.isEmpty ? "UniFusion" : s.name)
                                            .font(.system(size: 15))
                                            .foregroundColor(.primary)
                                        Text(s.base)
                                            .font(.system(size: 11))
                                            .foregroundColor(.secondary)
                                            .lineLimit(1)
                                    }
                                    Spacer()
                                    if conn.state == .online
                                        && settings.mode == .selfHosted
                                        && settings.activeServerID == s.id {
                                        Image(systemName: "checkmark")
                                            .font(.system(size: 13, weight: .semibold))
                                            .foregroundColor(VSColor.blue)
                                    }
                                }
                            }
                            .accessibilityIdentifier("vhs.unifusion.row.\(s.orgId ?? s.id)")
                            .swipeActions {
                                Button {
                                    unifusionEditID = s.id
                                } label: {
                                    Label("编辑", systemImage: "pencil")
                                }
                                Button(role: .destructive) {
                                    settings.removeServer(s.id)
                                } label: {
                                    Label("删除", systemImage: "trash")
                                }
                            }
                        }
                    }
                } header: {
                    Text("UniFusion 独立部署")
                } footer: {
                    Text("按你所属组织提供的私有化部署入口。")
                }
            }
            .navigationTitle("选择机器")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button {
                        dismiss()
                        model.showSettings = true
                    } label: {
                        Label("管理", systemImage: "gearshape")
                    }
                }
            }
        }
        .presentationDetents([.medium, .large])
        .sheet(isPresented: $showUniFusionAdd) {
            UniFusionEditView(mode: .add)
                .environmentObject(settings)
        }
        .sheet(item: Binding(
            get: { unifusionEditID.map { IdentifiableID(id: $0) } },
            set: { unifusionEditID = $0?.id }
        )) { item in
            UniFusionEditView(mode: .edit(item.id))
                .environmentObject(settings)
        }
    }
}

/// 包裹 String 以适配 sheet(item:) 的 Identifiable。
private struct IdentifiableID: Identifiable {
    let id: String
}
