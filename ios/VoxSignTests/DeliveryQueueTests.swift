//
//  DeliveryQueueTests.swift
//  VoxSignTests
//
//  T1 投递队列单元测试：入队/持久化/补投/退避/幂等键。
//

import XCTest
@testable import VoxSign

final class DeliveryQueueTests: XCTestCase {

    private var tempDir: URL!
    private var queue: DeliveryQueue!

    override func setUpWithError() throws {
        tempDir = FileManager.default.temporaryDirectory
            .appendingPathComponent("vhs-dq-test-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: tempDir, withIntermediateDirectories: true)
        queue = DeliveryQueue()
        queue.storageDirectory = tempDir
        queue.clearAll()
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: tempDir)
    }

    func testEnqueueAndCount() {
        XCTAssertTrue(queue.isEmpty)
        queue.enqueue(PendingSubmission(text: "查一下 docs", mode: "voice"))
        XCTAssertEqual(queue.count, 1)
        queue.enqueue(PendingSubmission(text: "整理文件清单", mode: "text"))
        XCTAssertEqual(queue.count, 2)
        XCTAssertEqual(queue.pending.first?.text, "查一下 docs") // FIFO
    }

    func testPersistenceAcrossReload() {
        queue.enqueue(PendingSubmission(text: "第一条", mode: "text"))
        queue.enqueue(PendingSubmission(text: "第二条", mode: "voice"))

        let reloaded = DeliveryQueue()
        reloaded.storageDirectory = tempDir
        XCTAssertEqual(reloaded.count, 2, "重新加载后应恢复持久化条目")
        XCTAssertEqual(reloaded.pending.map(\.text), ["第一条", "第二条"])
    }

    func testFlushSuccessRemovesAll() async {
        var submitted: [String] = []
        queue.submitter = { item in submitted.append(item.text) }
        queue.enqueue(PendingSubmission(text: "a", mode: "text"))
        queue.enqueue(PendingSubmission(text: "b", mode: "voice"))

        let n = await queue.flush()
        XCTAssertEqual(n, 2)
        XCTAssertEqual(submitted, ["a", "b"])
        XCTAssertTrue(queue.isEmpty)
    }

    func testFlushFailureKeepsOrderAndBackoff() async {
        var calls = 0
        queue.submitter = { _ in
            calls += 1
            throw APIError.transport("网络不可达")
        }
        queue.enqueue(PendingSubmission(text: "a", mode: "text"))
        queue.enqueue(PendingSubmission(text: "b", mode: "voice"))

        let n = await queue.flush()
        XCTAssertEqual(n, 0, "首条失败应整轮停止（保序）")
        XCTAssertEqual(queue.count, 2, "失败条目应保留")
        XCTAssertEqual(queue.pending[0].retryCount, 1, "失败计数 +1")

        // 退避：刚失败后立刻 flush 应返回 0（不风暴）
        let again = await queue.flush()
        XCTAssertEqual(again, 0, "退避窗口内不应重试")
    }

    func testFlushRecoversAfterBackoff() async {
        var calls = 0
        queue.maxBackoffSeconds = 0 // 关闭退避以便复测
        queue.submitter = { _ in
            calls += 1
            if calls == 1 { throw APIError.transport("临时失败") }
        }
        queue.enqueue(PendingSubmission(text: "a", mode: "text"))
        _ = await queue.flush()
        XCTAssertEqual(queue.count, 1)

        let n = await queue.flush()
        XCTAssertEqual(n, 1, "第二次 flush 应补投成功")
        XCTAssertTrue(queue.isEmpty)
    }

    func testMaxStoredDropsOldest() {
        queue.maxStored = 3
        for i in 0..<5 {
            queue.enqueue(PendingSubmission(text: "t\(i)", mode: "text"))
        }
        XCTAssertEqual(queue.count, 3)
        XCTAssertEqual(queue.pending.map(\.text), ["t2", "t3", "t4"], "超限丢最旧")
    }
}
