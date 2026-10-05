//
//  VSLogicTests.swift
//  VoxSignTests
//
//  纯逻辑层 XCTest（1:1 覆盖 web/test.js 的 44 用例 + SSE/重连新增）。
//

import XCTest
@testable import VoxSign

final class VSLogicTests: XCTestCase {

    // MARK: - 1. 终态/决策点状态机

    func testTerminal() {
        XCTAssertTrue(VSLogic.isTerminal("done"))
        XCTAssertTrue(VSLogic.isTerminal("canceled"))
        XCTAssertTrue(VSLogic.isTerminal("interrupted"))
        XCTAssertFalse(VSLogic.isTerminal("running"))
        XCTAssertFalse(VSLogic.isTerminal("need_ask"))
        XCTAssertTrue(VSLogic.isDecision("need_confirm"))
        XCTAssertFalse(VSLogic.isDecision("done"))
    }

    // MARK: - 2. 回执四行解析

    func testParseReceipt() {
        let text = "动作：NOTE 追加一行\n文件：notes.md\n结果：OK 已追加\n撤销：从备份 notes.md.20261002T1530.bak 恢复"
        let r = VSLogic.parseReceipt(text)
        XCTAssertEqual(r.action, "NOTE 追加一行")
        XCTAssertEqual(r.files, "notes.md")
        XCTAssertEqual(r.result, "OK 已追加")
        XCTAssertEqual(r.undo, "从备份 notes.md.20261002T1530.bak 恢复")

        let empty = VSLogic.parseReceipt("")
        XCTAssertEqual(empty, Receipt())

        // 半角冒号 + 缺行
        let r3 = VSLogic.parseReceipt("动作:COMMIT 提交\n撤销：不可撤销（不可逆，已人工确认）")
        XCTAssertEqual(r3.result, "")
        XCTAssertEqual(r3.undo, "不可撤销（不可逆，已人工确认）")
    }

    // MARK: - 3. 撤销按钮裁决

    func testExtractUndo() {
        let r = VSLogic.parseReceipt("动作：NOTE 追加一行\n文件：notes.md\n结果：OK\n撤销：从备份 notes.md.20261002T1530.bak 恢复")
        let u = VSLogic.extractUndo(r, true)
        XCTAssertTrue(u.show)
        XCTAssertEqual(u.backup, "notes.md.20261002T1530.bak")
        XCTAssertFalse(u.irreversible)

        // 撤销行声明不可撤销 → 不给按钮
        let c = VSLogic.parseReceipt("动作：COMMIT\n撤销：不可撤销（不可逆，已人工确认）")
        XCTAssertFalse(VSLogic.extractUndo(c, true).show)
        // server 未标 reversible → 不给按钮
        XCTAssertFalse(VSLogic.extractUndo(r, false).show)
        // VHS_BACKUP_PATH 前缀兼容
        let b = VSLogic.extractUndo(VSLogic.parseReceipt("撤销：VHS_BACKUP_PATH: notes.md.20261002T.bak"), true)
        XCTAssertEqual(b.backup, "notes.md.20261002T.bak")
    }

    // MARK: - 4. 轻标签压缩

    func testBadges() {
        let receipt = "动作：NOTE 追加一行\n文件：notes.md\n结果：OK\n撤销：从备份 x.bak 恢复"
        let bs = VSLogic.compressBadges(TaskView(status: "running", receipt: receipt, reversible: true))
        XCTAssertTrue(bs.contains(where: { $0.label == "执行中" && $0.tone == "blue" }))
        XCTAssertTrue(bs.contains(where: { $0.label == "笔记" && $0.kind == "intent" }))
        XCTAssertTrue(bs.contains(where: { $0.label == "笔记域" }))
        XCTAssertTrue(bs.contains(where: { $0.label == "可逆" && $0.tone == "green" }))

        let b2 = VSLogic.compressBadges(TaskView(status: "need_confirm", question: "人工放行"))
        XCTAssertTrue(b2.contains(where: { $0.label == "高风险·待放行" && $0.tone == "red" }))

        let b3 = VSLogic.compressBadges(TaskView(status: "done", receipt: "动作：COMMIT 提交", reversible: false))
        XCTAssertTrue(b3.contains(where: { $0.label == "不可逆" && $0.tone == "red" }))
    }

    // MARK: - 5. 一屏一个决策点路由

    func testDecisionRoute() {
        XCTAssertEqual(VSLogic.nextDecisionPoint(TaskView(status: "need_confirm", question: "人工放行")).kind, .confirm)
        let ask = VSLogic.nextDecisionPoint(TaskView(status: "need_ask", question: "哪个？",
            options: [TaskOption(id: "f1", label: "notes.md"), TaskOption(id: "f2", label: "main.go")]))
        XCTAssertEqual(ask.kind, .ask)
        XCTAssertEqual(ask.options.count, 2)
        XCTAssertEqual(VSLogic.nextDecisionPoint(TaskView(status: "done",
            receipt: "动作：NOTE\n文件：notes.md\n结果：OK\n撤销：x.bak", reversible: true)).kind, .receipt)
        XCTAssertEqual(VSLogic.nextDecisionPoint(TaskView(status: "running")).kind, .running)
        XCTAssertEqual(VSLogic.nextDecisionPoint(TaskView(status: "canceled", error: "取消")).kind, .error)
        XCTAssertEqual(VSLogic.nextDecisionPoint(TaskView(status: "interrupted")).kind, .error)
    }

    // MARK: - 6. 角色映射

    func testRole() {
        XCTAssertEqual(VSLogic.roleForStatus("need_ask"), "planner")
        XCTAssertEqual(VSLogic.roleForStatus("need_confirm"), "planner")
        XCTAssertEqual(VSLogic.roleForStatus("running"), "executor")
        XCTAssertEqual(VSLogic.roleForStatus("done"), "verifier")
        XCTAssertEqual(VSLogic.roleLabels["verifier"], "Verifier")
    }

    // MARK: - 7. 打断状态机

    func testInterruptBar() {
        let bar = VSLogic.interruptSystemBar(TaskView(status: "running"))
        XCTAssertTrue(bar.active[0].contains("尚未产生文件变更"))
        XCTAssertEqual(bar.actions, ["继续"])

        let receipt = "动作：NOTE 追加一行\n文件：notes.md\n结果：OK\n撤销：x.bak"
        let bar2 = VSLogic.interruptSystemBar(TaskView(status: "done", receipt: receipt, reversible: true))
        XCTAssertTrue(bar2.active[0].contains("NOTE 追加一行"))
        XCTAssertTrue(bar2.actions.contains("撤销"))
        XCTAssertTrue(bar2.actions.contains("继续"))

        XCTAssertEqual(VSLogic.interruptSystemBar(TaskView()).actions, [])
    }

    // MARK: - 8. request_id / 执行阶段 / 打断词

    func testRequestId() {
        XCTAssertEqual(VSLogic.genRequestId().prefix(4), "req-")
        XCTAssertNotEqual(VSLogic.genRequestId(), VSLogic.genRequestId())
        XCTAssertGreaterThanOrEqual(VSLogic.execStages.count, 6)
        XCTAssertTrue(VSLogic.isInterruptPhrase("停"))
        XCTAssertTrue(VSLogic.isInterruptPhrase(" stop "))
        XCTAssertFalse(VSLogic.isInterruptPhrase("记一下"))
    }
}
