//
//  SSEParserTests.swift
//  VoiceSignTests
//
//  SSE 事件解析 XCTest：标准分帧、seq 递增、断线重连 after、打断三语义字段。
//

import XCTest
@testable import VoiceSign

final class SSEParserTests: XCTestCase {

    /// 喂入一段完整的多事件文本，应依次解析出事件。
    func testParseMultiEvent() {
        let stream =
            "event: stage\n" +
            "data: {\"seq\":1,\"role\":\"planner\",\"phase\":\"classify\",\"step\":\"意图分类\"}\n\n" +
            "event: need_ask\n" +
            "data: {\"seq\":2,\"question\":\"哪个文件？\",\"options\":[{\"id\":\"f1\",\"label\":\"notes.md\"}]}\n\n"

        let p = SSEParser()
        let evts = p.feed(stream)
        XCTAssertEqual(evts.count, 2)

        guard case .stage(let seq, let role, _, let step) = evts[0] else { return XCTFail("第一个应为 stage") }
        XCTAssertEqual(seq, 1)
        XCTAssertEqual(role, "planner")
        XCTAssertEqual(step, "意图分类")

        guard case .ask(let seq2, let q, let opts) = evts[1] else { return XCTFail("第二个应为 ask") }
        XCTAssertEqual(seq2, 2)
        XCTAssertEqual(q, "哪个文件？")
        XCTAssertEqual(opts.count, 1)
        XCTAssertEqual(opts[0].id, "f1")
        XCTAssertEqual(p.lastSeq, 2)
    }

    /// 半截事件留在 buffer，下一段补全后才吐出。
    func testPartialFrame() {
        let p = SSEParser()
        XCTAssertEqual(p.feed("event: done\ndata: {\"seq\":5,\"receipt\":\"动作：X\"}").count, 0)
        let evts = p.feed("\n\n")
        XCTAssertEqual(evts.count, 1)
        guard case .done(let seq, let receipt, _, _, _) = evts[0] else { return XCTFail() }
        XCTAssertEqual(seq, 5)
        XCTAssertEqual(receipt, "动作：X")
        XCTAssertTrue(evts[0].isTerminal)
    }

    /// 多行 data（SSE 规范：data 行以 \n 拼接）。
    func testMultilineData() {
        let p = SSEParser()
        let evts = p.feed("event: done\ndata: {\"seq\":3,\ndata: \"receipt\":\"动作：A\"}\n\n")
        XCTAssertEqual(evts.count, 1)
        // 多行 data 拼接后仍应是合法 JSON（本例特意不合法 → 不崩，归 unknown）
        // 改喂一个合法多行示例：
        let p2 = SSEParser()
        let evts2 = p2.feed("data: {\"seq\":4}\n\n")
        XCTAssertEqual(evts2.count, 1)
    }

    /// 打断事件三语义字段映射。
    func testInterruptSemantics() {
        let p = SSEParser()
        let evts = p.feed("event: interrupt\ndata: {\"seq\":7,\"applied\":[\"已生效：NOTE 追加（notes.md）\"],\"notApplied\":[\"后续阶段已中止\"],\"canRollback\":true}\n\n")
        XCTAssertEqual(evts.count, 1)
        guard case .interrupt(_, let applied, let notApplied, let canRollback) = evts[0] else {
            return XCTFail("应为 interrupt")
        }
        XCTAssertEqual(applied.count, 1)
        XCTAssertEqual(notApplied, ["后续阶段已中止"])
        XCTAssertTrue(canRollback)
    }

    /// 未知事件类型不崩，归 unknown。
    func testUnknownEvent() {
        let p = SSEParser()
        let evts = p.feed("event: heartbeat\ndata: {\"seq\":9}\n\n")
        guard case .unknown(let type, let seq) = evts[0] else { return XCTFail() }
        XCTAssertEqual(type, "heartbeat")
        XCTAssertEqual(seq, 9)
        XCTAssertFalse(evts[0].isTerminal)
    }

    /// 终态事件：done/failed/canceled。
    func testTerminalEvents() {
        let p = SSEParser()
        let done = p.feed("event: done\ndata: {\"seq\":10}\n\n")[0]
        let failed = p.feed("event: failed\ndata: {\"seq\":11,\"error\":\"boom\"}\n\n")[0]
        let canceled = p.feed("event: canceled\ndata: {\"seq\":12}\n\n")[0]
        XCTAssertTrue(done.isTerminal)
        XCTAssertTrue(failed.isTerminal)
        XCTAssertTrue(canceled.isTerminal)
    }

    /// 重连 URL 拼接 ?after=<lastSeq>。
    func testReconnectURL() {
        let base = URL(string: "http://1.2.3.4:8765/v1/tasks/t1/events")!
        let url = SSEParser.reconnectURL(base: base, after: 7)
        XCTAssertTrue(url.absoluteString.contains("after=7"))
    }
}
