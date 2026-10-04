//
//  ConnectionStatusView.swift
//  VoiceSign
//
//  T2 豆包式交互·连接状态胶囊：顶部常驻一条细胶囊，
//  绿=在线 · 黄=重连中 · 灰=离线（指令已排队）—— 网络状态对用户永远透明，不再"黑盒卡死"。
//

import SwiftUI

struct ConnectionStatusView: View {
    @ObservedObject var conn = ConnectivityService.shared

    var body: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(color)
                .frame(width: 8, height: 8)
            Text(label)
                .font(.system(size: 11, weight: .medium))
                .foregroundColor(.secondary)
            Spacer()
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 4)
    }

    private var color: Color {
        switch conn.state {
        case .online: return .green
        case .reconnecting: return .yellow
        case .offline: return .gray
        case .unknown: return .gray
        }
    }

    private var label: String {
        switch conn.state {
        case .online: return "已连接"
        case .reconnecting: return "正在重连…"
        case .offline: return "离线 · 语音指令将自动排队"
        case .unknown: return "连接检测中…"
        }
    }
}
