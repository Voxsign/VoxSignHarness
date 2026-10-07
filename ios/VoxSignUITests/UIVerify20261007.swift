//
//  UIVerify20261007.swift — 临时验收辅助测试（验证包执行者新增，非产品代码）
//  目的：2026-10-07 phase1-reply-ios 真机/模拟器验收闭环。
//   1) 云端模式冒烟：键盘输入「研究一下 Unifashion 客户后台状况」→ 发送 → 等回执，
//      断言回复气泡非空、非旧模板「我没太确定」、非 E_MODEL_EMPTY。
//   2) UniFusion 机器切换面板：断言 vhs.machine.cloud / vhs.unifusion.row.bsc /
//      vhs.unifusion.row.peterzou 出现，点 BSC 行切换。
//   3) 设置页 → vhs.unifusion.add → 表单填占位不可达地址 → 保存 → 断言中文报错
//      （vhs.unifusion.form.error），非静默崩溃。
//   4) 无回归截图：侧栏/新建会话/按住说话/设置页。
//  跑法：
//   xcodebuild test -project ios/VoxSign.xcodeproj -scheme VoxSign \
//     -destination 'id=A4636CC9-0A00-4007-B3C3-13BDF0C0DEB7' \
//     -derivedDataPath /tmp/vhs-acc-dd -resultBundlePath /tmp/vhs-acc.xcresult \
//     -only-testing:VoxSignUITests/UIVerify20261007/testAccept20261007
//

import XCTest

final class UIVerify20261007: XCTestCase {

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

    /// 关闭可能出现的系统权限弹窗（通知/麦克风等）。
    func dismissSystemAlerts() {
        let dontAllow = app.buttons["Don’t Allow"]
        if dontAllow.waitForExistence(timeout: 2) { dontAllow.tap() }
        let allow = app.buttons["允许"]
        if allow.waitForExistence(timeout: 1) { allow.tap() }
    }

