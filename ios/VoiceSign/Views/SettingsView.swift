//
//  SettingsView.swift
//  VoiceSign
//
//  服务设置：server 地址 + Bearer token（UserDefaults 持久化，等价 web localStorage）。
//  "测试连接"调 GET /v1/status。
//

import SwiftUI
#if canImport(Speech)
import Speech
#endif

struct SettingsView: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var settings: SettingsStore
    #if canImport(Speech)
    @EnvironmentObject var speech: SpeechRecognizer
    #endif
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            Form {
                Section("Server 地址") {
                    TextField("http://192.168.x.x:8765", text: $settings.base)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                }
                Section("Bearer Token") {
                    SecureField("不配则仅本机", text: $settings.token)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                }
                Section {
                    Button("测试连接") { model.testConnection() }
                    if !model.statusLine.isEmpty {
                        Text(model.statusLine).font(.system(size: 12))
                    }
                }
                // T1 后台能力：常听开关 + 离线队列补投。
                #if canImport(Speech)
                Section("后台能力") {
                    Toggle("常听模式（后台持续收音）", isOn: $speech.alwaysOn)
                    Button("补投离线队列") {
                        Task { _ = await model.flushQueue() }
                    }
                    Text("离线队列 \(DeliveryQueue.shared.count) 条待投递 · 通知已授权=\(NotificationService.shared.authorized ? "是" : "否")")
                        .font(.system(size: 11)).foregroundColor(.secondary)
                }
                #endif
                Section {
                    Text("与 Mac 同 Wi-Fi；按 M3 指引起好 vhs serve（绑 0.0.0.0:8765 + token）。").font(.system(size: 11)).foregroundColor(.secondary)
                }
            }
            .navigationTitle("服务设置")
            .toolbar {
                Button("完成") { dismiss() }
            }
        }
    }
}
