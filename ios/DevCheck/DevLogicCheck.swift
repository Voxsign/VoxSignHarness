//
//  DevLogicCheck.swift — macOS 命令行断言（绕过模拟器沙箱，直接执行同一套纯逻辑）。
//  运行：swiftc -o /tmp/vscheck DevLogicCheck.swift ../VoiceSign/Core/Models.swift ../VoiceSign/Core/VSLogic.swift ../VoiceSign/Core/SSEParser.swift && /tmp/vscheck
//  注：此文件不在 Xcode 目标内，仅用于开发期在本机直接跑逻辑断言。
//

import Foundation

var pass = 0, fail = 0
func eq<T: Equatable>(_ a: T, _ b: T, _ name: String) {
    if a == b { pass += 1; print("  ✓ \(name)") }
    else { fail += 1; print("  ✗ \(name)\n      expected: \(b)\n      actual:   \(a)") }
}
func ok(_ c: Bool, _ name: String) { eq(c, true, name) }

print("== 1. 终态/决策点 ==")
eq(VSLogic.isTerminal("done"), true, "done 终态")
eq(VSLogic.isTerminal("canceled"), true, "canceled 终态")
eq(VSLogic.isTerminal("interrupted"), true, "interrupted 终态")
eq(VSLogic.isTerminal("running"), false, "running 非终态")
eq(VSLogic.isDecision("need_confirm"), true, "need_confirm 决策点")
eq(VSLogic.isDecision("done"), false, "done 非决策点")

print("== 2. 回执四行 ==")
let rec = "动作：NOTE 追加一行\n文件：notes.md\n结果：OK 已追加\n撤销：从备份 notes.md.20261002T1530.bak 恢复"
let r = VSLogic.parseReceipt(rec)
eq(r.action, "NOTE 追加一行", "动作行")
eq(r.files, "notes.md", "文件行")
eq(r.result, "OK 已追加", "结果行")
eq(r.undo, "从备份 notes.md.20261002T1530.bak 恢复", "撤销行")
eq(VSLogic.parseReceipt(""), Receipt(), "空回执不炸")
eq(VSLogic.parseReceipt("动作:COMMIT\n撤销：不可撤销（不可逆，已人工确认）").undo,
   "不可撤销（不可逆，已人工确认）", "半角冒号")

print("== 3. 撤销按钮 ==")
let u = VSLogic.extractUndo(r, true)
eq(u.show, true, "可逆+有.bak → 显示")
eq(u.backup, "notes.md.20261002T1530.bak", "备份名提取")
eq(u.irreversible, false, "非不可逆")
eq(VSLogic.extractUndo(VSLogic.parseReceipt("动作：COMMIT\n撤销：不可撤销"), true).show, false, "声明不可撤销不给按钮")
eq(VSLogic.extractUndo(r, false).show, false, "reversible=false 不给按钮")
eq(VSLogic.extractUndo(VSLogic.parseReceipt("撤销：VHS_BACKUP_PATH: notes.md.20261002T.bak"), true).backup,
   "notes.md.20261002T.bak", "VHS_BACKUP_PATH 前缀")

print("== 4. 轻标签 ==")
let bs = VSLogic.compressBadges(TaskView(status: "running", receipt: rec, reversible: true))
ok(bs.contains { $0.label == "执行中" && $0.tone == "blue" }, "running→执行中")
ok(bs.contains { $0.label == "笔记" && $0.kind == "intent" }, "NOTE→笔记")
ok(bs.contains { $0.label == "笔记域" }, "notes.md→笔记域")
ok(bs.contains { $0.label == "可逆" && $0.tone == "green" }, "reversible→可逆")
ok(VSLogic.compressBadges(TaskView(status: "need_confirm", question: "x")).contains { $0.label == "高风险·待放行" },
   "need_confirm→高风险")
ok(VSLogic.compressBadges(TaskView(status: "done", receipt: "动作：COMMIT", reversible: false)).contains { $0.label == "不可逆" },
   "done 不可逆→不可逆")

print("== 5. 决策点路由 ==")
eq(VSLogic.nextDecisionPoint(TaskView(status: "need_confirm", question: "放行")).kind, .confirm, "confirm")
let ask = VSLogic.nextDecisionPoint(TaskView(status: "need_ask", question: "哪个",
    options: [TaskOption(id: "f1", label: "a"), TaskOption(id: "f2", label: "b")]))
eq(ask.kind, .ask, "ask")
eq(ask.options.count, 2, "候选数")
eq(VSLogic.nextDecisionPoint(TaskView(status: "done", receipt: rec, reversible: true)).kind, .receipt, "receipt")
eq(VSLogic.nextDecisionPoint(TaskView(status: "running")).kind, .running, "running")
eq(VSLogic.nextDecisionPoint(TaskView(status: "canceled", error: "x")).kind, .error, "error")
eq(VSLogic.nextDecisionPoint(TaskView(status: "interrupted")).kind, .error, "interrupted error")

print("== 6. 角色 ==")
eq(VSLogic.roleForStatus("need_ask"), "planner", "回问→planner")
eq(VSLogic.roleForStatus("running"), "executor", "执行→executor")
eq(VSLogic.roleForStatus("done"), "verifier", "完成→verifier")

