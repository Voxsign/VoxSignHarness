//
//  TopBarDotTests.swift
//  VoiceSignTests
//
//  v2.4 顶栏状态点纯函数单测：连接态 × harness 态全组合映射。
//

import XCTest
@testable import VoiceSign

final class TopBarDotTests: XCTestCase {

    // offline 恒红（不论 harness 态）。
    func testOfflineAlwaysRed() {
        XCTAssertEqual(TopBarDot.tone(conn: .offline, harness: .idle), .red)
        XCTAssertEqual(TopBarDot.tone(conn: .offline, harness: .busy), .red)
        XCTAssertEqual(TopBarDot.tone(conn: .offline, harness: .decision), .red)
    }

    // online：decision → 橙。
    func testOnlineDecisionOrange() {
        XCTAssertEqual(TopBarDot.tone(conn: .online, harness: .decision), .orange)
    }

    // online：busy / idle → 蓝。
    func testOnlineBusyAndIdleBlue() {
        XCTAssertEqual(TopBarDot.tone(conn: .online, harness: .busy), .blue)
        XCTAssertEqual(TopBarDot.tone(conn: .online, harness: .idle), .blue)
    }

    // reconnecting / unknown → 灰（不论 harness 态）。
    func testReconnectingAndUnknownGray() {
        for conn: ConnectionState in [.reconnecting, .unknown] {
            XCTAssertEqual(TopBarDot.tone(conn: conn, harness: .idle), .gray)
            XCTAssertEqual(TopBarDot.tone(conn: conn, harness: .busy), .gray)
            XCTAssertEqual(TopBarDot.tone(conn: conn, harness: .decision), .gray)
        }
    }
}
