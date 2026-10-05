//
//  SessionStoreTests.swift
//  VoiceSignTests
//
//  v2.4 会话仓库单测：初始化幂等 / 增删切 / 跨会话消息隔离 / 删最后一个被拒 / 持久化重载。
//  用独立 UserDefaults suite 隔离，避免污染 standard。
//

import XCTest
@testable import VoiceSign

final class SessionStoreTests: XCTestCase {

    private var suiteName: String!

    override func setUpWithError() throws {
        suiteName = "vhs-session-test-\(UUID().uuidString)"
    }

    override func tearDownWithError() throws {
        UserDefaults.standard.removePersistentDomain(forName: suiteName)
        suiteName = nil
    }

    private func makeStore() -> SessionStore {
        // 每次都新建实例，模拟"进程内重载"。
        SessionStore(defaults: UserDefaults(suiteName: suiteName)!)
    }

    // MARK: - ensureInitialSession 幂等

    func testEnsureInitialSessionIdempotent() {
        let store = makeStore()
        XCTAssertEqual(store.sessions.count, 0, "全新仓库应为空")
        store.ensureInitialSession()
        XCTAssertEqual(store.sessions.count, 1)
        XCTAssertEqual(store.sessions.first?.title, "新会话")
        XCTAssertEqual(store.currentSessionID, store.sessions.first?.id)

        // 再次调用不得重复创建。
        store.ensureInitialSession()
        XCTAssertEqual(store.sessions.count, 1, "ensureInitialSession 幂等")
    }

    // MARK: - create / switch / delete

    func testCreateSwitchDelete() {
        let store = makeStore()
        store.ensureInitialSession()
        let first = store.currentSession!

        let second = store.createSession(title: "工作")
        XCTAssertEqual(store.sessions.count, 2)
        XCTAssertEqual(store.currentSessionID, second.id, "新建即切当前")

        store.switchTo(id: first.id)
        XCTAssertEqual(store.currentSessionID, first.id)

        // 删除非当前会话。
        XCTAssertTrue(store.deleteSession(id: second.id))
        XCTAssertEqual(store.sessions.count, 1)
        XCTAssertEqual(store.currentSessionID, first.id, "删非当前会话不影响当前")
    }

    func testDeleteCurrentSessionFallsBack() {
        let store = makeStore()
        store.ensureInitialSession()
        let first = store.currentSession!
        let second = store.createSession(title: "临时")

        // 当前是 second，删除 second → 回退到剩余第一个。
        XCTAssertEqual(store.currentSessionID, second.id)
        XCTAssertTrue(store.deleteSession(id: second.id))
        XCTAssertEqual(store.sessions.count, 1)
        XCTAssertEqual(store.currentSessionID, first.id, "删当前会话后切到剩余第一个")
    }

    // MARK: - 跨会话消息隔离

    func testCrossSessionMessageIsolation() {
        let store = makeStore()
        store.ensureInitialSession()
        let a = store.currentSession!
        let b = store.createSession(title: "会话B")

        // 会话 A 存消息。
        store.saveMessages([StoredMessage(id: "m-a1", role: "user", text: "A 的消息")], for: a.id)

        // 切到 B 存不同消息。
        store.switchTo(id: b.id)
        store.saveMessages([StoredMessage(id: "m-b1", role: "user", text: "B 的消息"),
                            StoredMessage(id: "m-b2", role: "harness", text: "B 的回复")], for: b.id)

        // 切回 A：数据完好，未被 B 污染。
        store.switchTo(id: a.id)
        let aMsgs = store.loadMessages(for: a.id)
        XCTAssertEqual(aMsgs.count, 1)
        XCTAssertEqual(aMsgs.first?.text, "A 的消息")

        // B 数据独立。
        let bMsgs = store.loadMessages(for: b.id)
        XCTAssertEqual(bMsgs.count, 2)
        XCTAssertEqual(bMsgs.first?.text, "B 的消息")
    }

    // MARK: - 删最后一个会话被拒

    func testDeleteLastSessionRejected() {
        let store = makeStore()
        store.ensureInitialSession()
        XCTAssertEqual(store.sessions.count, 1)
        let only = store.currentSession!

        XCTAssertFalse(store.deleteSession(id: only.id), "仅剩一个会话时删除应被拒绝")
        XCTAssertEqual(store.sessions.count, 1, "拒绝后会话数不变")
        XCTAssertEqual(store.currentSessionID, only.id)
    }

    // MARK: - 持久化重载

    func testPersistenceReload() {
        let store = makeStore()
        store.ensureInitialSession()
        let b = store.createSession(title: "持久化测试")
        store.saveMessages([StoredMessage(id: "m1", role: "user", text: "要持久化的消息")], for: b.id)
        store.switchTo(id: b.id)

        // 新建实例读回（同一 suite）。
        let reloaded = SessionStore(defaults: UserDefaults(suiteName: suiteName)!)
        XCTAssertEqual(reloaded.sessions.count, 2, "重载后会话数恢复")
        XCTAssertEqual(reloaded.currentSessionID, b.id, "重载后当前会话 id 恢复")
        let msgs = reloaded.loadMessages(for: b.id)
        XCTAssertEqual(msgs.count, 1)
        XCTAssertEqual(msgs.first?.text, "要持久化的消息")
        XCTAssertEqual(reloaded.sessions.first(where: { $0.id == b.id })?.title, "持久化测试")
    }

    // MARK: - rename / touch

    func testRenameAndTouch() {
        let store = makeStore()
        store.ensureInitialSession()
        let id = store.currentSessionID
        store.renameSession(id: id, title: "新标题")
        XCTAssertEqual(store.sessions.first?.title, "新标题")

        let before = store.sessions.first?.updatedAt ?? Date.distantPast
        // touch 刷新 updatedAt。
        Thread.sleep(forTimeInterval: 0.05)
        store.touch(sessionID: id)
        let after = store.sessions.first?.updatedAt ?? Date.distantPast
        XCTAssertGreaterThan(after, before)
    }
}
