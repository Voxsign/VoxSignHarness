//
//  CostTextTests.swift
//  VoxSignTests
//
//  消耗用量文案纯函数单测：服务端不回传（nil）时隐藏；回传时格式化 "消耗 n"。
//

import XCTest
@testable import VoxSign

final class CostTextTests: XCTestCase {

    // 服务端不回传（nil）→ 整行隐藏。
    func testNilHidesRow() {
        XCTAssertNil(CostText.caption(for: nil))
    }

    // 0 也如实显示（回传 0 token）。
    func testZeroFormatted() {
        XCTAssertEqual(CostText.caption(for: 0), "消耗 0")
    }

    // 常规用量。
    func testTypicalTokens() {
        XCTAssertEqual(CostText.caption(for: 123), "消耗 123")
    }

    // 大批量用量数字。
    func testLargeTokens() {
        XCTAssertEqual(CostText.caption(for: 987654), "消耗 987654")
    }
}
