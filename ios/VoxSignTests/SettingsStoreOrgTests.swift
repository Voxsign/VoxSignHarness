//
//  SettingsStoreOrgTests.swift
//  VoxSignTests
//
//  独立部署（组织目录）后端联动单测：
//  - mock /v1/orgs 返回 → store 自动出现对应条目（按 orgId upsert，base/viaRelay 以后端为准）
//  - 条目名 = 客户名（orgName），不加品牌前缀
//  - 空组织 / 拉取失败 → 不崩、不清空已有条目，手动添加兜底仍可用
//  用独立 UserDefaults suite 隔离；APIClient 注入测试 store + orgsMock，不走真实网络。
//

import XCTest
@testable import VoxSign

final class SettingsStoreOrgTests: XCTestCase {
    private var suiteName: String!

    override func setUpWithError() throws {
        suiteName = "vhs-org-test-\(UUID().uuidString)"
    }

    override func tearDownWithError() throws {
        UserDefaults.standard.removePersistentDomain(forName: suiteName)
        // 还原 APIClient 单例，避免污染其他用例。
        APIClient.shared.settings = .shared
        APIClient.shared.orgsMock = nil
        suiteName = nil
    }

    private func makeStore() -> SettingsStore {
        SettingsStore(defaults: UserDefaults(suiteName: suiteName)!)
    }

    /// 直接构造一个已存在的独立部署条目（绕开 addServer 的 ConnectivityService/网络副作用，
    /// 保持单测隔离、不触发真机网络与 RunLoop 定时器）。
    private func seedEntry(_ store: SettingsStore, orgId: String, base: String, token: String,
                           viaRelay: Bool = false) {
        store.servers.append(ServerConfig(id: "id-\(orgId)", name: orgId,
                                          base: base, token: token,
                                          machineCode: nil, viaRelay: viaRelay,
                                          orgId: orgId, orgName: orgId))
    }

    // MARK: - 成功路径

    func testSyncOrgsUpsertsEntries() async {
        let store = makeStore()
        let api = APIClient.shared
        api.settings = store
        api.orgsMock = [
            OrgEntry(orgId: "unifusion", orgName: "UniFusion", base: "https://unifusion.example", viaRelay: false),
            OrgEntry(orgId: "healthex", orgName: "HealthEx", base: "https://hx.example", viaRelay: true),
        ]

        await store.refreshUniFusionOrgs()

        let ids = Set(store.unifusionServers.compactMap { $0.orgId })
        XCTAssertTrue(ids.contains("unifusion"), "应同步出 unifusion 条目")
        XCTAssertTrue(ids.contains("healthex"), "应同步出后端新条目 healthex")
        XCTAssertEqual(store.unifusionServers.first(where: { $0.orgId == "healthex" })?.base,
                       "https://hx.example")
        XCTAssertEqual(store.unifusionServers.first(where: { $0.orgId == "healthex" })?.usesRelay, true)
        // 条目名 = 客户名，不加品牌前缀
        XCTAssertEqual(store.unifusionServers.first(where: { $0.orgId == "healthex" })?.name,
                       "HealthEx")
        XCTAssertEqual(store.backendOrgs.count, 2)
    }

    /// 已有条目被后端更新（base/viaRelay），但用户已填 token 不被覆盖。
    /// 用 orgId=acme 避免与 init 兜底播种的 unifusion/peterzou 条目撞 orgId。
    func testSyncUpdatesExistingKeepsToken() async {
        let store = makeStore()
        let api = APIClient.shared
        api.settings = store
        seedEntry(store, orgId: "acme", base: "https://old.example", token: "my-secret")

        api.orgsMock = [
            OrgEntry(orgId: "acme", orgName: "Acme", base: "https://new-acme.example", viaRelay: true),
        ]
        await store.refreshUniFusionOrgs()

        let acme = store.unifusionServers.first(where: { $0.orgId == "acme" })
        XCTAssertEqual(acme?.base, "https://new-acme.example", "base 以后端为准")
        XCTAssertEqual(acme?.usesRelay, true, "viaRelay 以后端为准")
        XCTAssertEqual(acme?.token, "my-secret", "用户已填 token 不应被后端同步覆盖")
        XCTAssertEqual(acme?.name, "Acme", "条目名=客户名，无前缀")
    }

    // MARK: - 失败 / 空路径

    func testEmptyOrgsKeepsManualAdd() async {
        let store = makeStore()
        let api = APIClient.shared
        api.settings = store
        api.orgsMock = []
        await store.refreshUniFusionOrgs()

        XCTAssertEqual(store.backendOrgs, [])
        // 手动添加兜底仍可用（直接构造一条手动条目，模拟用户手动添加）
        seedEntry(store, orgId: "manual-1", base: "http://10.0.0.9:8897", token: "t")
        XCTAssertTrue(store.unifusionServers.contains(where: { $0.orgId == "manual-1" }))
    }

    func testFailureDoesNotWipeExistingEntries() async {
        let store = makeStore()
        let api = APIClient.shared
        api.settings = store
        // 先成功同步一组
        api.orgsMock = [OrgEntry(orgId: "unifusion", orgName: "UniFusion", base: "https://unifusion.example", viaRelay: false)]
        await store.refreshUniFusionOrgs()
        XCTAssertTrue(store.unifusionServers.contains(where: { $0.orgId == "unifusion" }))

        // 再失败/空：既有条目不被清空
        api.orgsMock = []
        await store.refreshUniFusionOrgs()
        XCTAssertTrue(store.unifusionServers.contains(where: { $0.orgId == "unifusion" }),
                      "拉取空/失败不应删除已有条目")
    }
}
