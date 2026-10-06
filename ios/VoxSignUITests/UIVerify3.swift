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

//
//  V4Screenshots — 临时验收测试（非产品代码）：V4 三态截图。
//  跑法：
//   TEST_TARGET_NAME=VoxSign xcodebuild test -project VoxSign.xcodeproj \
//     -scheme VoxSign -destination 'id=61BF6DC5-ABAA-4ED9-BEA9-0656F6A4590E' \
//     -derivedDataPath /tmp/vhs-v4-dd -resultBundlePath /tmp/vhs-v4-ui.xcresult \
//     -only-testing:VoxSignUITests/V4Screenshots/<方法名>
//  通知/麦克风弹窗为 SpringBoard 级：interruption monitor + 主动查 springboard 按钮。
//

import XCTest

final class V4Screenshots: XCTestCase {

    let app = XCUIApplication()
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")

    override func setUp() {
        continueAfterFailure = true
        // 权限弹窗：通知点 Don't Allow、麦克风/语音识别点 Allow，均为永久决策。
        addUIInterruptionMonitor(withDescription: "Permission alert") { alert in
            if alert.buttons["Don’t Allow"].exists { alert.buttons["Don’t Allow"].tap(); return true }
            if alert.buttons["Don't Allow"].exists { alert.buttons["Don't Allow"].tap(); return true }
            if alert.buttons["Allow"].exists { alert.buttons["Allow"].tap(); return true }
            if alert.buttons["OK"].exists { alert.buttons["OK"].tap(); return true }
            return false
        }
        app.launch()
        // 兜底：主动点 SpringBoard 上的弹窗按钮（interruption monitor 未触发时）。
        sleep(1)
        for _ in 0..<5 {
            let dont = springboard.buttons["Don’t Allow"]
            let dont2 = springboard.buttons["Don't Allow"]
            let allow = springboard.buttons["Allow"]
            if dont.exists { dont.tap(); break }
            if dont2.exists { dont2.tap(); break }
            if allow.exists { allow.tap(); break }
            sleep(1)
        }
    }

    func shot(_ name: String) {
        sleep(1)
        let att = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        att.name = name
        att.lifetime = .keepAlways
        add(att)
    }

    /// 01 空态：shell 侧已清种子并重启 cfprefsd。启动后截空态。
    func testEmptyState() {
        let input = app.descendants(matching: .any)["vhs.input"]
        guard input.waitForExistence(timeout: 25) else { return }
        // 一次无害交互以触发 interruption monitor。
        input.tap()
        shot("01-home-empty")
    }

    /// 02 消息态：shell 侧已写种子会话到 App 容器 plist。启动后截消息态。
    func testChatState() {
        let input = app.descendants(matching: .any)["vhs.input"]
        guard input.waitForExistence(timeout: 25) else { return }
        input.tap()
        shot("02-chat-message")
    }

    /// 03 按住态：外部 shell 并行 simctl screenshot 抓帧；本测试只长按 mic 8s。
    /// 不 assert 录音态（模拟器无音频输入时红容器可能不出现）。
    func testHoldTalking() {
        let input = app.descendants(matching: .any)["vhs.input"]
        guard input.waitForExistence(timeout: 25) else { return }
        input.tap()
        sleep(1)
        let mic = app.descendants(matching: .any)["vhs.mic"]
        guard mic.waitForExistence(timeout: 5) else { return }
        // 外部抓屏循环在此期间每 0.5s 一帧。
        mic.press(forDuration: 8)
        sleep(1)
    }
}
