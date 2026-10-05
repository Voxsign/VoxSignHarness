//
//  RootView.swift
//  VoiceSign
//
//  根视图：多角色折叠条 / 红色打断系统条 / 对话流（气泡·执行卡·回执卡）/ 决策点区 / 底部输入条 / 设置页。
//

import SwiftUI

struct RootView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(spacing: 0) {
            // T3 豆包式简化：不再显示角色条（Planner/Executor/Verifier 收敛进执行卡内部状态）。
            // 只保留顶部连接状态胶囊 + 打断系统条 + 对话流 + 决策点 + 输入条。

            // T2 连接状态胶囊：绿=在线 · 黄=重连 · 灰=离线排队（网络状态永远透明）
            // T3 豆包式：顶部一行 = 连接胶囊（左）+ 纯黑标题（中）+ harness 状态点（预留）+ 设置齿轮（右）
            HStack(spacing: 8) {
                ConnectionStatusView()
                Spacer()
                Text("VoxSign")
                    .font(.system(size: 17, weight: .semibold))
                    .foregroundColor(.black)
                // UI v3：标题右侧 5pt harness 状态点（idle 灰 / busy 蓝呼吸 / decision 橙）——
                // 刻意压到最小，不破坏豆包式顶栏克制感。
                HarnessDot(state: model.harnessState)
                Spacer()
                // 设置入口（T3 修复：齿轮常驻顶部，不再随角色条隐藏）
                Button {
                    model.showSettings = true
                } label: {
                    Image(systemName: "gearshape")
                        .font(.system(size: 15, weight: .medium))
                        .foregroundColor(.black)
                        .frame(width: 34, height: 34)
                        .contentShape(Rectangle())
                }
                .accessibilityIdentifier("vhs.settings")
                .padding(.trailing, 10)
            }
            .padding(.leading, 12)
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

// MARK: - Harness 状态点（UI v3 预留位，豆包式 5pt 极小状态件）

/// 顶栏标题右侧 5pt 状态点：idle 灰 / busy 蓝呼吸 / decision 橙。
/// 刻意压到最小、不带文字——能力上线后也不破坏豆包式顶栏的克制感。
struct HarnessDot: View {
    let state: HarnessState

    var body: some View {
        TimelineView(.animation(minimumInterval: 0.6)) { timeline in
            let t = timeline.date.timeIntervalSinceReferenceDate
            Circle()
                .fill(color)
                .frame(width: 5, height: 5)
                .scaleEffect(state == .busy ? 1.0 + 0.35 * max(0, sin(t * 5)) : 1.0)
                .opacity(state == .idle ? 0.55 : 1.0)
        }
        .padding(.leading, 2)
    }

    private var color: Color {
        switch state {
        case .idle: return Color.gray
        case .busy: return VSColor.blue
        case .decision: return Color.orange
        }
    }
}
