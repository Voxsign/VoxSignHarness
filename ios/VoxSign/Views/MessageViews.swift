//
//  MessageViews.swift
//  VoxSign
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

// MARK: - 品牌常量（V4 §0）

/// V4 豆包式 UI 共享设计令牌：发件人标注。
/// 定义权归 MessageViews 执行者；RootView 顶栏只引用 `VSBrand.agentLabel`，不得重复定义。
enum VSBrand {
    static let agentLabel = "VoxSign·metasystem"
}

/// V4 §3c：用户语音气泡时长文案纯函数。
/// secs >= 60 → "共用时X分X秒"；否则 → "共用时X秒"。
func voiceDurationCaption(_ secs: Int) -> String {
    if secs >= 60 {
        return "共用时\(secs / 60)分\(secs % 60)秒"
    } else {
        return "共用时\(secs)秒"
    }
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

    private static let timeFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm"
        return f
    }()

    var body: some View {
        HStack {
            Spacer()
            VStack(alignment: .trailing, spacing: 4) {
                HStack(alignment: .center, spacing: 6) {
                    if bubble.fromVoice {
                        WaveView()
                            // UI v3：录音态声波为白色；完成态保持白色细条（豆包同款）。
                            .opacity(0.9)
                    }
                    Text(bubble.text)
                        // V4 §3d：消息气泡文本统一 16pt（豆包消息字号）。
                        .font(.system(size: 16))
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

                // 附件 chips：灰底小标签，不喧宾夺主；图片类如有 localPath 显示 40×40 缩略图。
                if !bubble.attachments.isEmpty {
                    VStack(alignment: .trailing, spacing: 4) {
                        ForEach(bubble.attachments) { att in
                            attachmentChip(att)
                        }
                    }
                }

                // V4 §3c：气泡下方右对齐元信息——HH:mm；语音消息追加 "· 共用时X分X秒"。
                metadataRow
            }
        }
    }

    /// V4 §3c：用户气泡下方元信息小字（11pt secondary，右对齐）。
    private var metadataRow: some View {
        HStack(spacing: 4) {
            Spacer()
            Text(Self.timeFormatter.string(from: bubble.timestamp))
            if bubble.fromVoice, let secs = bubble.voiceSeconds, secs > 0 {
                Text("· \(voiceDurationCaption(secs))")
            }
        }
        .font(.system(size: 11))
        .foregroundColor(.secondary)
        .padding(.trailing, 4)
        .padding(.top, 2)
    }

    @ViewBuilder
    private func attachmentChip(_ att: Attachment) -> some View {
        if att.kind == .image, let p = att.localPath, let img = UIImage(contentsOfFile: p) {
            Image(uiImage: img)
                .resizable()
                .scaledToFill()
                .frame(width: 40, height: 40)
                .clipShape(RoundedRectangle(cornerRadius: 6))
        } else {
            HStack(spacing: 5) {
                Image(systemName: chipIcon(att.kind))
                    .font(.system(size: 11))
                Text(att.title)
                    .font(.system(size: 11, weight: .medium))
            }
            .foregroundColor(.secondary)
            .padding(.horizontal, 8).padding(.vertical, 5)
            .background(Color(.secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 8))
        }
    }

    private func chipIcon(_ kind: AttachmentKind) -> String {
        switch kind {
        case .text: return "doc.text"
        case .url:  return "link"
        case .image: return "photo"
        case .file: return "doc"
        }
    }
}

struct HarnessBubbleView: View {
    let bubble: Bubble

