//
//  DecisionZoneView.swift
//  VoiceSign
//
//  一屏一个决策点：need_ask 候选按钮 / need_confirm 红色强确认条 / canceled·interrupted 错误条。
//  为空则不占位（对齐 web decisionZone）。
//

import SwiftUI

struct DecisionZoneView: View {
    let decision: DecisionPoint?
    let onAnswer: (String) -> Void

    var body: some View {
        switch decision?.kind {
        case .confirm:
            VStack(alignment: .leading, spacing: 10) {
                Text("⚠ \(decision?.question ?? "人工放行（不可逆）操作")")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.red)
                HStack {
                    Button("执行") { onAnswer("执行") }
                        .font(.system(size: 14, weight: .bold))
                        .padding(.horizontal, 18).padding(.vertical, 8)
                        .background(Color.red)
                        .foregroundColor(.white)
                        .cornerRadius(10)
                    Button("拒绝") { onAnswer("拒绝") }
                        .font(.system(size: 14))
                        .padding(.horizontal, 18).padding(.vertical, 8)
                        .background(Color.gray.opacity(0.2))
                        .foregroundColor(.primary)
                        .cornerRadius(10)
                }
            }
            .padding(12)
            .background(VSColor.confirmRed)
            .cornerRadius(14)

        case .ask:
            VStack(alignment: .leading, spacing: 8) {
                Text(decision?.question ?? "你想让我做什么？")
                    .font(.system(size: 13))
                ForEach(decision?.options ?? [], id: \.id) { opt in
                    Button(opt.label) { onAnswer(opt.id) }
                        .font(.system(size: 13))
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.horizontal, 12).padding(.vertical, 9)
                        .background(Color.white)
                        .foregroundColor(VSColor.blue)
                        .cornerRadius(10)
                }
            }
            .padding(12)
            .background(Color.white)
            .cornerRadius(14)
            .shadow(radius: 2)

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
