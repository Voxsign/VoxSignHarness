//
//  SessionStore.swift
//  VoxSign
//
//  v2.4 多会话仓库：本地存储 + 会话列表（默认隐藏）。
//  UserDefaults JSON 持久化（key "vhs-ios-sessions"），读写范式对齐 Net/SettingsStore.swift。
//  观测式：App 启动读回，所有变更方法内显式 persist()。
//

import Foundation
import Combine

/// 持久化载荷：整个 sessions 数组 + 当前会话 id。
private struct PersistedState: Codable, Equatable {
    var sessions: [ChatSession]
    var currentID: String
}

/// 会话仓库（单例）。线程模型：与 AppModel 一致，主 actor / 串行访问下调用。
final class SessionStore: ObservableObject {
    static let shared = SessionStore()

    private let defaults: UserDefaults
    private let key = "vhs-ios-sessions"
    private let containerKey = "vhs-ios-containers"

    /// 会话列表（顺序即创建顺序；列表排序由视图按 updatedAt 决定）。
    @Published var sessions: [ChatSession] = []
    /// 当前会话 id。
    @Published var currentSessionID: String = ""
    /// V6.2 容器（角色/域）列表。
    @Published var containers: [ContainerItem] = []

    /// - Parameter defaults: 注入便于单测隔离（默认 .standard）。
    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        if let data = defaults.data(forKey: key),
           let persisted = try? JSONDecoder().decode(PersistedState.self, from: data) {
            sessions = persisted.sessions
            // 兼容老数据：当前 id 失效时回退到第一个会话。
            if sessions.contains(where: { $0.id == persisted.currentID }) {
                currentSessionID = persisted.currentID
            } else {
                currentSessionID = sessions.first?.id ?? ""
            }
        }
        // V6.2 容器独立持久化（key 分开，不动旧 sessions 载荷）。
        if let cdata = defaults.data(forKey: containerKey),
           let cs = try? JSONDecoder().decode([ContainerItem].self, from: cdata) {
            containers = cs
        }
    }

    // MARK: - 会话生命周期

    /// 无会话时创建"新会话"并设为当前；幂等。
    func ensureInitialSession() {
        guard sessions.isEmpty else { return }
        let s = makeSession(title: "新会话")
        sessions.append(s)
        currentSessionID = s.id
        persist()
    }

    /// 新建会话（追加 + 切当前 + 持久化）。
    @discardableResult
    func createSession(title: String = "新会话") -> ChatSession {
        let s = makeSession(title: title)
        sessions.append(s)
        currentSessionID = s.id
        persist()
        return s
    }

    /// V6.2 新建归属会话：挂到指定容器（角色/域）之下。
    @discardableResult
    func createSession(title: String = "新会话",
                       containerKind: ContainerKind,
                       containerID: String) -> ChatSession {
        let s = makeSession(title: title,
                            containerKind: containerKind,
                            containerID: containerID)
        sessions.append(s)
        currentSessionID = s.id
        persist()
        return s
    }

    // MARK: - V6.2 容器（角色/域）

    /// 容器列表按 kind 过滤。
    func containers(of kind: ContainerKind) -> [ContainerItem] {
        containers.filter { $0.kind == kind }
    }

    /// 创建或复用容器（同名同 kind 幂等返回已有）。持久化。
    @discardableResult
    func upsertContainer(kind: ContainerKind, name: String) -> ContainerItem {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        let finalName = trimmed.isEmpty ? (kind == .role ? "新角色" : "新域") : trimmed
        if let hit = containers.first(where: { $0.kind == kind && $0.name == finalName }) {
            return hit
        }
        let c = ContainerItem(id: UUID().uuidString, kind: kind, name: finalName)
        containers.append(c)
        persistContainers()
        return c
    }

    /// 删除容器（其下会话同时删除；至少保留一个会话时允许删除容器）。
    @discardableResult
    func deleteContainer(id: String) -> Bool {
        guard let c = containers.first(where: { $0.id == id }) else { return false }
        let childIDs = sessions.filter { $0.containerID == id }.map { $0.id }
        containers.removeAll { $0.id == id }
        sessions.removeAll { $0.containerID == id }
        if childIDs.contains(currentSessionID) {
            currentSessionID = sessions.first?.id ?? ""
        }
        persistContainers()
        persist()
        return true
    }

    /// 删除会话；sessions.count <= 1 时拒绝（返回 false）。
    @discardableResult
    func deleteSession(id: String) -> Bool {
        guard sessions.count > 1 else { return false }
        guard let idx = sessions.firstIndex(where: { $0.id == id }) else { return false }
        sessions.remove(at: idx)
        if currentSessionID == id {
            currentSessionID = sessions.first?.id ?? ""
        }
        persist()
        return true
    }

    func renameSession(id: String, title: String) {
        guard let idx = sessions.firstIndex(where: { $0.id == id }) else { return }
        sessions[idx].title = title
        sessions[idx].updatedAt = Date()
        persist()
    }

    // MARK: - V6.3 先聊后归：会话归类/移回

    /// 把会话归入指定容器（角色/域）。
    func setContainer(sessionID: String, kind: ContainerKind, containerID: String) {
        guard let idx = sessions.firstIndex(where: { $0.id == sessionID }) else { return }
        sessions[idx].containerKind = kind
        sessions[idx].containerID = containerID
        sessions[idx].updatedAt = Date()
        persist()
    }

    /// 移回未分组（清空归属）。
    func clearContainer(sessionID: String) {
        guard let idx = sessions.firstIndex(where: { $0.id == sessionID }) else { return }
        sessions[idx].containerKind = nil
        sessions[idx].containerID = nil
        sessions[idx].updatedAt = Date()
        persist()
    }

    /// 切当前 + 持久化。
    func switchTo(id: String) {
        guard sessions.contains(where: { $0.id == id }) else { return }
        currentSessionID = id
        persist()
    }

    // MARK: - 消息

    /// 覆盖写回某会话的消息，并刷新 updatedAt。
    func saveMessages(_ messages: [StoredMessage], for sessionID: String) {
        guard let idx = sessions.firstIndex(where: { $0.id == sessionID }) else { return }
        sessions[idx].messages = messages
        sessions[idx].updatedAt = Date()
        persist()
    }

    func loadMessages(for sessionID: String) -> [StoredMessage] {
        sessions.first { $0.id == sessionID }?.messages ?? []
    }

    /// 标记某会话有更新（updatedAt = now）+ 持久化。
    func touch(sessionID: String) {
        guard let idx = sessions.firstIndex(where: { $0.id == sessionID }) else { return }
        sessions[idx].updatedAt = Date()
        persist()
    }

    // MARK: - 便捷访问

    var currentSession: ChatSession? {
        sessions.first { $0.id == currentSessionID } ?? sessions.first
    }

    // MARK: - 内部

    private func makeSession(title: String) -> ChatSession {
        ChatSession(id: UUID().uuidString,
                    title: title,
                    createdAt: Date(),
                    updatedAt: Date(),
                    messages: [])
    }

    private func makeSession(title: String,
                             containerKind: ContainerKind,
                             containerID: String) -> ChatSession {
        ChatSession(id: UUID().uuidString,
                    title: title,
                    createdAt: Date(),
                    updatedAt: Date(),
                    messages: [],
                    containerKind: containerKind,
                    containerID: containerID)
    }

    private func persist() {
        let state = PersistedState(sessions: sessions, currentID: currentSessionID)
        if let data = try? JSONEncoder().encode(state) {
            defaults.set(data, forKey: key)
        }
    }

    private func persistContainers() {
        if let data = try? JSONEncoder().encode(containers) {
            defaults.set(data, forKey: containerKey)
        }
    }
}
