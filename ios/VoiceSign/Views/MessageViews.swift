//
//  MessageViews.swift
//  VoiceSign
//
//  对话流渲染：用户右蓝气泡（语音带声波）、Harness 左白气泡、轻标签、三点处理中、执行卡、回执卡。
//  纯视图，无判断逻辑（判断在 VSLogic）。
//

import SwiftUI

// MARK: - 配色（蓝白主题，对齐 web styles.css：主蓝 #1f6bff）

enum VSColor {
    static let blue = Color(red: 0.12, green: 0.42, blue: 1.0)
    static let bg = Color(red: 0.95, green: 0.96, blue: 0.99)
    static let harnessBubble = Color.white
    static let userBubble = blue
    static let receiptGreen = Color(red: 0.90, green: 0.97, blue: 0.91)
    static let confirmRed = Color(red: 0.97, green: 0.90, blue: 0.90)
}

// MARK: - 徽章

struct BadgeView: View {
    let badge: Badge
    var body: some View {
        Text(badge.label)
            .font(.system(size: 10, weight: .medium))
            .padding(.horizontal, 6).padding(.vertical, 2)
            .background(toneColor.opacity(0.15))
            .foregroundColor(toneColor)
            .cornerRadius(6)
    }

    private var toneColor: Color {
        switch badge.tone {
        case "green": return .green
        case "red": return .red
        case "amber": return .orange
        case "gray": return .gray
        default: return VSColor.blue
        }
    }
}

// MARK: - 气泡

struct UserBubbleView: View {
    let bubble: Bubble
    var body: some View {
        HStack {
            Spacer()
            HStack(alignment: .bottom, spacing: 6) {
                if bubble.fromVoice { WaveView() }
                Text(bubble.text)
                    .foregroundColor(.white)
                    .padding(.horizontal, 12).padding(.vertical, 8)
            }
            .background(VSColor.userBubble)
            .cornerRadius(16)
        }
    }
}

struct HarnessBubbleView: View {
    let bubble: Bubble
    var body: some View {
        HStack {
            Text(bubble.text)
                .foregroundColor(.black)
                .padding(.horizontal, 12).padding(.vertical, 8)
                .background(VSColor.harnessBubble)
                .cornerRadius(16)
            Spacer()
        }
    }
}

/// 声波动画（语音输入指示）。用 TimelineView 驱动，避免依赖 @State 宏。
struct WaveView: View {
    var body: some View {
        TimelineView(.animation) { timeline in
            let t = timeline.date.timeIntervalSinceReferenceDate
            HStack(spacing: 2) {
                ForEach(0..<3) { i in
                    let phase = sin(t * 5 + Double(i) * 0.9)
                    Capsule()
                        .fill(Color.white)
                        .frame(width: 3, height: 5 + max(0, phase) * 6)
                }
            }
        }
        .frame(height: 14)
    }
}

/// 三点处理中 + 动态文案（I04 思考态 / I18 长任务升级文案）。
/// 用 TimelineView 驱动，避免依赖 @State 宏。
struct TypingView: View {
    var text: String = "正在思考…"

    var body: some View {
        HStack(spacing: 10) {
            TimelineView(.animation) { timeline in
                let t = timeline.date.timeIntervalSinceReferenceDate
                HStack(spacing: 4) {
                    ForEach(0..<3) { i in
                        let phase = sin(t * 4 + Double(i) * 0.9)
                        Circle()
                            .fill(Color.gray.opacity(0.6))
                            .frame(width: 7, height: 7)
                            .offset(y: max(0, phase) * -4)
                    }
                }
            }
            Text(text)
                .font(.system(size: 13))
                .foregroundColor(.gray)
        }
        .padding(12)
        .background(VSColor.harnessBubble)
        .cornerRadius(16)
    }
}

// MARK: - 执行卡（SSE stage 事件驱动滚动阶段行）

struct ExecCardView: View {
    let state: ExecCardState
    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("正在处理…")
                .font(.system(size: 12, weight: .semibold))
                .foregroundColor(.gray)
            ForEach(state.stages) { stage in
                HStack(spacing: 6) {
                    Image(systemName: stage.done ? "checkmark.circle.fill"
                          : stage.active ? "circle.fill" : "circle")
                        .font(.system(size: 12))
                        .foregroundColor(stage.done ? .green
                                         : stage.active ? VSColor.blue : .gray.opacity(0.5))
                    Text(stage.name)
                        .font(.system(size: 12))
                        .foregroundColor(stage.done || stage.active ? .black : .gray.opacity(0.7))
                }
            }
        }
        .padding(12)
        .background(VSColor.harnessBubble)
        .cornerRadius(16)
    }
}

// MARK: - 回执 → 人话气泡（v2.1：I01 人话回复 / I12 动作+对象 / I07+I17 撤销小字 44pt）

struct ReceiptCardView: View {
    let receipt: Receipt
    let undo: UndoInfo
    let badges: [Badge]
    let onRollback: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            // v2.3（用户需求：微信式反馈"处理完了之后有多少时间"）：气泡上方显示"已处理 X.X 秒"。
            if receipt.elapsedSec > 0.01 {
                Text("已处理 \(String(format: "%.1f", receipt.elapsedSec)) 秒")
                    .font(.system(size: 11))
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 2)
            }
            // 豆包式人话气泡：直接显示后台回复内容。
            // v2.3 用户原话"什么？又是给我反馈的'已完成'？已完成什么东西？"——去掉"已完成，动作。"前缀，
            // 界面只保留实质内容（后台说什么就显示什么）。
            Text(receipt.result)
                .font(.system(size: 14))
                .foregroundColor(.black)
                .padding(.horizontal, 12).padding(.vertical, 9)
                .background(VSColor.harnessBubble)
                .cornerRadius(16)
            // 撤销小字（I07 保留；I17 命中区≥44pt，且支持口答"撤销"）
            if undo.show {
                Button(action: onRollback) {
                    Text("可撤销")
                        .font(.system(size: 11))
                        .foregroundColor(.secondary)
                        .frame(minHeight: 44)
                        .padding(.horizontal, 4)
                }
            }
        }
    }
}
