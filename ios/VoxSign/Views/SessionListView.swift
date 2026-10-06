//
//  SessionListView.swift
//  VoxSign
//
//  豆包式多会话左侧抽屉（V6：从 sheet 弹窗改为左侧滑出，左右切开可见更多内容）：
//  - 按 updatedAt 降序展示历史会话
//  - 点击切换会话并关闭抽屉；滑动删除（仅剩一个时静默忽略）
//  - 头部右上"新建会话"入口
//

import SwiftUI

struct SessionDrawerView: View {
    @EnvironmentObject var model: AppModel
    /// 关闭抽屉（由 RootView 注入，带动画）。
    var onClose: () -> Void
    /// 打开设置（V6.1：设置入口收进抽屉左下角）。
    var onSettings: () -> Void

    /// V6.2 折叠状态：默认全展开，单击容器行折叠/展开（文件夹关系）。
    @State private var collapsed: Set<String> = []
    /// V6.3 待归类会话（左滑「归类」弹出面板）。
    @State private var classifyTarget: ChatSession?

    /// 按 updatedAt 降序排列（最近聊过的在前）。
    private var sorted: [ChatSession] {
        model.sessions.sorted { $0.updatedAt > $1.updatedAt }
    }

    private var domainContainers: [ContainerItem] {
        SessionStore.shared.containers(of: .domain)
    }
    private var roleContainers: [ContainerItem] {
        SessionStore.shared.containers(of: .role)
    }
    private var ungrouped: [ChatSession] {
        sorted.filter { $0.containerID == nil }
    }

    private func sessions(of c: ContainerItem) -> [ChatSession] {
        sorted.filter { $0.containerID == c.id }
    }

    private func toggle(_ key: String) {
        if collapsed.contains(key) { collapsed.remove(key) }
        else { collapsed.insert(key) }
    }

