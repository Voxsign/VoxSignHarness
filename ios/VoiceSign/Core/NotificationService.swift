//
//  NotificationService.swift
//  VoiceSign
//
//  T1 后台能力 · 本地通知桥：
//  App 处于后台/锁屏时，把 SSE 事件（need_ask / need_confirm / done / failed / canceled）
//  转成本地通知，用户点通知回到对应任务。
//  零第三方依赖：UNUserNotificationCenter（iOS 10+）。
//

import Foundation
import UserNotifications
#if canImport(UIKit)
import UIKit
#endif

final class NotificationService {
    static let shared = NotificationService()

    /// 通知权限是否已授权（设置页可显示状态）。
    private(set) var authorized: Bool = false

    private init() {}

    /// 请求通知权限（App 启动时调用；未授权不阻塞主流程）。
    func requestAuthorization() {
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { [weak self] granted, _ in
            DispatchQueue.main.async { self?.authorized = granted }
        }
    }

    /// 仅在后台/锁屏时发通知（前台由 UI 呈现，避免重复打扰）。
    /// - Returns: 是否已发出。
    @discardableResult
    func notifyIfBackground(title: String, body: String, taskId: String? = nil) -> Bool {
        guard isAppBackgrounded, authorized else { return false }
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        let req = UNNotificationRequest(
            identifier: taskId.map { "vhs-\($0)" } ?? "vhs-\(UUID().uuidString)",
            content: content,
            trigger: nil // 立即投递
        )
        UNUserNotificationCenter.current().add(req) { _ in }
        return true
    }

    /// 由 SSE 事件驱动：need_ask / need_confirm / done / failed / canceled。
    func routeEvent(_ type: String, taskId: String, seq: Int, payload: [String: Any]) {
        switch type {
        case "need_ask":
            let q = payload["question"] as? String ?? "需要你确认一个问题"
            notifyIfBackground(title: "VoiceSign · 需要你回答", body: q, taskId: taskId)
        case "need_confirm":
            let q = payload["question"] as? String ?? "需要你确认执行"
            notifyIfBackground(title: "VoiceSign · 需要确认", body: q, taskId: taskId)
        case "done":
            notifyIfBackground(title: "VoiceSign · 任务完成", body: "回执已到，可查看执行结果", taskId: taskId)
        case "failed":
            let e = payload["error"] as? String ?? "任务失败"
            notifyIfBackground(title: "VoiceSign · 任务失败", body: String(e.prefix(80)), taskId: taskId)
        case "canceled":
            notifyIfBackground(title: "VoiceSign · 任务已取消", body: "任务已取消", taskId: taskId)
        default:
            break
        }
    }

    /// App 是否处于后台/锁屏（前台不打扰）。
    private var isAppBackgrounded: Bool {
        #if canImport(UIKit)
        return UIApplication.shared.applicationState != .active
        #else
        return false
        #endif
    }
}
