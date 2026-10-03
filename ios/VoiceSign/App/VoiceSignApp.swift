//
//  VoiceSignApp.swift
//  VoiceSign
//
//  App 入口。
//

import SwiftUI

@main
struct VoiceSignApp: App {
    @StateObject private var model = AppModel()
    @StateObject private var settings = SettingsStore.shared
    #if canImport(Speech)
    @StateObject private var speech = SpeechRecognizer.shared
    #endif

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(model)
                .environmentObject(settings)
                #if canImport(Speech)
                .environmentObject(speech)
                #endif
                .onAppear {
                    #if canImport(Speech)
                    speech.requestAuthorization()
                    #endif
                    // T1 后台能力：请求通知权限 + 启动时补投离线队列。
                    NotificationService.shared.requestAuthorization()
                    Task { await model.flushQueue() }
                }
        }
    }
}
