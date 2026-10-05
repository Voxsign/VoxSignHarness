//
//  UIVerify2.swift — 临时验收辅助测试 第 2 轮（非产品代码）
//  目标补齐：AI 气泡信息行（会话切回后回执还原为 harness 气泡）/ 弹出 …菜单 /
//           用户气泡附件 chip（直接在 TextEditor 输入，不依赖剪贴板）/ 会话列表 / 滑动删除。
//  前置：上一轮已把自建服务器(127.0.0.1:8897/m7-token)写入 UserDefaults 并留有历史会话。
//

import XCTest

final class UIVerify2: XCTestCase {

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

    func testSecondPass() {
        let input = app.textFields["vhs.input"]
        guard input.waitForExistence(timeout: 25) else { return }
        sleep(1)

        // 1) 再发一条，拿新回执
        input.tap()
        input.typeText("verify2 second run")
        if app.buttons["vhs.send"].waitForExistence(timeout: 3) {
            app.buttons["vhs.send"].tap()
        }
        let done = app.staticTexts.containing(
            NSPredicate(format: "label CONTAINS '已完成' OR label CONTAINS '撤销'")
        ).firstMatch
        _ = done.waitForExistence(timeout: 40)
        sleep(1)

        // 2) 会话列表 → 新建会话 → 回列表 → 切回旧会话（回执还原为 harness 气泡，带信息行）
        app.buttons["vhs.sessions"].tap()
        sleep(1)
        if app.buttons["vhs.session.new"].waitForExistence(timeout: 5) {
            app.buttons["vhs.session.new"].tap()
        }
        sleep(1)
        // 回到旧会话：打开列表 → 点第二条（旧会话）
        app.buttons["vhs.sessions"].tap()
        sleep(1)
        // 区12：会话列表（多会话）
        shot("12-session-list")
        let cells = app.cells
        _ = cells.firstMatch.waitForExistence(timeout: 5)
        if cells.count >= 2 {
            cells.element(boundBy: 1).tap()
        } else {
            cells.firstMatch.tap()
        }
        sleep(2)
        // 区13：切回旧会话 → AI 气泡轻量信息行（时间 + …菜单，无🔔🔊）
        shot("13-harness-bubble-info-row")

        // 3) 点 AI 气泡下方的小 ellipsis（信息行菜单）
        let win = app.windows.firstMatch.frame
        var ell: XCUIElement?
        for b in app.buttons.allElementsBoundByIndex {
            let f = b.frame
            guard f.width <= 30, f.height <= 30,
                  f.midY > 120, f.midY < win.midY,
                  b.identifier != "vhs.send", b.identifier != "vhs.mic", b.identifier != "vhs.attach"
            else { continue }
            ell = b
        }
        ell?.tap()
        sleep(1)
        // 区14：AI 气泡 …菜单（复制/朗读/分享）
        shot("14-bubble-menu")
        if app.buttons["复制"].waitForExistence(timeout: 3) {
            app.buttons["复制"].tap()
            sleep(1)
        }

        // 4) 添加资料：直接在 TextEditor 输入文本 → 添加 → 用户气泡 chip
        app.buttons["vhs.attach"].tap()
        sleep(1)
        let editor = app.textViews.firstMatch
        if editor.waitForExistence(timeout: 5) {
            editor.tap()
            editor.typeText("attached note from verify")
        }
        let addBtns = app.buttons.matching(NSPredicate(format: "label == '添加'"))
        if addBtns.firstMatch.waitForExistence(timeout: 3) {
            addBtns.firstMatch.tap()
        }
        sleep(1)
        // 5) 发带附件的消息
        input.tap()
        input.typeText("send with attachment")
        if app.buttons["vhs.send"].waitForExistence(timeout: 3) {
            app.buttons["vhs.send"].tap()
        }
        _ = app.staticTexts.containing(
            NSPredicate(format: "label CONTAINS '已完成' OR label CONTAINS '撤销'")
        ).firstMatch.waitForExistence(timeout: 30)
        sleep(2)
        // 区15：用户气泡附件 chip（灰底小标签）
        shot("15-user-bubble-chip")

        // 6) 会话列表：滑动删除一个会话
        app.buttons["vhs.sessions"].tap()
        sleep(1)
        _ = cells.firstMatch.waitForExistence(timeout: 5)
        if cells.count >= 1 {
            cells.firstMatch.swipeLeft()
            sleep(1)
            let del = app.buttons["删除"]
            if del.waitForExistence(timeout: 3) { del.tap() }
            sleep(1)
        }
        // 区16：会话列表（删除后）
        shot("16-session-list-after-delete")
    }
}
