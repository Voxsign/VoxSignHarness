//
//  ClassifySheetView.swift
//  VoxSign
//
//  V6.3 归类面板（会话左滑「归类」弹出，先聊后归）：
//  - 顶部分段：角色 Role / 域 Domain
//  - 中部：已有容器列表，点选 → 会话归入
//  - 底部：输入新容器名 +「新建并归入」
//

import SwiftUI

struct ClassifySheetView: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.dismiss) private var dismiss
    /// 待归类的会话。
    let sessionID: String
    @State private var kind: ContainerKind = .role
    @State private var newName: String = ""

    private var store: SessionStore { SessionStore.shared }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                // 分段：角色 / 域
                Picker("容器类型", selection: $kind) {
                    Text("角色 Role").tag(ContainerKind.role)
                    Text("域 Domain").tag(ContainerKind.domain)
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)
                .padding(.top, 14)

                List {
                    Section {
                        if store.containers(of: kind).isEmpty {
                            Text(kind == .role ? "还没有角色，先新建一个" : "还没有域，先新建一个")
                                .font(.system(size: 13))
                                .foregroundColor(.secondary)
                        } else {
                            ForEach(store.containers(of: kind)) { c in
                                Button {
                                    model.classifySession(sessionID, kind: kind, containerID: c.id)
                                    dismiss()
                                } label: {
                                    HStack(spacing: 10) {
                                        Image(systemName: kind == .role ? "person.circle" : "folder")
                                            .font(.system(size: 15))
                                            .foregroundColor(kind == .role ? VSColor.blue : Color.green)
                                        VStack(alignment: .leading, spacing: 2) {
                                            Text(c.name)
                                                .font(.system(size: 15, weight: .medium))
                                                .foregroundColor(.primary)
                                            Text("会话 \(store.sessions.filter { $0.containerID == c.id }.count) 个")
                                                .font(.system(size: 11))
                                                .foregroundColor(.secondary)
                                        }
                                        Spacer()
                                        Image(systemName: "chevron.right")
                                            .font(.system(size: 12))
                                            .foregroundColor(.secondary)
                                    }
                                }
                            }
                        }
                    } header: {
                        Text(kind == .role ? "归入角色" : "归入域")
                    }
                }
                .listStyle(.insetGrouped)

                // 底部：新建容器并归入
                HStack(spacing: 10) {
                    TextField(kind == .role ? "新角色名称…" : "新域名…", text: $newName)
                        .textFieldStyle(.roundedBorder)
                        .font(.system(size: 14))
                    Button {
                        model.createContainerAndClassify(sessionID, kind: kind, name: newName)
                        newName = ""
                        dismiss()
                    } label: {
                        Text("新建并归入")
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundColor(.white)
                            .padding(.horizontal, 14)
                            .padding(.vertical, 8)
                            .background(VSColor.blue)
                            .clipShape(Capsule())
                    }
                    .disabled(newName.trimmingCharacters(in: .whitespaces).isEmpty)
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 12)
                .background(Color(.secondarySystemBackground))
            }
            .navigationTitle("归类会话")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button {
                        dismiss()
                    } label: {
                        Image(systemName: "xmark")
                            .font(.system(size: 13, weight: .medium))
                    }
                }
            }
            .accessibilityIdentifier("vhs.classify")
        }
        .presentationDetents([.medium, .large])
    }
}
