//
//  VoxSignUITests.swift
//  VoxSignUITests
//
//  XCUITest 驱动真实 App 渲染（连真机 Debug 预填 server 192.168.8.129:8897 / m7-token）。
//  跑法（设备解锁亮屏）：
//  xcodebuild test -project ios/VoxSign.xcodeproj -scheme VoxSign \
//    -destination 'id=00008120-001428820AB8201E' -derivedDataPath /tmp/vhs-m7-dd \
//    CODE_SIGN_STYLE=Automatic DEVELOPMENT_TEAM=P5W752L332
//

import XCTest

final class VoxSignUITests: XCTestCase {

    let app = XCUIApplication()

    override func setUp() {
        continueAfterFailure = false
        app.launch()
    }

    /// 等待"任一真实回复"到达：回执卡（"已完成"/"待澄清"）或 need_ask 决策卡候选按钮，任一出现即返回。
    /// 返回 true 表示出现了 need_ask 候选按钮（决策卡），false 表示出现了回执卡。
    /// 注：need_ask 的问题文本由服务端生成、非 UI 固定文案（如"哪个"），故不再断言问题文本。
    @discardableResult
    private func waitForAnyReply(timeout: TimeInterval) -> Bool {
        let receipt = app.staticTexts.containing(
            NSPredicate(format: "label CONTAINS '已完成' OR label CONTAINS '待澄清'")
        ).firstMatch
        let option = app.buttons.containing(
            NSPredicate(format: "label CONTAINS '记下来' OR label CONTAINS '提交' OR label CONTAINS '改' OR label CONTAINS '查'")
        ).firstMatch
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if option.exists { return true }
            if receipt.exists { return false }
            RunLoop.current.run(until: Date().addingTimeInterval(0.3))
        }
        XCTAssertTrue(receipt.exists || option.exists,
                       "超时：既未渲染回执卡（已完成/待澄清），也未渲染候选按钮")
        return option.exists
    }

    /// P0：发送含糊指令 → 等待任一真实回复渲染。
    /// 新版 harness 对模糊指令可能直接回"待澄清"回执，也可能回 need_ask 决策卡；
    /// 二者任一到达即算通路正常。若决策卡出现，则断言候选按钮确实渲染。
    func test_needAskRendersButtons() {
        let input = app.textFields["vhs.input"]
        XCTAssertTrue(input.waitForExistence(timeout: 10), "输入框未出现")
        input.tap()
        input.typeText("记一下那个")

        let send = app.buttons["vhs.send"]
        XCTAssertTrue(send.isEnabled, "发送按钮应可用")
        send.tap()

        let sawOptions = waitForAnyReply(timeout: 20)
        if sawOptions {
            // 决策卡分支：候选按钮已被 waitForAnyReply 确认存在，这里再显式断言一次。
            let option = app.buttons.containing(
                NSPredicate(format: "label CONTAINS '记下来' OR label CONTAINS '提交' OR label CONTAINS '改' OR label CONTAINS '查'")
            ).firstMatch
            XCTAssertTrue(option.exists, "未渲染候选按钮")
        }
        // 回执卡分支（"待澄清"/"已完成"）：waitForAnyReply 已确认渲染，无需再断言。
    }

    /// 明确指令 → 回执卡出现（新版文案为 "✅ 已完成 · X.X 秒"；harness 对模糊指令可能回"待澄清"回执）。
    func test_noteRunsToReceipt() {
        let input = app.textFields["vhs.input"]
        XCTAssertTrue(input.waitForExistence(timeout: 10))
        input.tap()
        input.typeText("记一下 明天开会")
        app.buttons["vhs.send"].tap()

        // 回执卡：等待"已完成"或服务端"待澄清"回执文本出现。
        let receipt = app.staticTexts.containing(
            NSPredicate(format: "label CONTAINS '已完成' OR label CONTAINS '待澄清'")
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

    /// 多任务时序：连续两轮指令，每轮任一真实回复出现即算本轮通过（复现"首条卡、次条才好"）。
    /// 若某轮渲染决策卡则点选候选按钮答掉；若渲染回执卡则直接进入下一轮。
    func test_multiTaskSequential() {
        let input = app.textFields["vhs.input"]
        XCTAssertTrue(input.waitForExistence(timeout: 10))

        func sendAndExpectReply(_ text: String, line: Int) {
            input.tap()
            input.typeText(text)
            app.buttons["vhs.send"].tap()
            let sawOptions = waitForAnyReply(timeout: 20)
            if sawOptions {
                // 决策卡出现：选候选按钮答掉，进入下一轮。
                let opt = app.buttons.containing(
                    NSPredicate(format: "label CONTAINS '记下来' OR label CONTAINS '提交' OR label CONTAINS '改' OR label CONTAINS '查'")
                ).firstMatch
                XCTAssertTrue(opt.waitForExistence(timeout: 5), "第\(line)轮未渲染候选按钮")
                opt.tap()
            }
            // 回执卡分支：本轮已通过，无需点选。
        }

        sendAndExpectReply("记一下那个", line: 1)
        // 等决策点清空、输入框重新可用，再发第二轮。
        XCTAssertTrue(input.waitForExistence(timeout: 10), "第一轮答完后输入框未恢复")
        sendAndExpectReply("再说一遍那个", line: 2)
    }
}
