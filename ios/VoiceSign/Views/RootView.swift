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
            RoleBarView()

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
                .onChange(of: model.rows.count) { _ in
                    if let last = model.rows.last {
                        withAnimation { proxy.scrollTo(last.id, anchor: .bottom) }
                    }
                }
            }

            // 一屏一个决策点
            DecisionZoneView(decision: model.decision, onAnswer: { ans in model.answer(ans) })
                .padding(.horizontal, 12)

            // M7 诊断行：上屏显示最近轮询状态/失败原因（DEBUG），避免黑盒"正在处理…"
            #if DEBUG
            if !model.diagLine.isEmpty {
                Text(model.diagLine)
                    .font(.system(size: 10, weight: .regular))
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 12)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            #endif

            InputBarView()
        }
        .background(VSColor.bg.ignoresSafeArea())
        .sheet(isPresented: $model.showSettings) {
            SettingsView()
        }
    }

    @ViewBuilder
    private func rowView(_ row: ChatRow) -> some View {
        switch row {
        case .user(let b): UserBubbleView(bubble: b)
        case .harness(let b): HarnessBubbleView(bubble: b)
        case .typing: TypingView()
        case .execCard(let s): ExecCardView(state: s)
        case .receipt(let r):
            ReceiptCardView(receipt: r.receipt, undo: r.undo, badges: r.badges, onRollback: { model.rollback() })
        }
    }
}
