//
//  SessionListView.swift
//  VoxSign
//
//  豆包式多会话列表（顶栏左上角小图标进入，默认隐藏不占主界面）：
//  - 按 updatedAt 降序展示历史会话
//  - 点击切换会话并 dismiss；滑动删除（仅剩一个时静默忽略）
//  - 顶部"新建会话"入口
//

import SwiftUI

struct SessionListView: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.dismiss) private var dismiss

    /// 按 updatedAt 降序排列（最近聊过的在前）。
    private var sorted: [ChatSession] {
        model.sessions.sorted { $0.updatedAt > $1.updatedAt }
    }

    var body: some View {
        NavigationStack {
            Group {
                if sorted.isEmpty {
                    // 空列表一行提示
                    Text("暂无历史会话")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                } else {
                    List {
                        Section {
                            ForEach(sorted) { s in
                                SessionRow(session: s,
                                            isCurrent: s.id == model.currentSessionID,
                                            onTap: {
                                                model.switchSession(s.id)
                                                dismiss()
                                            })
                                            .swipeActions(edge: .trailing, allowsFullSwipe: true) {
                                                Button(role: .destructive) {
                                                    // deleteSession 返回 false（仅剩一个）时静默忽略。
                                                    _ = model.deleteSession(s.id)
                                                } label: {
                                                    Label("删除", systemImage: "trash")
                                                }
                                            }
                            }
                        }
                    }
                    .listStyle(.insetGrouped)
                }
            }
            .navigationTitle("会话")
            .navigationBarTitleDisplayMode(.inline)
            .accessibilityIdentifier("vhs.session.list")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button {
                        dismiss()
                    } label: {
                        Image(systemName: "xmark")
                            .font(.system(size: 13, weight: .medium))
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button {
                        model.newSession()
                        dismiss()
                    } label: {
                        Label("新建会话", systemImage: "square.and.pencil")
                    }
                    .accessibilityIdentifier("vhs.session.new")
                }
            }
        }
        .presentationDetents([.medium, .large])
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
