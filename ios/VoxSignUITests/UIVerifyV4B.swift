//
//  UIVerifyV4B.swift — V4 方案B 临时验收辅助测试（非产品代码）
//  目的：模拟器上验证并截图 V4 方案B 三个核心态：
//        ① 默认态（空态 + ＋/🎤大按钮/⌨ 输入条）
//        ② 按住态（满底波形界面：白底红波形 + “正在听…” + “松手发送 · 上移取消”）
//        ③ 文字模式（点 ⌨ → 输入框 + 发送箭头）
//  跑法：
//   TEST_TARGET_NAME=VoxSign xcodebuild test -project ios/VoxSign.xcodeproj \
//     -scheme VoxSign -destination 'id=61BF6DC5-ABAA-4ED9-BEA9-0656F6A4590E' \
//     -derivedDataPath /tmp/vhs-v4-dd-test -resultBundlePath /tmp/vhs-v4-ui.xcresult \
//     -only-testing:VoxSignUITests/UIVerifyV4B/testV4BVerify
//

import XCTest

final class UIVerifyV4B: XCTestCase {

    let app = XCUIApplication()

    override func setUp() {
        continueAfterFailure = true
        app.launch()
    }

    /// 截图并 keepAlways，后续用 xcresulttool export attachments 导出。
    func shot(_ name: String) {
        let att = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        att.name = name
        att.lifetime = .keepAlways
        add(att)
    }

    func testV4BVerify() {
        // 点掉首次安装的系统通知授权弹窗（springboard 级）
        let sb = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        let alert = sb.alerts.firstMatch
        if alert.waitForExistence(timeout: 5) {
            let allow = alert.buttons["Allow"]
            if allow.exists {
                allow.tap()
            } else if alert.buttons["允许"].exists {
                alert.buttons["允许"].tap()
            }
            sleep(1)
        }

        // ① 默认态：等空态 + 语音大按钮出现
        let mic = app.descendants(matching: .any)["vhs.mic"]
        _ = mic.waitForExistence(timeout: 25)
        sleep(2)
        shot("01-v4-default")

        // ② 按住态：按住语音大按钮，按住中途在辅助线程截图（波形界面）
        let holdShot = expectation(description: "hold-shot")
        Thread.detachNewThread { [weak self] in
            Thread.sleep(forTimeInterval: 2.0)
            guard let self = self else { return }
            let att = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
            att.name = "02-v4-hold-wave"
            att.lifetime = .keepAlways
            DispatchQueue.main.async {
                self.add(att)
                holdShot.fulfill()
            }
        }
        mic.press(forDuration: 4.0)
        wait(for: [holdShot], timeout: 12)

        // ③ 文字模式：点 ⌨ → 输入框出现
        let kb = app.descendants(matching: .any)["vhs.keyboard"]
        if kb.waitForExistence(timeout: 5) { kb.tap() }
        let input = app.textFields["vhs.input"]
        _ = input.waitForExistence(timeout: 8)
        sleep(1)
        shot("03-v4-textmode")

        // 输入文本 → 发送箭头出现
        input.tap()
        input.typeText("hello v4")
        sleep(1)
        shot("04-v4-text-typed")
    }
}
