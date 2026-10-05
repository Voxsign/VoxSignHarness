//
//  UIVerify20261005.swift — 临时验收辅助测试（验证包执行者新增，非产品代码）
//  目的：在模拟器上驱动导航到各验收区，逐区截图（XCTAttachment keepAlways），
//        并实测 添加资料 / 多会话 / …菜单复制 / 会话切换删除。
//  跑法：
//   TEST_TARGET_NAME=VoxSign xcodebuild test -project ios/VoxSign.xcodeproj \
//     -scheme VoxSign -destination 'id=EC6F72A0-C06B-45A8-89AC-5F9A8D3A9F90' \
//     -derivedDataPath /tmp/vhs24-test-dd -resultBundlePath /tmp/vhs24-ui.xcresult \
//     -only-testing:VoxSignUITests/UIVerify20261005/testVerifyAllRegions
//

import XCTest

final class UIVerify20261005: XCTestCase {

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

    func testVerifyAllRegions() {
        let input = app.textFields["vhs.input"]
        guard input.waitForExistence(timeout: 25) else {
            XCTFail("vhs.input 未出现")
            return
        }
        sleep(1)
        // 区1：顶栏（状态点+标题+左右入口）+ 极简输入条三件套 + 空态欢迎语
        shot("01-home-empty")

        // —— 配置自建服务器（设置 → 自建 → 添加服务器 → IP）——
        app.buttons["vhs.more"].tap()
        _ = app.buttons["设置"].waitForExistence(timeout: 5)
        app.buttons["设置"].tap()
        sleep(1)
        // 区2：设置页（云道默认态）
        shot("02-settings-cloud")

        _ = app.buttons["自建"].waitForExistence(timeout: 5)
        app.buttons["自建"].tap()
        sleep(1)
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
        // 等服务器行出现（连通性检测通过才保存）
        let row = app.staticTexts["本机Mac"]
        _ = row.waitForExistence(timeout: 15)
        sleep(1)
        // 区3：设置页（自建已连接态）
        shot("03-settings-selfhosted")
        // 完成 → 回聊天
        app.buttons["完成"].firstMatch.tap()
        sleep(1)

        // —— 发一条消息，等 AI 回执 ——
        input.tap()
        input.typeText("verify note 20261005")
        if app.buttons["vhs.send"].waitForExistence(timeout: 3) {
            app.buttons["vhs.send"].tap()
        }
        let done = app.staticTexts.containing(
            NSPredicate(format: "label CONTAINS '已完成' OR label CONTAINS '撤销' OR label CONTAINS '没'")
        ).firstMatch
        _ = done.waitForExistence(timeout: 45)
        sleep(1)
        // 区4：AI 消息信息行（消耗·时间·…，无🔔🔊）+ 回执卡片 + 用户气泡
        shot("04-chat-receipt")

        // —— AI 气泡「…」菜单：定位窗口下半区的小尺寸 ellipsis 按钮 ——
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
        // 区5：AI 消息操作菜单（复制/朗读/分享）
        shot("05-ai-bubble-menu")
        if app.buttons["复制"].waitForExistence(timeout: 3) {
            app.buttons["复制"].tap()
        }
        sleep(1)

        // —— 添加资料面板：＋ → 从剪贴板读取（即刚复制的 AI 文本）→ 添加 ——
        app.buttons["vhs.attach"].tap()
        sleep(1)
        if app.buttons["从剪贴板读取"].waitForExistence(timeout: 5) {
            app.buttons["从剪贴板读取"].tap()
            sleep(1)
        }
        // 区6：添加资料面板（四入口 + 剪贴板已读入文本）
        shot("06-attach-panel")
        let addBtns = app.buttons.matching(NSPredicate(format: "label == '添加'"))
        if addBtns.firstMatch.waitForExistence(timeout: 3) {
            addBtns.firstMatch.tap()
        }
        sleep(1)
        // 回聊天：发带附件的消息 → 用户气泡出现附件 chip
        input.tap()
        input.typeText("with attachment")
        if app.buttons["vhs.send"].waitForExistence(timeout: 3) {
            app.buttons["vhs.send"].tap()
        }
        sleep(3)
        // 区7：用户气泡附件 chip
        shot("07-user-bubble-chip")

        // —— 会话列表 → 新建会话 ——
        app.buttons["vhs.sessions"].tap()
        sleep(1)
        // 区8：会话列表（≥1 条历史）
        shot("08-session-list")
        if app.buttons["vhs.session.new"].waitForExistence(timeout: 5) {
            app.buttons["vhs.session.new"].tap()
        }
        sleep(1)
        // 区9：新会话空态（一行欢迎语）
        shot("09-new-session-empty")

        // —— 再进列表：删掉新建的空会话（滑动删除），再切回旧会话 ——
        app.buttons["vhs.sessions"].tap()
        sleep(1)
        let cells = app.cells
        _ = cells.firstMatch.waitForExistence(timeout: 5)
        if cells.count >= 2 {
            cells.firstMatch.swipeLeft()
            sleep(1)
            let del = app.buttons["删除"]
            if del.waitForExistence(timeout: 3) { del.tap() }
            sleep(1)
        }
        // 点剩下的旧会话行 → 切回
        if cells.firstMatch.waitForExistence(timeout: 3) {
            cells.firstMatch.tap()
        }
        sleep(2)
        // 区10：切回旧会话（历史气泡恢复显示）
        shot("10-switch-back")
    }
}
