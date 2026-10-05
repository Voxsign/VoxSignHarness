//
//  RootView.swift
//  VoxSign
//
//  根视图：顶栏（会话入口 + 连接状态点 + 标题 + …菜单）/ 红色打断系统条 / 对话流（气泡·执行卡·回执卡）/ 决策点区 / 底部输入条 / 设置页 / 会话列表。
//

import SwiftUI

struct RootView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(spacing: 0) {
            // T3 豆包式简化：不再显示角色条（Planner/Executor/Verifier 收敛进执行卡内部状态）。
            // 顶栏（极简）：[会话历史小图标] [5pt 状态点] [VoxSign] … [ellipsis Menu]
            HStack(spacing: 6) {
                // 左上：会话历史入口（只占 34pt 触控区，不占视觉）
                Button {
                    model.showSessions = true
                } label: {
                    Image(systemName: "bubble.left.and.bubble.right")
                        .font(.system(size: 15, weight: .medium))
                        .foregroundColor(.black)
                        .frame(width: 34, height: 34)
                        .contentShape(Rectangle())
                }
                .accessibilityIdentifier("vhs.sessions")

                // 5pt 连接/ harness 状态点（蓝=正常 · 红=离线 · 灰=重连/未知 · 橙=决策）
                ConnectionDotView(conn: ConnectivityService.shared.state,
                                  harness: model.harnessState)

                Text("VoxSign")
                    .font(.system(size: 17, weight: .semibold))
                    .foregroundColor(.black)

                Spacer()

                // 右上：… 更多菜单（设置 / 新建会话）
                Menu {
                    Button {
                        model.newSession()
                    } label: {
                        Label("新建会话", systemImage: "square.and.pencil")
                    }
                    Button {
                        model.showSettings = true
                    } label: {
                        Label("设置", systemImage: "gearshape")
                    }
                } label: {
                    Image(systemName: "ellipsis")
                        .font(.system(size: 15, weight: .medium))
                        .foregroundColor(.black)
                        .frame(width: 34, height: 34)
                        .contentShape(Rectangle())
                }
                .accessibilityIdentifier("vhs.more")
                .padding(.trailing, 10)
            }
            .padding(.leading, 6)
            .background(.ultraThinMaterial)

            // 红色打断系统条（可关闭）
            if let bar = model.systemBar {
                SystemBarView(bar: bar,
                              onRollback: { model.rollback() },
                              onClose: { model.closeSystemBar() })
                    .padding(.horizontal, 12).padding(.vertical, 6)
                    .transition(.move(edge: .top).combined(with: .opacity))
            }

            // 对话流
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 10) {
                        // UI v3 豆包式空态：新会话只有一行极淡灰字（无欢迎屏、无示例卡片堆）。
                        if model.rows.isEmpty {
                            Text("说点什么，或按住下方按钮说话")
                                .font(.system(size: 14))
                                .foregroundColor(Color(red: 0.682, green: 0.682, blue: 0.698)) // #AEAEB2
                                .frame(maxWidth: .infinity)
                                .padding(.top, 80)
                        }
                        ForEach(model.rows) { row in
                            rowView(row)
                        }
                    }
                    .padding(12)
                }
                // T2 滚动修复：改用 scrollTick（每次追加气泡 +1），
                // 确保说完话后 Harness 的回复/执行卡/回执一定滚动到可见。
                .onChange(of: model.scrollTick) { _ in
                    if let last = model.rows.last {
                        // T3 豆包式：快速轻滚到底（0.1s），不僵硬不打断阅读。
                        withAnimation(.easeOut(duration: 0.1)) { proxy.scrollTo(last.id, anchor: .bottom) }
                    }
                }
            }

            // 一屏一个决策点
            DecisionZoneView(decision: model.decision, onAnswer: { ans in model.answer(ans) })
                .padding(.horizontal, 12)

            // T3 豆包式：不再显示任何诊断行/轮询标识（保持纯对话流）。
            InputBarView()
        }
        .background(VSColor.bg.ignoresSafeArea())
        .preferredColorScheme(.light)
        .sheet(isPresented: $model.showSettings) {
            SettingsView()
        }
        .sheet(isPresented: $model.showSessions) {
            SessionListView()
        }
    }

    @ViewBuilder
    private func rowView(_ row: ChatRow) -> some View {
        switch row {
        case .user(let b): UserBubbleView(bubble: b)
        case .harness(let b): HarnessBubbleView(bubble: b)
        case .typing: TypingView(text: model.typingText)
        // T3 豆包式：执行过程不再铺七项流程卡，统一收敛成"三点正在思考"（与豆包一致）。
        // v2.1 I18：思考态文案动态升级（5s/10s），由 model.typingText 驱动。
        case .execCard: TypingView(text: model.typingText)
        case .receipt(let r):
            ReceiptCardView(receipt: r.receipt, undo: r.undo, badges: r.badges, onRollback: { model.rollback() })
        }
    }
}

// MARK: - 顶栏 5pt 连接状态点（豆包式极简）

/// 5pt 圆点：颜色由 `TopBarDot.tone(conn:harness:)` 决定（红/蓝/灰/橙）；
/// busy / decision 时沿用呼吸动画。不带文字、不带胶囊——保持顶栏克制。
struct ConnectionDotView: View {
    let conn: ConnectionState
    let harness: HarnessState

    var body: some View {
        TimelineView(.animation(minimumInterval: 0.6)) { timeline in
            let t = timeline.date.timeIntervalSinceReferenceDate
            Circle()
                .fill(color)
                .frame(width: 5, height: 5)
                .scaleEffect(animating ? 1.0 + 0.35 * max(0, sin(t * 5)) : 1.0)
                .opacity(conn == .unknown ? 0.55 : 1.0)
        }
        .padding(.leading, 2)
        .accessibilityIdentifier("vhs.status.dot")
    }

    private var tone: DotTone {
        TopBarDot.tone(conn: conn, harness: harness)
    }

    private var color: Color {
        switch tone {
        case .blue:   return VSColor.blue
        case .red:    return Color.red
        case .gray:   return Color.gray
        case .orange: return Color.orange
        }
    }

    /// 仅在 harness 正在忙碌 / 需要决策时做呼吸动画（静态状态点不抖）。
    private var animating: Bool {
        harness == .busy || harness == .decision
    }
}
