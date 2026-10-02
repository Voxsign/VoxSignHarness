//
//  SettingsStore.swift
//  VoiceSign
//
//  服务设置持久化（等价 web localStorage 的 vhs-web-settings）：server 地址 + Bearer token。
//

import Foundation

/// 观测式设置仓库：App 启动时读 UserDefaults，保存时回写。
final class SettingsStore: ObservableObject {
    static let shared = SettingsStore()

    private let defaults = UserDefaults.standard
    private let baseKey = "vhs-ios-base"
    private let tokenKey = "vhs-ios-token"

    @Published var base: String {
        didSet { defaults.set(base, forKey: baseKey) }
    }
    @Published var token: String {
        didSet { defaults.set(token, forKey: tokenKey) }
    }

    init() {
        let storedBase = defaults.string(forKey: baseKey) ?? ""
        let storedToken = defaults.string(forKey: tokenKey) ?? ""
        // Debug 预填 M7 真机冒烟 server（免手输；设置页仍可改）；Release 回退本机回环。
        #if DEBUG
        let defaultBase = "http://192.168.8.129:8897"
        let defaultToken = "m7-token"
        #else
        let defaultBase = "http://127.0.0.1:8765"
        let defaultToken = ""
        #endif
        self.base = storedBase.isEmpty ? defaultBase : storedBase
        self.token = storedToken.isEmpty ? defaultToken : storedToken
    }

    /// 去掉末尾斜杠，拼出绝对 URL（path 以 / 开头）。
    func url(_ path: String) -> URL? {
        let clean = base.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        return URL(string: clean + path)
    }
}
