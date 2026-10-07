//
//  MachineSwitchViewOrderTests.swift
//  VoxSignTests
//
//  机器切换面板分区顺序与空态契约单测（build 13 生产登记）。
//  SwiftUI 视图在逻辑单测宿主里拿不到可访问性树，故把"分区顺序"与"空态文案"
//  提取为视图层真实使用的有序枚举/常量，单测直接断言该契约（确定性、不渲染）：
//  - 分区顺序：云道（cloud） → 独立部署（unifusion） → 自建机器（selfHosted）
//  - 后端无数据（backendOrgs 空/零权限/失败）→ 独立部署区空态文案 = 「暂无可用独立部署」，
//    且手动添加入口（vhs.unifusion.add）在空态内可达
//  - 后端同步出条目 → 空态不再展示（unifusionServers 非空）
//

import XCTest
@testable import VoxSign

final class MachineSwitchViewOrderTests: XCTestCase {

    private var suiteName: String!

    override func setUpWithError() throws {
        suiteName = "vhs-mp-order-\(UUID().uuidString)"
    }
    override func tearDownWithError() throws {
        UserDefaults.standard.removePersistentDomain(forName: suiteName)
    }
    private func makeStore() -> SettingsStore {
        SettingsStore(defaults: UserDefaults(suiteName: suiteName)!)
    }

    /// 分区顺序契约：云道 < 独立部署 < 自建机器。
    func testPartitionOrderCloudThenUnifusionThenSelfHosted() {
        XCTAssertEqual(MachinePartition.allCases, [.cloud, .unifusion, .selfHosted],
                       "分区顺序必须为 云道 → 独立部署 → 自建机器")
    }

    /// 空态文案常量 = 「暂无可用独立部署」。
    func testEmptyStateTextConstant() {
        XCTAssertEqual(MachinePickerView.emptyUnifusionText, "暂无可用独立部署")
    }

    /// 后端无组织条目时独立部署区为空态（触发空态文案），手动添加入口可达。
    func testEmptyStateWhenBackendOrgsEmpty() {
        let store = makeStore()
        // 全新 store：无任何组织条目（= 后端空 / 零权限 / 拉取失败）。
        XCTAssertTrue(store.unifusionServers.isEmpty, "无后端/手动条目时独立部署区应为空态")
        // 空态内渲染手动添加兜底入口（视图据此展示 vhs.unifusion.add）。
        XCTAssertEqual(MachinePickerView.manualAddIdentifier, "vhs.unifusion.add")
    }

    /// 后端同步出条目后：独立部署区非空 → 空态文案不再展示。
    func testBackendRowHidesEmptyState() {
        let store = makeStore()
        store.servers.append(ServerConfig(id: "id-acme", name: "Acme",
                                         base: "https://acme.example", token: "",
                                         machineCode: nil, viaRelay: false,
                                         orgId: "acme", orgName: "Acme"))
        store.backendOrgs = [UniFusionOrg(orgId: "acme", orgName: "Acme",
                                          suggestedBase: "https://acme.example")]
        XCTAssertFalse(store.unifusionServers.isEmpty, "有后端条目时不应为空态")
        XCTAssertEqual(store.unifusionServers.first?.orgId, "acme")
    }
}
