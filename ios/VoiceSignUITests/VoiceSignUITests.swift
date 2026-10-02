//
//  VoiceSignUITests.swift
//  VoiceSignUITests
//
//  XCUITest 驱动真实 App 渲染（连真机 Debug 预填 server 192.168.8.129:8897 / m7-token）。
//  跑法（设备解锁亮屏）：
//  xcodebuild test -project ios/VoiceSign.xcodeproj -scheme VoiceSign \
//    -destination 'id=00008120-001428820AB8201E' -derivedDataPath /tmp/vhs-m7-dd \
//    CODE_SIGN_STYLE=Automatic DEVELOPMENT_TEAM=P5W752L332
//

import XCTest

final class VoiceSignUITests: XCTestCase {

    let app = XCUIApplication()

    override func setUp() {
        continueAfterFailure = false
        app.launch()
    }

    /// P0：含糊指令 → 等待 → 断言问题文本 + 候选按钮渲染。
    func test_needAskRendersButtons() {
        let input = app.textFields["vhs.input"]
        XCTAssertTrue(input.waitForExistence(timeout: 10), "输入框未出现")
        input.tap()
        input.typeText("记一下那个")

        let send = app.buttons["vhs.send"]
        XCTAssertTrue(send.isEnabled, "发送按钮应可用")
        send.tap()

        // 等待决策点：问题文本 + 至少一个候选按钮出现。
        // server 会把"那个"判为 UNKNOWN → need_ask（4 个 options）。
        let question = app.staticTexts.containing(
            NSPredicate(format: "label CONTAINS '哪个'")
        ).firstMatch
        XCTAssertTrue(question.waitForExistence(timeout: 15),
                      "未渲染 need_ask 问题文本（诊断：看 App 底部 diagLine / diag-*.log）")

        // 候选按钮：label 为 edit/query/note/commit 的中文 label（改笔记/查一下/记下来/提交）。
        let option = app.buttons.containing(
            NSPredicate(format: "label CONTAINS '记下来' OR label CONTAINS '提交' OR label CONTAINS '改' OR label CONTAINS '查'")
        ).firstMatch
        XCTAssertTrue(option.waitForExistence(timeout: 5), "未渲染候选按钮")
    }

    /// 明确指令 → 绿色回执卡出现。
    func test_noteRunsToReceipt() {
        let input = app.textFields["vhs.input"]
        XCTAssertTrue(input.waitForExistence(timeout: 10))
        input.tap()
        input.typeText("记一下 明天开会")
        app.buttons["vhs.send"].tap()

        // 回执/完成：等待"撤销"或"已追加"/receipt 相关文本出现。
        let receipt = app.staticTexts.containing(
            NSPredicate(format: "label CONTAINS '撤销' OR label CONTAINS '已追加' OR label CONTAINS 'OK'")
        ).firstMatch
        XCTAssertTrue(receipt.waitForExistence(timeout: 20),
                      "未走到回执卡（可能中途有 need_ask 未答）")
    }

    /// P2/标点：文本路径注入后断言识别/输入回显——
    /// UI 自动化无法真机注入语音（Speech 权限+语音），故用文本路径验证输入框回显可发送。
    func test_punctuationViaTextPath() {
        let input = app.textFields["vhs.input"]
        XCTAssertTrue(input.waitForExistence(timeout: 10))
        input.tap()
        // 直接在文本框输入带标点的句子（绕过 ASR，验证输入路径不丢标点）。
        input.typeText("记一下冀总厂房下周一，顺便问问他下周三能不能开会？")
        XCTAssertEqual(input.value as? String,
                       "记一下冀总厂房下周一，顺便问问他下周三能不能开会？",
                       "文本输入路径标点应原样保留")
        // 注：ASR 端 addsPunctuation 出标点由真机语音验证覆盖（无法 UI 自动化注入语音）。
    }

    /// 多任务时序：连续两轮含糊指令，每轮都必须渲染候选按钮（复现"首条卡、次条才好"）。
    func test_multiTaskSequential() {
        let input = app.textFields["vhs.input"]
        XCTAssertTrue(input.waitForExistence(timeout: 10))

        func sendAndExpectButtons(_ text: String, line: Int) {
            input.tap()
            input.typeText(text)
            app.buttons["vhs.send"].tap()
            let q = app.staticTexts.containing(
                NSPredicate(format: "label CONTAINS '哪个'")
            ).firstMatch
            XCTAssertTrue(q.waitForExistence(timeout: 15), "第\(line)轮未渲染问题文本")
            let opt = app.buttons.containing(
                NSPredicate(format: "label CONTAINS '记下来' OR label CONTAINS '提交' OR label CONTAINS '改' OR label CONTAINS '查'")
            ).firstMatch
            XCTAssertTrue(opt.waitForExistence(timeout: 5), "第\(line)轮未渲染候选按钮")
            // 选"记下来"答掉，进入下一轮。
            opt.tap()
        }

        sendAndExpectButtons("记一下那个", line: 1)
        // 等决策点清空、输入框重新可用，再发第二轮。
        XCTAssertTrue(input.waitForExistence(timeout: 10), "第一轮答完后输入框未恢复")
        sendAndExpectButtons("再说一遍那个", line: 2)
    }
}
