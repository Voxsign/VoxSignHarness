//
//  SettingsView.swift
//  VoiceSign
//
//  服务设置：server 地址 + Bearer token（UserDefaults 持久化，等价 web localStorage）。
//  "测试连接"调 GET /v1/status。
//

import SwiftUI

struct SettingsView: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var settings: SettingsStore
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