    private static let timeFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm"
        return f
    }()

    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 3) {
                // V4 §3b：AI 气泡发件人标注（文本上方，11pt secondary）。
                Text(VSBrand.agentLabel)
                    .font(.system(size: 11))
                    .foregroundColor(.secondary)
                    .padding(.leading, 6)
                    .padding(.bottom, 2)
                Text(bubble.text)
                    // V4 §3d：消息气泡文本统一 16pt（豆包消息字号）。
                    .font(.system(size: 16))
                    .foregroundColor(.black)
                    .padding(.horizontal, 12).padding(.vertical, 8)
                    .background(VSColor.harnessBubble)
                    .clipShape(UnevenRoundedRectangle(topLeadingRadius: 18, bottomLeadingRadius: 4,
                                                      bottomTrailingRadius: 18, topTrailingRadius: 18))
                    // UI v3：AI 气泡阴影压到几乎看不见（豆包式）。
                    .shadow(color: VSColor.shadowSoft, radius: 0.75, x: 0, y: 1)

                // 轻量信息行：消耗（服务端不回传则整行隐藏）· 时间 + …菜单（无朗读/喇叭主按钮，不常用收进菜单）。
                infoRow
            }
            Spacer()
        }
    }

    private var infoRow: some View {
        HStack(spacing: 6) {
            if let cost = CostText.caption(for: bubble.costTokens) {
                Text(cost)
            }
            Text(Self.timeFormatter.string(from: bubble.timestamp))
            Spacer(minLength: 0)
            Menu {
                Button {
                    UIPasteboard.general.string = bubble.text
                } label: {
                    Label("复制", systemImage: "doc.on.doc")
                }
                Button {
                    VoiceOutputService.shared.speak(bubble.text)
                } label: {
                    Label("朗读", systemImage: "waveform")
                }
                ShareLink(item: bubble.text)
            } label: {
                Image(systemName: "ellipsis")
                    .font(.system(size: 12, weight: .medium))
                    .foregroundColor(.secondary)
                    .frame(width: 24, height: 24)
                    .contentShape(Rectangle())
            }
        }
        .font(.system(size: 11))
        .foregroundColor(.secondary)
        .padding(.leading, 4)
        .padding(.trailing, 2)
    }
}

/// 声波动画（语音输入指示）。波形高度 = 实时录音振幅(meterLevel) + 轻微相位动画，
/// 豆包式"按住有反应"：说话越响波形越高。
/// 默认 3 根小条（顶部状态条用）；录音态满底波形传 barCount: 18 / barWidth: 5 / barMaxHeight: 64。
struct WaveView: View {
    var meterLevel: Float = 0.5
    var barCount: Int = 3
    var color: Color = .white
    var barWidth: CGFloat = 3
    var barMaxHeight: CGFloat = 20

    var body: some View {
        TimelineView(.animation) { timeline in
            let t = timeline.date.timeIntervalSinceReferenceDate
            let lvl = Double(meterLevel)
            HStack(spacing: 2) {
                ForEach(0..<barCount, id: \.self) { i in
                    let phase = sin(t * 5 + Double(i) * 0.9)
                    // 高度 = 静息底 + 相位脉动 + 音量驱动；默认参数下与原 3 根行为一致（≈17pt）
                    let h = min(barMaxHeight, barMaxHeight * 0.22
                                + max(0, phase) * barMaxHeight * 0.14
                                + lvl * barMaxHeight * 0.5)
                    Capsule()
                        .fill(color)
                        .frame(width: barWidth, height: h)
                }
            }
        }
        .frame(height: barMaxHeight)
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
        VStack(alignment: .leading, spacing: 0) {
            // V4 §3b：回执卡上方同款发件人小字（左对齐）。
            Text(VSBrand.agentLabel)
                .font(.system(size: 11))
                .foregroundColor(.secondary)
                .padding(.leading, 2)
                .padding(.bottom, 4)

            VStack(alignment: .leading, spacing: 8) {
                // 状态行：✅ 已完成 主文案 + · X.X 秒 次要（elapsedSec > 0.01 时显示）。
                HStack(spacing: 6) {
                    Text("✅ 已完成")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(.black)
                    if receipt.elapsedSec > 0.01 {
                        Text("· \(String(format: "%.1f", receipt.elapsedSec)) 秒")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                    }
                }

                // 内容文本（后台人话回复）。
                Text(receipt.result)
                    .font(.system(size: 14))
                    .foregroundColor(.black)

                // 图片回执（闭环验收场景）：后台截图回执含 "/screenshots/<file>.png" → 直接渲染图片。
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
                }

                // 撤销按钮行（次要按钮，命中区 ≥44pt）。
                if undo.show {
                    Button(action: onRollback) {
                        Text("撤销")
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                            .frame(minHeight: 44)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(14)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color.white)
            .clipShape(RoundedRectangle(cornerRadius: 16))
            .overlay(
                RoundedRectangle(cornerRadius: 16)
                    .stroke(Color.black.opacity(0.08), lineWidth: 0.5)
            )
            .shadow(color: VSColor.shadowSoft, radius: 4, x: 0, y: 2)
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