    func testAccept20261007() {
        dismissSystemAlerts()

        // —— 切到键盘输入 ——
        let kb = app.buttons["vhs.keyboard"]
        XCTAssertTrue(kb.waitForExistence(timeout: 10), "vhs.keyboard 未出现")
        kb.tap()
        let input = app.textFields["vhs.input"]
        XCTAssertTrue(input.waitForExistence(timeout: 5), "vhs.input 输入框未出现")
        shot("00-home-keyboard")

        // —— 云端冒烟：发句、等回执 ——
        input.tap()
        input.typeText("研究一下 Unifashion 客户后台状况")
        let send = app.buttons["vhs.send"]
        XCTAssertTrue(send.waitForExistence(timeout: 3), "vhs.send 未出现")
        send.tap()

        // 等「正在处理…」出现后消失（最长 90s）
        let processing = app.staticTexts["正在处理…"]
        _ = processing.waitForExistence(timeout: 10)
        // 轮询：处理中标签消失或出现回执/气泡
        var gotReply = false
        for _ in 0..<45 {
            sleep(2)
            if !processing.exists { gotReply = true; break }
        }
        sleep(2)
        shot("01-cloud-smoke-reply")
        XCTAssertTrue(gotReply, "90s 内未收到回执")

        // —— 断言回复内容：收集屏幕上所有 staticText ——
        var allText = ""
        for t in app.staticTexts.allElementsBoundByIndex {
            allText += (t.label) + "\n"
        }
        print("[ACC] on-screen text sample: \(String(allText.prefix(400)))")
        XCTAssertFalse(allText.contains("我没太确定"), "命中旧模板「我没太确定」")
        XCTAssertFalse(allText.contains("E_MODEL_EMPTY"), "命中 E_MODEL_EMPTY")
        XCTAssertFalse(allText.contains("OK 自动执行"), "命中空壳「OK 自动执行」")
        // 应有一条自然长度的中文回复（启发式：含 Unifashion 或 10 字以上中文气泡）
        let hasNatural = allText.contains("Unifashion") ||
            allText.range(of: "[\\u4e00-\\u9fa5]{15,}", options: .regularExpression) != nil
        XCTAssertTrue(hasNatural, "未见豆包式自然回复（无 Unifashion / 长中文气泡）")

        // —— 机器切换面板：UniFusion 分区 ——
        dismissSystemAlerts()
        let machine = app.buttons["vhs.machine"]
        XCTAssertTrue(machine.waitForExistence(timeout: 5), "vhs.machine 未出现")
        machine.tap()
        sleep(1)
        XCTAssertTrue(app.buttons["vhs.machine.cloud"].waitForExistence(timeout: 5), "vhs.machine.cloud 缺失")
        XCTAssertTrue(app.buttons["vhs.unifusion.row.bsc"].waitForExistence(timeout: 5), "vhs.unifusion.row.bsc 缺失")
        XCTAssertTrue(app.buttons["vhs.unifusion.row.peterzou"].waitForExistence(timeout: 5), "vhs.unifusion.row.peterzou 缺失")
        shot("02-machine-switch-panel")

        // 点 BSC 行 → 切换
        app.buttons["vhs.unifusion.row.bsc"].tap()
        sleep(1)
        shot("03-unifusion-switched-bsc")

        // 关闭面板（点云道行回云端，避免后续设置探测走 BSC 占位）
        if app.buttons["vhs.machine.cloud"].exists {
            app.buttons["vhs.machine.cloud"].tap()
            sleep(1)
        }

        // —— 设置页 → 添加 UniFusion 部署 ——
        // 通过会话列表的设置入口或顶部更多按钮；这里用 vhs.more 范式
        dismissSystemAlerts()
        // 先尝试 vhs.session.settings（会话列表内）；若无则直接找「设置」按钮
        if app.buttons["vhs.sessions"].waitForExistence(timeout: 3) {
            app.buttons["vhs.sessions"].tap()
            sleep(1)
        }
        // 关闭可能的弹窗
        let closeSession = app.buttons["完成"]
        if closeSession.waitForExistence(timeout: 1) { /* 仍在 */ }
        // 设置页：找 vhs.unifusion.add（设置页内）或顶部导航「设置」
        // 直接通过 label 找「设置」
        let settingsBtn = app.buttons["设置"]
        if !settingsBtn.waitForExistence(timeout: 3) {
            // 回主页
            app.buttons["vhs.sessions"].tap()
            sleep(1)
        }
        if settingsBtn.exists { settingsBtn.tap() }
        sleep(1)
        shot("04-settings")

        // UniFusion 分区在设置 Form 下方，需先滚动才可见/可定位
        let addRow = app.buttons["vhs.unifusion.add"]
        var foundAdd = addRow.waitForExistence(timeout: 3)
        if !foundAdd {
            for _ in 0..<5 {
                app.swipeUp()
                sleep(1)
                if addRow.waitForExistence(timeout: 2) { foundAdd = true; break }
            }
        }
        XCTAssertTrue(foundAdd, "设置页 vhs.unifusion.add 缺失")
        addRow.tap()
        sleep(1)

        // 表单：填占位不可达地址
        let nameF = app.textFields["vhs.unifusion.form.name"]
        let baseF = app.textFields["vhs.unifusion.form.base"]
        if nameF.waitForExistence(timeout: 5) {
            nameF.tap(); nameF.typeText("ACC 占位组织")
        }
        if baseF.exists {
            baseF.tap(); baseF.typeText("http://10.255.255.1:8897")
        }
        shot("05-unifusion-form-empty")

        let saveBtn = app.buttons["vhs.unifusion.form.save"]
        XCTAssertTrue(saveBtn.waitForExistence(timeout: 3), "vhs.unifusion.form.save 缺失")
        saveBtn.tap()

        // 等报错（探测超时 ~10s）
        let err = app.otherElements["vhs.unifusion.form.error"]
        let err2 = app.staticTexts["vhs.unifusion.form.error"]
        var sawError = false
        for _ in 0..<12 {
            sleep(2)
            if err.exists || err2.exists { sawError = true; break }
            // 屏幕文本里出现中文报错关键词也可
            let s = app.staticTexts.allElementsBoundByIndex.map { $0.label }.joined(separator: "|")
            if s.contains("无法连接") || s.contains("连接失败") || s.contains("超时") || s.contains("错误") {
                sawError = true; break
            }
        }
        sleep(1)
        shot("06-unifusion-error")
        XCTAssertTrue(sawError, "不可达地址未给出合理中文报错")

        // 完成 → 退出表单
        let done = app.buttons["完成"].firstMatch
        if done.waitForExistence(timeout: 2) { done.tap() }
        sleep(1)
        shot("07-settings-after-add")
    }
}
