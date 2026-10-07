//
//  MachineSwitchView.swift
//  VoxSign
//
//  机器切换面板（点顶栏机器名打开）。分区顺序（生产登记 build 13）：
//    1. 云道（默认）：零配置，点击即切换
//    2. 独立部署：按客户/部署实例各一台（条目名=客户名），主数据源=后端 /v1/orgs；
//       后端空/零权限/拉取失败 → 空态「暂无可用独立部署」+ 手动添加兜底入口
//    3. 自建机器：机器码/直连/云端转发均可，用户自加
//  - 右上「管理」→ 设置页（添加/管理服务器）
//

import SwiftUI

/// 机器切换面板分区顺序契约（build 13 生产登记）：
/// CaseIterable 顺序 = 面板实际渲染顺序：云道 → 独立部署 → 自建机器。
/// 单测据此断言顺序，改分区顺序必须同步改此枚举。
enum MachinePartition: CaseIterable {
    case cloud       // 云道（默认）
    case unifusion   // 独立部署（主数据源 = 后端 /v1/orgs）
    case selfHosted  // 自建机器
}

struct MachinePickerView: View {
    /// 独立部署区空态文案（后端空/零权限/失败时展示）。
    static let emptyUnifusionText = "暂无可用独立部署"
    /// 空态内手动添加企业部署兜底入口的 accessibility id（设置页同名）。
    static let manualAddIdentifier = "vhs.unifusion.add"

    @EnvironmentObject var model: AppModel
    @ObservedObject private var settings: SettingsStore
    @ObservedObject private var conn: ConnectivityService
    @Environment(\.dismiss) private var dismiss

    // UniFusion 添加/编辑弹层
    @State private var showUniFusionAdd = false
    @State private var unifusionEditID: String?

    /// 生产用法：MachinePickerView()（默认共享单例）；单测可注入独立 UserDefaults 的 store。
    init(settings: SettingsStore = .shared, conn: ConnectivityService = .shared) {
        self.settings = settings
        self.conn = conn
    }

    var body: some View {
        NavigationStack {
            List {
                // 1) 云道（默认）
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

                // 2) 独立部署（主数据源=后端 /v1/orgs；空态文案 + 手动添加兜底入口）
                Section {
                    if settings.unifusionServers.isEmpty {
                        Text(MachinePickerView.emptyUnifusionText)
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                        Button {
                            showUniFusionAdd = true
                        } label: {
                            Label("手动添加企业部署地址", systemImage: "building.2")
                                .font(.system(size: 15))
                        }
                        .accessibilityIdentifier(MachinePickerView.manualAddIdentifier)
                    } else {
                        ForEach(settings.unifusionServers) { s in
                            Button {
                                settings.setMode(.selfHosted)
                                settings.switchServer(s.id)
                                dismiss()
                            } label: {
                                HStack {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(s.name.isEmpty ? "独立部署" : s.name)
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
                    Text("独立部署")
                } footer: {
                    Text("按客户/部署实例提供的私有化部署入口（来自 /v1/orgs）。")
                }

                // 3) 自建机器（不含 UniFusion 组织条目）
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
