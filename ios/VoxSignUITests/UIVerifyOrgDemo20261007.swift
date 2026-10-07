import XCTest

/// UniFusion 组织目录端到端演示（用户 A：demo-org-a → bsc + peterzou）。
/// 跑法：先装新 Debug 包并注入 demo-org-a 的 auth + 本地演示机器，再
/// TEST_TARGET_NAME=VoxSign xcodebuild test -only-testing:VoxSignUITests/OrgDemoUserA
final class OrgDemoUserA: XCTestCase {
    func testOrgCatalogFromBackend() throws {
        let app = XCUIApplication()
        app.launch()

        // 云端初始态：机器名按钮存在
        let machineBtn = app.buttons["vhs.machine"].firstMatch
        XCTAssertTrue(machineBtn.waitForExistence(timeout: 20), "顶栏机器名（vhs.machine）应存在")
        machineBtn.tap() // 打开机器切换面板

        // 切到本地演示自建机（127.0.0.1:8898）
        let localRow = app.descendants(matching: .any)["vhs.selfhost.row.local-8898"].firstMatch
        if !localRow.waitForExistence(timeout: 10) {
            let byText = app.staticTexts["本地演示"].firstMatch
            XCTAssertTrue(byText.waitForExistence(timeout: 10), "本地演示自建机行应存在")
            byText.tap()
        } else {
            localRow.tap() // 切换即关闭面板并触发组织拉取
        }
        sleep(6)       // 等 refreshUniFusionOrgs 完成（本地 harness 快）

        // 重新打开面板：UniFusion 区应出现后端返回的组织条目
        machineBtn.tap()
        let rowBSC = app.descendants(matching: .any)["vhs.unifusion.row.bsc"].firstMatch
        let rowPZ  = app.descendants(matching: .any)["vhs.unifusion.row.peterzou"].firstMatch
        XCTAssertTrue(rowBSC.waitForExistence(timeout: 12), "UniFusion·BSC 应来自后端 /v1/orgs")
        XCTAssertTrue(rowPZ.exists, "UniFusion·PeterZou 应来自后端 /v1/orgs")

        let listShot = XCTAttachment(screenshot: app.screenshot())
        listShot.name = "unifusion-org-list"
        listShot.lifetime = .keepAlways
        add(listShot)

        // 添加企业部署地址表单（手动兜底入口仍可用）
        let addBtn = app.descendants(matching: .any)["vhs.unifusion.add"].firstMatch
        if addBtn.exists { addBtn.tap(); sleep(3) }
        let addShot = XCTAttachment(screenshot: app.screenshot())
        addShot.name = "unifusion-add"
        addShot.lifetime = .keepAlways
        add(addShot)
    }
}

/// 添加企业部署地址表单截图（设置页入口）。
final class AddFormDemo: XCTestCase {
    func testAddFormScreenshot() throws {
        let app = XCUIApplication()
        app.launch()
        let machineBtn = app.buttons["vhs.machine"].firstMatch
        XCTAssertTrue(machineBtn.waitForExistence(timeout: 20), "顶栏机器名应存在")
        machineBtn.tap()
        // 面板右上「管理」→ 设置页
        let gear = app.buttons["管理"].firstMatch
        XCTAssertTrue(gear.waitForExistence(timeout: 6), "面板「管理」按钮应存在")
        gear.tap()
        sleep(2)
        // 设置页「添加企业部署地址」（页面下方，需滚动）
        let addBtn = app.descendants(matching: .any)["vhs.unifusion.add"].firstMatch
        var found = addBtn.waitForExistence(timeout: 5)
        for _ in 0..<6 where !found {
            app.swipeUp()
            sleep(1)
            found = addBtn.waitForExistence(timeout: 2)
        }
        XCTAssertTrue(found, "设置页添加企业部署地址入口应存在（滚动后）")
        addBtn.tap()
        sleep(3)
        let formShot = XCTAttachment(screenshot: app.screenshot())
        formShot.name = "unifusion-add-form"
        formShot.lifetime = .keepAlways
        add(formShot)
    }
}

extension XCUIElement {
    func tapIfExists() {
        if exists { tap() }
    }
}
final class OrgDemoUserB: XCTestCase {
    func testOrgIsolationUserB() throws {
        let app = XCUIApplication()
        app.launch()

        let machineBtn = app.buttons["vhs.machine"].firstMatch
        XCTAssertTrue(machineBtn.waitForExistence(timeout: 20), "顶栏机器名应存在")
        machineBtn.tap()
        let localRow = app.descendants(matching: .any)["vhs.selfhost.row.local-8898"].firstMatch
        if !localRow.waitForExistence(timeout: 10) {
            let byText = app.staticTexts["本地演示"].firstMatch
            XCTAssertTrue(byText.waitForExistence(timeout: 10), "本地演示自建机行应存在")
            byText.tap()
        } else {
            localRow.tap()
        }
        sleep(6)

        machineBtn.tap()
        // 只应有 healthex，绝无 bsc/peterzou（按组织隔离）
        let rowHE  = app.descendants(matching: .any)["vhs.unifusion.row.healthex"].firstMatch
        XCTAssertTrue(rowHE.waitForExistence(timeout: 12), "UniFusion·HealthEx 应来自后端")
        XCTAssertFalse(app.descendants(matching: .any)["vhs.unifusion.row.bsc"].exists, "B 用户不应看到 bsc 组织")
        XCTAssertFalse(app.descendants(matching: .any)["vhs.unifusion.row.peterzou"].exists, "B 用户不应看到 peterzou 组织")

        let isoShot = XCTAttachment(screenshot: app.screenshot())
        isoShot.name = "unifusion-isolation"
        isoShot.lifetime = .keepAlways
        add(isoShot)
    }
}
