//
//  MessageViews.swift
//  VoiceSign
//
//  对话流渲染：用户右蓝气泡（语音带声波）、Harness 左白气泡、轻标签、三点处理中、执行卡、回执卡。
//  纯视图，无判断逻辑（判断在 VSLogic）。
//

import SwiftUI

// MARK: - 配色（豆包视觉：主蓝 #3370FF → 紫 #8B5CF6 渐变、白底浅灰会话、大圆角）

enum VSColor {
    /// 豆包主蓝 #3370FF
    static let blue = Color(red: 0.20, green: 0.44, blue: 1.0)
    /// 豆包渐变紫 #8B5CF6
    static let purple = Color(red: 0.545, green: 0.36, blue: 0.965)
    /// 会话背景（浅灰，豆包式）
    static let bg = Color(red: 0.949, green: 0.953, blue: 0.965)
    static let harnessBubble = Color.white
    /// 卡片级阴影（App Store 精致度：柔和低透明度，不抢内容）
    static let shadow = Color.black.opacity(0.06)
    /// UI v3 豆包式 AI 气泡阴影：刻意压到几乎看不见（设计稿：opacity 0.045 / radius 0.75）。
    static let shadowSoft = Color.black.opacity(0.045)
    /// 用户气泡高光（顶部左上更亮，增加立体感）
    static var userBubbleGradientHigh: LinearGradient {
        LinearGradient(colors: [Color(red: 0.32, green: 0.55, blue: 1.0), purple],
                       startPoint: .topLeading, endPoint: .bottomTrailing)
    }
    /// 品牌渐变（标题/按钮统一用）
    static var brandGradient: LinearGradient {
        LinearGradient(colors: [blue, purple], startPoint: .topLeading, endPoint: .bottomTrailing)
    }
    /// UI v3 用户气泡：豆包同款 135° 蓝紫渐变 #4E7CFF → #8E6BFF（仅用户气泡与语音按钮使用）。
    static var userBubbleGradient: LinearGradient {
        LinearGradient(colors: [Color(red: 0.306, green: 0.486, blue: 1.0),
                                Color(red: 0.557, green: 0.42, blue: 1.0)],
                       startPoint: .topLeading, endPoint: .bottomTrailing)
    }
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
            HStack(alignment: .center, spacing: 6) {
                if bubble.fromVoice {
                    WaveView()
                        // UI v3：录音态声波为白色；完成态保持白色细条（豆包同款）。
                        .opacity(0.9)
                }
                Text(bubble.text)
                    .foregroundColor(.white)
                    .padding(.horizontal, 12).padding(.vertical, 8)
                // UI v3：语音消息时长（豆包同款 "3″" 小字）。
                if let secs = bubble.voiceSeconds {
                    Text("\(secs)″")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.white.opacity(0.85))
                        .padding(.trailing, 4)
                }
            }
            .background(VSColor.userBubbleGradientHigh)
            .clipShape(UnevenRoundedRectangle(topLeadingRadius: 18, bottomLeadingRadius: 18,
                                              bottomTrailingRadius: 4, topTrailingRadius: 18))
            .shadow(color: VSColor.shadow, radius: 6, x: 0, y: 2)
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
                .clipShape(UnevenRoundedRectangle(topLeadingRadius: 18, bottomLeadingRadius: 4,
                                                  bottomTrailingRadius: 18, topTrailingRadius: 18))
                // UI v3：AI 气泡阴影压到几乎看不见（豆包式）。
                .shadow(color: VSColor.shadowSoft, radius: 0.75, x: 0, y: 1)
            Spacer()
        }
    }
}

/// 声波动画（语音输入指示）。波形高度 = 实时录音振幅(meterLevel) + 轻微相位动画，
/// 豆包式"按住有反应"：说话越响波形越高。
struct WaveView: View {
    var meterLevel: Float = 0.5

    var body: some View {
        TimelineView(.animation) { timeline in
            let t = timeline.date.timeIntervalSinceReferenceDate
            let lvl = Double(meterLevel)
            HStack(spacing: 2) {
                ForEach(0..<3) { i in
                    let phase = sin(t * 5 + Double(i) * 0.9)
                    let h = 5 + max(0, phase) * 3 + lvl * 9
                    Capsule()
                        .fill(Color.white)
                        .frame(width: 3, height: h)
                }
            }
        }
        .frame(height: 20)
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
                            .fill(VSColor.blue.opacity(0.75))
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
                .clipShape(UnevenRoundedRectangle(topLeadingRadius: 18, bottomLeadingRadius: 4,
                                                  bottomTrailingRadius: 18, topTrailingRadius: 18))
            // 图片回执（闭环验收场景）：后台截图回执含 "/screenshots/<file>.png" → 直接渲染图片。
            // 图片 URL = 当前活动服务器 base + 相对路径（截图经 /screenshots/ 静态端点提供）。
            if let shotURL = ScreenshotURL.from(receipt.result, base: SettingsStore.shared.base) {
                AsyncImage(url: shotURL) { phase in
                    switch phase {
                    case .success(let img):
                        img.resizable().scaledToFit()
                            .frame(maxWidth: 240)
                            .clipShape(RoundedRectangle(cornerRadius: 12))
                            .shadow(color: VSColor.shadow, radius: 5, x: 0, y: 2)
                    case .failure:
                        Text("（截图加载失败）").font(.system(size: 12)).foregroundColor(.secondary)
                    default:
                        ProgressView().frame(width: 80, height: 80)
                    }
                }
                .padding(.leading, 4)
            }
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


// MARK: - 图片回执 URL 解析

enum ScreenshotURL {
    /// 从回执文本提取 /screenshots/<file>.png 相对路径，拼上当前服务器 base。
    static func from(_ text: String, base: String) -> URL? {
        guard let rng = text.range(of: "/screenshots/") else { return nil }
        var end = text.index(rng.lowerBound, offsetBy: "/screenshots/".count)
        var path = "/screenshots/"
        while end < text.endIndex {
            let ch = text[end]
            if ch == " " || ch == "\n" || ch == "（" || ch == ")" || ch == "。" { break }
            path.append(ch)
            end = text.index(after: end)
        }
        guard path.hasSuffix(".png") else { return nil }
        return URL(string: base + path)
    }
}