    var body: some View {
        VStack(spacing: 0) {
            // 抽屉头部：X 关闭 | 标题"会话" | 右上"新建会话"
            HStack(spacing: 8) {
                Button {
                    onClose()
                } label: {
                    Image(systemName: "xmark")
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.black)
                        .frame(width: 30, height: 30)
                        .contentShape(Rectangle())
                        .background(Color.black.opacity(0.05), in: RoundedRectangle(cornerRadius: 8))
                }
                .accessibilityIdentifier("vhs.session.close")

                Spacer()

                Text("会话")
                    .font(.system(size: 16, weight: .semibold))
                    .foregroundColor(.black)

                Spacer()

                Button {
                    model.newSession()
                    onClose()
                } label: {
                    Label("新建", systemImage: "square.and.pencil")
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.black)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 6)
                        .contentShape(Rectangle())
                        .background(Color.black.opacity(0.05), in: RoundedRectangle(cornerRadius: 8))
                }
                .accessibilityIdentifier("vhs.session.new")
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)

            Divider()

            // V6.3 列表：未分组置顶（先聊后归）→ 域组 → 角色组；容器行单击折叠/展开
            List {
                // 未分组（新会话默认落点）
                if !ungrouped.isEmpty {
                    Section {
                        if !collapsed.contains("__ungrouped__") {
                            ForEach(ungrouped) { s in
                                sessionRow(s)
                            }
                        }
                    } header: {
                        Button {
                            toggle("__ungrouped__")
                        } label: {
                            HStack(spacing: 6) {
                                Image(systemName: "tray")
                                    .font(.system(size: 13))
                                    .foregroundColor(.secondary)
                                Text("未分组")
                                    .font(.system(size: 14, weight: .semibold))
                                    .foregroundColor(.primary)
                                Text("\(ungrouped.count)")
                                    .font(.system(size: 11))
                                    .foregroundColor(.secondary)
                                Spacer()
                                Image(systemName: "chevron.down")
                                    .font(.system(size: 11, weight: .semibold))
                                    .foregroundColor(.secondary)
                                    .rotationEffect(.degrees(collapsed.contains("__ungrouped__") ? -90 : 0))
                            }
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                    }
                }

                // 域组
                if !domainContainers.isEmpty {
                    ForEach(domainContainers) { c in
                        Section {
                            if !collapsed.contains(c.id) {
                                ForEach(sessions(of: c)) { s in
                                    sessionRow(s)
                                }
                            }
                        } header: {
                            containerHeader(c)
                        }
                    }
                }

                // 角色组
                if !roleContainers.isEmpty {
                    ForEach(roleContainers) { c in
                        Section {
                            if !collapsed.contains(c.id) {
                                ForEach(sessions(of: c)) { s in
                                    sessionRow(s)
                                }
                            }
                        } header: {
                            containerHeader(c)
                        }
                    }
                }

                if sorted.isEmpty {
                    Section {
                        VStack(spacing: 6) {
                            Image(systemName: "bubble.left.and.bubble.right")
                                .font(.system(size: 22))
                                .foregroundColor(Color.black.opacity(0.25))
                            Text("暂无历史会话")
                                .font(.system(size: 14))
                                .foregroundColor(.secondary)
                            Text("点右上「新建」开始一段新对话")
                                .font(.system(size: 12))
                                .foregroundColor(.secondary)
                        }
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 40)
                    }
                }
            }
            .listStyle(.plain)

            Divider()

            // 左下角：设置入口（V6.1：设置不再放右上…菜单，收进抽屉底部）
            Button {
                onSettings()
            } label: {
                HStack(spacing: 8) {
                    Image(systemName: "gearshape")
                        .font(.system(size: 14, weight: .medium))
                    Text("设置")
                        .font(.system(size: 14, weight: .medium))
                    Spacer()
                }
                .foregroundColor(.primary)
                .padding(.horizontal, 14)
                .padding(.vertical, 12)
                .contentShape(Rectangle())
            }
            .accessibilityIdentifier("vhs.session.settings")
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color(.systemBackground))
        .accessibilityIdentifier("vhs.session.list")
        .sheet(item: $classifyTarget) { target in
            ClassifySheetView(sessionID: target.id)
                .environmentObject(model)
        }
    }

    // MARK: - V6.2 分组行

    /// 会话行（点选切换；左滑归类/移回；右滑删除）。
    private func sessionRow(_ s: ChatSession) -> some View {
        SessionRow(session: s,
                   isCurrent: s.id == model.currentSessionID,
                   onTap: {
                       model.switchSession(s.id)
                       onClose()
                   })
                   .swipeActions(edge: .leading, allowsFullSwipe: false) {
                       Button {
                           classifyTarget = s
                       } label: {
                           Label("归类", systemImage: "folder.badge.plus")
                       }
                       .tint(.blue)
                       // 已归类 → 可移回未分组
                       if s.containerID != nil {
                           Button {
                               model.unclassifySession(s.id)
                           } label: {
                               Label("移回", systemImage: "arrow.uturn.backward")
                           }
                           .tint(.gray)
                       }
                   }
                   .swipeActions(edge: .trailing, allowsFullSwipe: true) {
                       Button(role: .destructive) {
                           // deleteSession 返回 false（仅剩一个）时静默忽略。
                           _ = model.deleteSession(s.id)
                       } label: {
                           Label("删除", systemImage: "trash")
                       }
                   }
    }

    /// 容器头（文件夹行）：单击折叠/展开。
    private func containerHeader(_ c: ContainerItem) -> some View {
        Button {
            toggle(c.id)
        } label: {
            HStack(spacing: 6) {
                Image(systemName: c.kind == .domain ? "folder" : "person.circle")
                    .font(.system(size: 13))
                    .foregroundColor(c.kind == .domain ? Color.green : VSColor.blue)
                Text(c.name)
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.primary)
                Text("\(sessions(of: c).count)")
                    .font(.system(size: 11))
                    .foregroundColor(.secondary)
                Spacer()
                Image(systemName: "chevron.down")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(.secondary)
                    .rotationEffect(.degrees(collapsed.contains(c.id) ? -90 : 0))
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }
}

// MARK: - 单行（标题 + 更新时间 + 当前标记）

private struct SessionRow: View {
    let session: ChatSession
    let isCurrent: Bool
    let onTap: () -> Void

    var body: some View {
        HStack(spacing: 10) {
            // 当前会话左侧蓝点标记
            Circle()
                .fill(isCurrent ? VSColor.blue : Color.clear)
                .frame(width: 5, height: 5)

            VStack(alignment: .leading, spacing: 3) {
                Text(session.title.isEmpty ? "新会话" : session.title)
                    .font(.system(size: 15, weight: isCurrent ? .semibold : .regular))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                Text(relativeTime(session.updatedAt))
                    .font(.system(size: 11))
                    .foregroundColor(.secondary)
            }
            Spacer()
            if isCurrent {
                Text("当前")
                    .font(.system(size: 11))
                    .foregroundColor(VSColor.blue)
            }
        }
        .contentShape(Rectangle())
        .onTapGesture { onTap() }
    }

    /// 相对时间：今天→HH:mm；本周→周X；更早→MM-dd。
    private func relativeTime(_ d: Date) -> String {
        let f = DateFormatter()
        if Calendar.current.isDateInToday(d) {
            f.dateFormat = "HH:mm"
        } else if Calendar.current.isDate(d, equalTo: Date(), toGranularity: .year) {
            f.dateFormat = "MM-dd HH:mm"
        } else {
            f.dateFormat = "yyyy-MM-dd"
        }
        return f.string(from: d)
    }
}
