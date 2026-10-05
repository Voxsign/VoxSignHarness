//
//  UIVerify3.swift — 临时验收辅助测试 第 3 轮（非产品代码）
//  按住说话手势：等外部窗口后按住 mic 12s（红条由外部 simctl io 抓屏），松手后观察不崩溃。
//

import XCTest

final class UIVerify3: XCTestCase {

    let app = XCUIApplication()

    override func setUp() {
        continueAfterFailure = true
        app.launch()
    }

    func testPressToTalkGesture() {
        let mic = app.buttons["vhs.mic"]
        guard mic.waitForExistence(timeout: 25) else { return }
        // 外部抓屏窗口：先睡 8s，再按住 12s（外部在中间用 simctl screenshot 抓红条）。
        sleep(8)
        mic.press(forDuration: 12)
        sleep(1)
        let att = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        att.name = "18-after-release"
        att.lifetime = .keepAlways
        add(att)
        // 松手后不应崩溃；再观察 2s 语音状态条。
        sleep(2)
    }
}
