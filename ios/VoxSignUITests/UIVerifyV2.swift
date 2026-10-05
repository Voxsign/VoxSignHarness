//
//  UIVerifyV2.swift — 临时验收辅助测试（验收方新增，非产品代码，untracked）
//  目的：在模拟器上驱动一次真实对话，验证 AI 气泡轻量信息行
//        「只有时间 + …菜单、无"消耗"」（服务端不回传 costTokens 场景），
//        以及回执卡正常渲染。逐区 XCTAttachment(keepAlways) 截图。
//  跑法：
//   TEST_TARGET_NAME=VoxSign xcodebuild test -project ios/VoxSign.xcodeproj \
//     -scheme VoxSign -destination 'id=EC6F72A0-C06B-45A8-89AC-5F9A8D3A9F90' \
//     -derivedDataPath /tmp/vhs-sim-dd -resultBundlePath /tmp/vhs-v2-ui.xcresult \
//     -only-testing:VoxSignUITests/UIVerifyV2/testChatInfoRowNoCost
//

import XCTest

final class UIVerifyV2: XCTestCase {

    let app = XCUIApplication()

    override func setUp() {
        continueAfterFailure = true
        app.launch()
    }

    func shot(_ name: String) {
        let att = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        att.name = name
        att.lifetime = .keepAlways
        add(att)
    }

    /// 确保自建服务器 127.0.0.1:8897 / m7-token 已配置（幂等：已存在则跳过添加）。
    private func ensureSelfHosted() {
        let input = app.textFields["vhs.input"]
        guard input.waitForExistence(timeout: 25) else {
            XCTFail("vhs.input 未出现")
            return
        }
        app.buttons["vhs.more"].tap()
        _ = app.buttons["设置"].waitForExistence(timeout: 5)
        app.buttons["设置"].tap()
        sleep(1)

        _ = app.buttons["自建"].waitForExistence(timeout: 5)
        app.buttons["自建"].tap()
        sleep(1)

        // 已配置过"本机Mac"则跳过添加。
        let existing = app.staticTexts["本机Mac"]
        if !existing.waitForExistence(timeout: 3) {
            _ = app.buttons["添加服务器"].waitForExistence(timeout: 5)
            app.buttons["添加服务器"].tap()
            sleep(1)
            let ipBtn = app.buttons.containing(
                NSPredicate(format: "label CONTAINS 'IP 地址'")
            ).firstMatch
            if ipBtn.waitForExistence(timeout: 5) { ipBtn.tap() }
            sleep(1)

            let nameF = app.textFields["名称（如：办公室 Mac）"]
            let baseF = app.textFields["http://192.168.x.x:8897"]
            let tokF = app.secureTextFields["Bearer Token（可选）"]
            if nameF.waitForExistence(timeout: 5) {
                nameF.tap(); nameF.typeText("本机Mac")
                baseF.tap(); baseF.typeText("http://127.0.0.1:8897")
                tokF.tap(); tokF.typeText("m7-token")
            }
            let save = app.buttons["保存并检测连接"]
            if save.waitForExistence(timeout: 5) { save.tap() }
            _ = app.staticTexts["本机Mac"].waitForExistence(timeout: 15)
            sleep(1)
        }
        // 完成 → 回聊天
        app.buttons["完成"].firstMatch.tap()
        sleep(1)
    }

    func testChatInfoRowNoCost() {
        let input = app.textFields["vhs.input"]
        guard input.waitForExistence(timeout: 25) else {
            XCTFail("vhs.input 未出现")
            return
        }
        // 区1：空态/顶栏（显示名 VoxSign）+ 输入条
        sleep(1)
        shot("01-home-topbar")

        ensureSelfHosted()

        // —— 发一条消息，等 AI 回执 ——
        input.tap()
        input.typeText("记一下 验收v2 消耗解耦")
        if app.buttons["vhs.send"].waitForExistence(timeout: 3) {
            app.buttons["vhs.send"].tap()
        }
        // 回执卡：等"已完成"/"待澄清"/"撤销"任一出现
        let done = app.staticTexts.containing(
            NSPredicate(format: "label CONTAINS '已完成' OR label CONTAINS '撤销' OR label CONTAINS '待澄清'")
        ).firstMatch
        _ = done.waitForExistence(timeout: 45)
        sleep(1)
        // 区2：AI 气泡信息行（时间 + …，无"消耗"）+ 回执卡
        shot("02-ai-bubble-inforow")

        // —— AI 气泡「…」菜单：下半区小尺寸 ellipsis 按钮 ——
        let win = app.windows.firstMatch.frame
        var bubbleEllipsis: XCUIElement?
        for b in app.buttons.allElementsBoundByIndex {
            let f = b.frame
            guard f.width <= 30, f.height <= 30,
                  f.midY > win.midY, f.midY < win.maxY - 70,
                  b.identifier != "vhs.send", b.identifier != "vhs.mic", b.identifier != "vhs.attach"
            else { continue }
            bubbleEllipsis = b
        }
        bubbleEllipsis?.tap()
        sleep(1)
        // 区3：AI 消息操作菜单（复制/朗读/分享）
        shot("03-ai-bubble-menu")
        if app.buttons["复制"].waitForExistence(timeout: 3) {
            app.buttons["复制"].tap()
        }
        sleep(1)
    }
}