print("== 7. 打断 ==")
let bar1 = VSLogic.interruptSystemBar(TaskView(status: "running"))
ok(bar1.active[0].contains("尚未产生文件变更"), "running→尚未变更")
eq(bar1.actions, ["继续"], "running 动作")
let bar2 = VSLogic.interruptSystemBar(TaskView(status: "done", receipt: rec, reversible: true))
ok(bar2.active[0].contains("NOTE 追加一行"), "done→列动作")
ok(bar2.actions.contains("撤销") && bar2.actions.contains("继续"), "可逆→撤销+继续")
eq(VSLogic.interruptSystemBar(TaskView()).actions, [], "空→无动作")

print("== 8. request_id / 打断词 ==")
ok(VSLogic.genRequestId().hasPrefix("req-"), "req 前缀")
ok(VSLogic.genRequestId() != VSLogic.genRequestId(), "唯一性")
ok(VSLogic.isInterruptPhrase("停"), "停")
ok(VSLogic.isInterruptPhrase(" stop "), "stop")
ok(!VSLogic.isInterruptPhrase("记一下"), "非打断词")

print("== 9. SSE 解析 ==")
let stream =
    "event: stage\ndata: {\"seq\":1,\"role\":\"planner\",\"step\":\"意图分类\"}\n\n" +
    "event: need_ask\ndata: {\"seq\":2,\"question\":\"哪个？\",\"options\":[{\"id\":\"f1\",\"label\":\"x\"}]}\n\n"
let p = SSEParser()
let evs = p.feed(stream)
eq(evs.count, 2, "双事件")
if case .stage(let s, let role, _, let step) = evs[0] {
    eq(s, 1, "stage seq"); eq(role, "planner", "stage role"); eq(step, "意图分类", "stage step")
} else { ok(false, "stage 类型") }
if case .ask(let s2, let q, let opts) = evs[1] {
    eq(s2, 2, "ask seq"); eq(q, "哪个？", "ask question"); eq(opts.count, 1, "ask options")
} else { ok(false, "ask 类型") }
eq(p.lastSeq, 2, "lastSeq=2")

// 半截帧
let p2 = SSEParser()
eq(p2.feed("event: done\ndata: {\"seq\":5,\"receipt\":\"动作：X\"}").count, 0, "半截不出")
let doneEvts = p2.feed("\n\n")
eq(doneEvts.count, 1, "补全后出")
if case .done(let s3, let receipt, _, _, _) = doneEvts[0] {
    eq(s3, 5, "done seq"); eq(receipt, "动作：X", "done receipt"); ok(doneEvts[0].isTerminal, "done 终态")
} else { ok(false, "done 类型") }

// 打断三语义
let p3 = SSEParser()
let intEvts = p3.feed("event: interrupt\ndata: {\"seq\":7,\"applied\":[\"已生效：NOTE（notes.md）\"],\"notApplied\":[\"后续已中止\"],\"canRollback\":true}\n\n")
if case .interrupt(_, let applied, let notApplied, let rb) = intEvts[0] {
    eq(applied.count, 1, "applied"); eq(notApplied, ["后续已中止"], "notApplied"); eq(rb, true, "canRollback")
} else { ok(false, "interrupt 类型") }

// 重连 URL
let url = SSEParser.reconnectURL(base: URL(string: "http://h/v1/tasks/t/events")!, after: 7)
ok(url.absoluteString.contains("after=7"), "重连 after=7")

print("== 10. M7 快速可用：执行卡高亮 / 语音一轮一清 / need_ask 决策 ==")
// P2 执行卡 stage 索引
eq(VSLogic.execIndex(ofStep: "意图分类"), 0, "stage 意图分类→0")
eq(VSLogic.execIndex(ofStep: "确认闸"), 3, "stage 确认闸→3")
eq(VSLogic.execIndex(ofStep: "不存在"), nil as Int?, "未知 stage→nil")
// P2 执行卡按状态推进
let progAsk = VSLogic.execProgress(forStatus: "need_ask")
eq(progAsk.doneCount, 4, "need_ask→此前4行✓")
eq(VSLogic.execProgress(forStatus: "done").doneCount, VSLogic.execStages.count, "done→全✓")
eq(VSLogic.execProgress(forStatus: "running").doneCount, 0, "running→不抢跑（交 stage 事件）")
// P1 语音一轮一清：final 整段替换，不拼接
eq(VSLogic.voiceReplace(previous: "旧的半句话", final: "今天天气"), "今天天气", "final 整段替换不拼接")
// P0 need_ask 渲染决策点（轮询命中 task json → 必须出 ask 带 options）
let polledAsk = VSLogic.nextDecisionPoint(TaskView(taskId: "t1", status: "need_ask",
    question: "你说的「这个」指的是哪个？",
    options: [TaskOption(id: "edit", label: "改笔记"),
              TaskOption(id: "query", label: "查一下"),
              TaskOption(id: "note", label: "记下来"),
              TaskOption(id: "commit", label: "提交")]))
eq(polledAsk.kind, .ask, "need_ask→ask 决策点")
eq(polledAsk.options.count, 4, "4 个候选按钮")
eq(polledAsk.options[0].id, "edit", "候选 id 原样回传（answer 用）")

print("\n----------------------------------------")
print("结果: \(pass) 通过, \(fail) 失败")
exit(fail == 0 ? 0 : 1)
