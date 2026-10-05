//
//  DecisionZoneView.swift
//  VoxSign
//
//  一屏一个决策点：need_ask 候选按钮 / need_confirm 确认卡 / canceled·interrupted 错误条。
//  UI v3（豆包式）：两类决策卡都是会话流内的浅色卡片——白底 14pt 圆角 + 0.5pt 系统描边，
//  头部小字标签（「需要确认」灰字 + 蓝对勾 / 「需要你选一个」），按钮 40-44pt 高。
//  为空则不占位（对齐 web decisionZone）。
//

import SwiftUI

struct DecisionZoneView: View {
    let decision: DecisionPoint?
    let onAnswer: (String) -> Void

    // UI v3：问询候选选中态（点中 #EAF0FF 蓝底蓝字）。
    @State private var selectedID: String? = nil

    var body: some View {
        switch decision?.kind {
        case .confirm:
            VStack(alignment: .leading, spacing: 10) {
                // 头部标签：需要确认（豆包式 11.5pt 灰字 + 蓝对勾）
                HStack(spacing: 4) {
                    Image(systemName: "checkmark.circle.fill")
                        .font(.system(size: 12))
                        .foregroundColor(VSColor.blue)
                    Text("需要确认")
                        .font(.system(size: 11.5, weight: .medium))
                        .foregroundColor(Color(red: 0.557, green: 0.557, blue: 0.576))
                }
                Text(decision?.question ?? "确认执行这个操作吗？")
                    .font(.system(size: 14))
                    .foregroundColor(.primary)
                // 按钮等分 40pt 高：取消灰底 / 执行蓝底
                HStack(spacing: 10) {
                    Button("取消") { onAnswer("拒绝") }
                        .font(.system(size: 14, weight: .medium))
                        .foregroundColor(.primary)
                        .frame(maxWidth: .infinity, minHeight: 40)
                        .background(Color.black.opacity(0.06))
                        .cornerRadius(10)
                    Button("执行") { onAnswer("执行") }
                        .font(.system(size: 14, weight: .semibold))
                        .foregroundColor(.white)
                        .frame(maxWidth: .infinity, minHeight: 40)
                        .background(VSColor.blue)
                        .cornerRadius(10)
                }
            }
            .padding(12)
            .background(Color.white)
            .cornerRadius(14)
            .overlay(
                RoundedRectangle(cornerRadius: 14)
                    .stroke(Color(red: 0.898, green: 0.898, blue: 0.918), lineWidth: 0.5)
            )

        case .ask:
            VStack(alignment: .leading, spacing: 8) {
                // 头部标签：需要你选一个
                HStack(spacing: 4) {
                    Image(systemName: "questionmark.circle.fill")
                        .font(.system(size: 12))
                        .foregroundColor(VSColor.blue)
                    Text("需要你选一个")
                        .font(.system(size: 11.5, weight: .medium))
                        .foregroundColor(Color(red: 0.557, green: 0.557, blue: 0.576))
                }
                Text(decision?.question ?? "你想让我做什么？")
                    .font(.system(size: 13))
                // 选项整行 44pt 白底描边按钮，点中变 #EAF0FF 蓝底蓝字（豆包式）
                ForEach(decision?.options ?? [], id: \.id) { opt in
                    Button {
                        selectedID = opt.id
                        onAnswer(opt.id)
                    } label: {
                        HStack {
                            Text(opt.label)
                                .font(.system(size: 14, weight: .medium))
                            Spacer()
                            if selectedID == opt.id {
                                Image(systemName: "checkmark.circle.fill")
                                    .font(.system(size: 14))
                                    .foregroundColor(VSColor.blue)
                            }
                        }
                        .padding(.horizontal, 12)
                        .frame(maxWidth: .infinity, minHeight: 44)
                        .background(selectedID == opt.id
                                    ? Color(red: 0.918, green: 0.941, blue: 1.0)   // #EAF0FF
                                    : Color.white)
                        .foregroundColor(selectedID == opt.id ? VSColor.blue : Color.primary)
                        .cornerRadius(10)
                        .overlay(
                            RoundedRectangle(cornerRadius: 10)
                                .stroke(selectedID == opt.id
                                        ? VSColor.blue.opacity(0.4)
                                        : Color(red: 0.898, green: 0.898, blue: 0.918),
                                        lineWidth: 0.5)
                        )
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(12)
            .background(Color.white)
            .cornerRadius(14)
            .overlay(
                RoundedRectangle(cornerRadius: 14)
                    .stroke(Color(red: 0.898, green: 0.898, blue: 0.918), lineWidth: 0.5)
            )

        case .error:
            Text(decision?.message ?? "任务出错")
                .font(.system(size: 13))
                .foregroundColor(.white)
                .padding(12)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Color.red.opacity(0.85))
                .cornerRadius(12)

        default:
            EmptyView()
        }
    }
}
