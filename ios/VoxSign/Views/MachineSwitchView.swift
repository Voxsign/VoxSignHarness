//
//  MachineSwitchView.swift
//  VoxSign
//
//  V6.1 机器切换面板（点顶栏机器名打开）：
//  - 云道（默认）：零配置，点击即切换
//  - 自建服务器列表：点击切换（机器码/直连/云端转发均可）
//  - 右上「管理」→ 设置页（添加/管理服务器）
//

import SwiftUI

struct MachinePickerView: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            List {
                // 云端（默认）
                Section {
                    Button {
                        SettingsStore.shared.setMode(.cloud)
                        dismiss()
                    } label: {
                        HStack {
                            Label("VoxSign 云端", systemImage: "cloud")
                                .font(.system(size: 15))
                            Spacer()
                            if ConnectivityService.shared.state == .online
                                && SettingsStore.shared.mode == .cloud {
                                Image(systemName: "checkmark")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(VSColor.blue)
                            }
                        }
                    }
                } header: {
                    Text("云端")
                } footer: {
                    Text("零配置，走 VoxSign 云端服务。")
                }

                // 自建机器
                Section {
                    if SettingsStore.shared.servers.isEmpty {
                        Text("暂无自建机器，点右上「管理」添加")
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                    } else {
                        ForEach(SettingsStore.shared.servers) { s in
                            Button {
                                SettingsStore.shared.setMode(.selfHosted)
                                SettingsStore.shared.switchServer(s.id)
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
                                    if ConnectivityService.shared.state == .online
                                        && SettingsStore.shared.mode == .selfHosted
                                        && SettingsStore.shared.activeServerID == s.id {
                                        Image(systemName: "checkmark")
                                            .font(.system(size: 13, weight: .semibold))
                                            .foregroundColor(VSColor.blue)
                                    }
                                }
                            }
                        }
                    }
                } header: {
                    Text("自建机器")
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
    }
}
