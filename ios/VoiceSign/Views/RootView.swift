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
            // T3 豆包式：顶部一行 = 连接胶囊（左）+ 标题（中）+ 设置齿轮（右）
            HStack(spacing: 8) {
                ConnectionStatusView()
                Spacer()
                Text("VoxSign")
                    .font(.system(size: 15, weight: .heavy, design: .rounded))
                    .foregroundStyle(VSColor.brandGradient)
                Spacer()
                // 设置入口（T3 修复：齿轮常驻顶部，不再随角色条隐藏）
                Button {
                    model.showSettings = true
                } label: {
                    Image(systemName: "gearshape.fill")
                        .font(.system(size: 16))
                        .foregroundColor(.secondary)
                        .frame(width: 32, height: 32)
                        .contentShape(Rectangle())
                }
                .accessibilityIdentifier("vhs.settings")
                .padding(.trailing, 10)
            }
            .padding(.leading, 12)

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
