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
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(bubble.text)
                    .foregroundColor(.black)
                    .padding(.horizontal, 12).padding(.vertical, 8)
                    .background(VSColor.harnessBubble)
                    .cornerRadius(16)
                Spacer()
            }
            if !bubble.badges.isEmpty {
                HStack {
                    ForEach(bubble.badges, id: \.label) { BadgeView(badge: $0) }
                    Spacer()
                }
            }
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

/// 三点处理中。用 TimelineView 驱动，避免依赖 @State 宏。
struct TypingView: View {
    var body: some View {
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

// MARK: - 回执卡（绿色四行 + 撤销按钮）

struct ReceiptCardView: View {
    let receipt: Receipt
    let undo: UndoInfo
    let badges: [Badge]
    let onRollback: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Image(systemName: "checkmark.seal.fill").foregroundColor(.green)
                Text("完成").font(.system(size: 13, weight: .semibold))
                Spacer()
            }
            line("动作", receipt.action)
            line("文件", receipt.files)
            line("结果", receipt.result)
            HStack {
                Text("撤销").font(.system(size: 12, weight: .bold))
                Text(receipt.undo).font(.system(size: 12))
                    .foregroundColor(.secondary)
                Spacer()
                if undo.show {
                    Button("撤销") { onRollback() }
                        .font(.system(size: 12, weight: .semibold))
                        .padding(.horizontal, 10).padding(.vertical, 4)
                        .background(Color.red.opacity(0.12))
                        .foregroundColor(.red)
                        .cornerRadius(8)
                }
            }
            if !badges.isEmpty {
                HStack { ForEach(badges, id: \.label) { BadgeView(badge: $0) }; Spacer() }
            }
        }
        .padding(12)
        .background(VSColor.receiptGreen)
        .cornerRadius(16)
    }

    private func line(_ k: String, _ v: String) -> some View {
        HStack(alignment: .top) {
            Text(k).font(.system(size: 12, weight: .bold))
            Text(v.isEmpty ? "—" : v).font(.system(size: 12))
            Spacer()
        }
    }
}
