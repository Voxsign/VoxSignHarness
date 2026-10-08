import XCTest

final class OrgProdDrive: XCTestCase {

    /// 显式点掉系统通知权限弹窗（最多等 8s）
    private func killNotifAlert(_ app: XCUIApplication) {
        let pred = NSPredicate(format: "label CONTAINS 'Notifications' OR label CONTAINS '通知'")
        for _ in 0..<8 {
            let alerts = app.alerts.matching(pred)
            if alerts.count > 0, alerts.firstMatch.buttons["Don't Allow"].exists {
                alerts.firstMatch.buttons["Don't Allow"].tap()
                Thread.sleep(forTimeInterval: 1.0)
                return
            }
            // 兜底：任何 alert 的 Don't Allow
            for b in app.alerts.buttons.allElementsBoundByIndex {
                if b.label == "Don't Allow" { b.tap(); Thread.sleep(forTimeInterval: 1.0); return }
            }
            Thread.sleep(forTimeInterval: 1.0)
        }
    }

    private func drive(shotName: String) throws {
        // 直接按 bundle id 启动已安装的 app（绕过 targetApplicationPath 未配置）
        let app = XCUIApplication(bundleIdentifier: "ai.voxsign.ios")
        addUIInterruptionMonitor(withDescription: "perms") { a in
            if a.buttons["Don't Allow"].exists { a.buttons["Don't Allow"].tap(); return true }
            return false
        }
        app.launch()
        Thread.sleep(forTimeInterval: 3.0)
        killNotifAlert(app)
        let machineBtn = app.buttons["vhs.machine"].firstMatch
        XCTAssertTrue(machineBtn.waitForExistence(timeout: 20), "vhs.machine 应存在")

        machineBtn.tap()
        Thread.sleep(forTimeInterval: 1.2)
        let cloudRow = app.buttons["vhs.machine.cloud"].firstMatch
        if cloudRow.waitForExistence(timeout: 5) { cloudRow.tap() }
        Thread.sleep(forTimeInterval: 7.0)

        machineBtn.tap()
        Thread.sleep(forTimeInterval: 1.5)

        let shot = XCUIScreen.main.screenshot()
        let out = "/Users/zouyongming/DoubaoWork/chats/2026-10-07/ios-accept/screenshots/\(shotName)"
        try shot.pngRepresentation.write(to: URL(fileURLWithPath: out))
        print("SHOT_WRITTEN: \(out)")
    }

    func testUserA() throws { try drive(shotName: "A-final-machine-picker.png") }
    func testUserB() throws { try drive(shotName: "B-v2-machine-picker.png") }
    func testUserC() throws { try drive(shotName: "C-v2-machine-picker.png") }
}
