//
//  AppModel.swift
//  VoiceSign
//
//  服务对接 + 状态编排（对应 web/app.js）。纯判断/状态机集中在 VSLogic 与 SSEParser；
//  本类只做：状态持有、HTTP/SSE 编排、UI 驱动。对应控制流处写【伪代码逻辑层】注释块。
//

import Foundation
import SwiftUI
import Combine

// MARK: - 对话流模型

/// 对话流里的一行（用户/ Harness 气泡、三点、执行卡、回执卡）。
enum ChatRow: Identifiable {
    case user(Bubble)
    case harness(Bubble)
    case typing
    case execCard(ExecCardState)
    case receipt(ReceiptRow)

    var id: UUID {
        switch self {
        case .user(let b), .harness(let b): return b.id
        case .typing: return UUID(uuidString: "00000000-0000-0000-0000-000000000001")!
        case .execCard(let s): return s.id
        case .receipt(let r): return r.id
        }
    }
}

/// 回执行：带稳定 id（修复原 .receipt 每次 id=UUID() 导致 ForEach 身份漂移、渲染不出的 bug）。
struct ReceiptRow: Identifiable {
    let id = UUID()
    let receipt: Receipt
    let undo: UndoInfo
    let badges: [Badge]
}

struct Bubble: Identifiable {
    let id = UUID()
    var text: String
    var badges: [Badge] = []
    var fromVoice: Bool = false
}

/// 执行卡一行阶段状态。
struct StageState: Identifiable {
    let id = UUID()
    let name: String
    var done: Bool = false
    var active: Bool = false
}

struct ExecCardState: Identifiable {
    let id = UUID()
    var stages: [StageState]
}

// MARK: - AppModel

@MainActor
final class AppModel: ObservableObject {

    // 对话流
    @Published var rows: [ChatRow] = []
    @Published var decision: DecisionPoint? = nil
    @Published var systemBar: SystemBarInfo? = nil

    // 角色折叠条
    @Published var activeRole: String = "executor"
    @Published var roleBarOpen: Bool = false
    @Published var roles: [RoleInfo] = [
        RoleInfo(id: "planner", label: "Planner", active: false),
        RoleInfo(id: "executor", label: "Executor", active: true),
        RoleInfo(id: "verifier", label: "Verifier", active: false)
    ]

    // 设置
    @Published var showSettings: Bool = false
    @Published var statusLine: String = ""
    @Published var inputText: String = ""

    /// 诊断行（M7 真机排障）：上屏显示最近一次轮询状态/错误，避免黑盒"正在处理…"。
    @Published var diagLine: String = ""

    // 当前任务（打断/续跑用）
    private var currentTaskId: String?
    private var currentView: TaskView?
    private var sseTask: Task<Void, Never>?
    private var pollTask: Task<Void, Never>?
    private var lastSeq: Int = 0
    private var execCardRowId: UUID?

    /// 一轮语音识别结果（P1 一轮一清：发送/新一轮时清空）。由 SpeechRecognizer 写入。
    var pendingVoiceTranscript: String = ""

    private let api = APIClient.shared
    private let sse = SSEClient.shared

    init() {
        rows.append(.harness(Bubble(text: "你好，说点什么。\n语音优先 · 键盘兜底 · 一屏一个决策点")))
    }

    // MARK: - 发送入口
    //
    /**
     * 【伪代码逻辑层】（必写：发送→打断判定→提交）
     *   send(text):
     *     若 isInterruptPhrase(text) 且有进行中任务（currentTaskId 非终态）:
     *       → 走 maybeInterrupt(text)（红色系统条 + POST /v1/tasks/{id}/cancel）
     *     否则:
     *       → submit(text)
     */
    func send() {
        let text = inputText.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty else { return }
        inputText = ""
        appendUser(text, fromVoice: false)

        if VSLogic.isInterruptPhrase(text),
           let id = currentTaskId, let view = currentView,
           !VSLogic.isTerminal(view.status) {
            maybeInterrupt(taskId: id, view: view)
            return
        }
        submit(text)
    }

    // MARK: - 提交 → SSE 流转
    //
    /**
     * 【伪代码逻辑层】（必写：提交→SSE 事件→决策点流转）
     *   submit(text):
     *     reqId = genRequestId()                 // 客户端幂等键
     *     POST /v1/tasks {text,request_id} → {task_id}
     *     显示三点 typing
     *     openSSE(task_id)
     *   openSSE(id):
     *     GET /v1/tasks/{id}/events?after=lastSeq（断线重连）
     *     for await ev:
     *       stage     → 推进执行卡 + syncRole
     *       need_ask  → decision = ask(question, options)（停在确认闸）
     *       need_confirm → decision = confirm(question)
     *       done      → 停流；用 done 事件构 TaskView → 渲染回执卡 + 徽章
     *       failed    → 错误条
     *       interrupt → 红色系统条三语义
     *       canceled  → 终态错误条
     *     流异常（未到终态）→ 带 ?after=lastSeq 重连
     *   answer(id, ans):
     *     POST /v1/tasks/{id}/answer {answer:ans}
     *     → 清 decision；重新 openSSE(id) 续跑
     */
    func submit(_ text: String) {
        let reqId = VSLogic.genRequestId()
        rows.append(.typing)
        Task {
            do {
                let res = try await api.submitTask(text: text, requestId: reqId)
                removeTyping()
                // 多任务加固：新任务开始前清掉上一轮未决的 decision 与执行卡追踪，
                // 避免上一轮 SSE/轮询残留把新任务误判成旧状态（首条卡/次条好的时序矛盾）。
                decision = nil
                execCardRowId = nil
                currentTaskId = res.taskId
                currentView = TaskView(taskId: res.taskId, status: res.status)
                ensureExecCard()
                DiagLogger.shared.log("SUBMIT", "新任务 task=\(res.taskId) status=\(res.status ?? "-") reqId=\(reqId)")
                // P0：决策以轮询 GET /v1/tasks/{id} 为准（与 web/app.js 一致，证据来自 task json）；
                //     SSE 并行只驱动执行卡 stage 高亮（P2）。两者并行不冲突：轮询命中终态/决策点即停轮询。
                startFlow(taskId: res.taskId)
            } catch {
                removeTyping()
                appendHarness("提交失败：\(error.localizedDescription)（检查右上角 ⚙ server 地址/token）",
                              view: TaskView(status: "canceled"))
            }
        }
    }

    /// 一轮任务的双流编排：SSE stage 流 + 轮询决策。
    private func startFlow(taskId: String) {
        openSSE(taskId: taskId)
        startPolling(taskId: taskId)
    }

    /**
     * 【伪代码逻辑层】（必写：轮询 tick → 决策点路由）
     *   startPolling(id): 每 ~900ms GET /v1/tasks/{id}
     *   tick(view):
     *     running      → 同步角色；按状态推进执行卡；继续轮询
     *     need_ask     → 停轮询；收起执行卡；decision = ask(question, options)
     *     need_confirm → 停轮询；收起执行卡；decision = confirm(question)
     *     done         → 停轮询；收 SSE；渲染回执卡
     *     canceled/interrupted → 停轮询；错误条
     *   命中"一屏一个决策点"即停轮询，等待用户点选/确认后由 answer() 续跑。
     */
    private func startPolling(taskId: String) {
        pollTask?.cancel()
        DiagLogger.shared.log("POLL", "开始轮询 task=\(taskId) 间隔900ms")
        var failCount = 0
        pollTask = Task {
            while !Task.isCancelled {
                do {
                    let view = try await api.fetchTask(taskId)
                    failCount = 0
                    DiagLogger.shared.log("POLL", "task=\(taskId) status=\(view.status ?? "nil") options=\(view.options?.count ?? -1) question=\(view.question ?? "-")")
                    await MainActor.run {
                        self.diagLine = "轮询: \(view.status ?? "?")"
                        self.route(polled: view)
                    }
                    if isPollTerminal(view.status) {
                        DiagLogger.shared.log("POLL", "到达终态/决策点，停轮询 task=\(taskId)")
                        return
                    }
                } catch {
                    failCount += 1
                    DiagLogger.shared.log("POLL", "轮询第\(failCount)次失败: \(error.localizedDescription)")
                    await MainActor.run {
                        self.diagLine = "轮询失败x\(failCount): \(error.localizedDescription)"
                    }
                }
                try? await Task.sleep(nanoseconds: 900_000_000)
                if Task.isCancelled { return }
            }
        }
    }

    private func isPollTerminal(_ status: String?) -> Bool {
        guard let s = status else { return true }
        return s != "running"
    }

    @MainActor
    private func route(polled view: TaskView) {
        guard let id = currentTaskId, id == view.taskId else {
            DiagLogger.shared.log("ROUTE", "跳过轮询: taskId 不匹配 current=\(currentTaskId ?? "-") polled=\(view.taskId ?? "-")")
            return
        }
        currentView = view
        switch view.status {
        case "running":
            DiagLogger.shared.log("ROUTE", "running → 继续轮询")
            syncRole(VSLogic.roleForStatus(view.status))
            applyExecProgress(forStatus: view.status)
        case "need_ask", "need_confirm":
            DiagLogger.shared.log("ROUTE", "\(view.status) → 渲染决策点 question=\(view.question ?? "-") options=\(view.options?.count ?? 0)")
            stopPolling()
            applyExecProgress(forStatus: view.status)
            closeExecCard()
            decision = VSLogic.nextDecisionPoint(view)
            DiagLogger.shared.log("ROUTE", "decision.kind=\(String(describing: decision?.kind)) options=\(decision?.options.count ?? 0)")
        case "done":
            DiagLogger.shared.log("ROUTE", "done → 回执")
            stopPolling()
            sseTask?.cancel()
            syncRole(VSLogic.roleForStatus(view.status))
            closeExecCard()
            decision = nil
            renderReceipt(view)
            // 兜底：QUERY 等无四行 receipt 的任务，receipt 卡可能为空 → 补一条可见完成气泡，杜绝"done 了但界面无反应"。
            if view.receipt == nil || view.receipt?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == true {
                let text = view.question?.isEmpty == false ? "完成：\(view.question!)" : "完成（server 已返回 done）"
                appendHarness(text, view: view)
            }
        case "canceled", "interrupted":
            DiagLogger.shared.log("ROUTE", "\(view.status) → 错误条")
            stopPolling()
            closeExecCard()
            if systemBar == nil {
                decision = VSLogic.nextDecisionPoint(view)
            }
        default:
            DiagLogger.shared.log("ROUTE", "未知 status=\(view.status ?? "nil") → 无动作（吞掉？）")
            break
        }
    }

    private func stopPolling() {
        pollTask?.cancel()
        pollTask = nil
    }

    /// P2：按状态词推进执行卡（need_ask/confirm→确认闸；done→全✓）。
    private func applyExecProgress(forStatus status: String?) {
        let (doneCount, _) = VSLogic.execProgress(forStatus: status)
        guard doneCount > 0 else { return }
        for i in rows.indices {
            if case .execCard(var st) = rows[i] {
                for j in st.stages.indices {
                    st.stages[j].done = (j < doneCount)
                    st.stages[j].active = false
                }
                rows[i] = .execCard(st)
            }
        }
    }

    private func openSSE(taskId: String) {
        sseTask?.cancel()
        sseTask = Task {
            // 简单重连循环：未到终态则按 after=lastSeq 重连。
            while !Task.isCancelled {
                do {
                    let stream = sse.events(taskId: taskId, after: lastSeq)
                    for try await ev in stream {
                        if Task.isCancelled { break }
                        lastSeq = max(lastSeq, ev.seq ?? lastSeq)
                        handle(event: ev, taskId: taskId)
                        if ev.isTerminal {
                            return
                        }
                    }
                    return // 流正常结束（终态已处理）
                } catch {
                    // 断线：短暂退避后按 lastSeq 重连；连续失败给一次 UI 提示后停。
                    appendHarness("事件流断开，重连中…", view: TaskView(status: "running"))
                    try? await Task.sleep(nanoseconds: 800_000_000)
                    if Task.isCancelled { return }
                }
            }
        }
    }

    private func handle(event: SSEEvent, taskId: String) {
        switch event {
        case .stage(_, let role, _, let step):
            if let r = role { syncRole(r) }
            if let step = step { advanceExec(toStage: step) }

        case .ask(_, let question, let options):
            currentView = TaskView(taskId: taskId, status: "need_ask",
                                   question: question, options: options)
            closeExecCard()
            decision = VSLogic.nextDecisionPoint(currentView!)

        case .confirm(_, let question):
            currentView = TaskView(taskId: taskId, status: "need_confirm", question: question)
            closeExecCard()
            decision = VSLogic.nextDecisionPoint(currentView!)

        case .done(_, let receipt, let attribution, let reversible, let role):
            let view = TaskView(taskId: taskId, status: "done",
                                 receipt: receipt, attribution: attribution,
                                 reversible: reversible)
            currentView = view
            if let role = role { syncRole(role) }
            closeExecCard()
            decision = nil
            renderReceipt(view)

        case .failed(_, let error):
            currentView = TaskView(taskId: taskId, status: "canceled", error: error)
            closeExecCard()
            decision = DecisionPoint(kind: .error, message: error ?? "执行失败")

        case .interrupt(_, let applied, let notApplied, _):
            // 三语义 → 红色系统条（已生效/未执行/可撤销）。
            var base = VSLogic.interruptSystemBar(currentView)
            if !applied.isEmpty { base.active = applied }
            if !notApplied.isEmpty { base.blocked = notApplied }
            systemBar = base

        case .canceled:
            currentView = TaskView(taskId: taskId, status: "canceled")
            closeExecCard()
            if systemBar == nil {
                decision = DecisionPoint(kind: .error, message: "任务被取消")
            }

        case .unknown:
            break
        }
    }

    // MARK: - 应答 / 续跑

    func answer(_ ans: String) {
        guard let id = currentTaskId else { return }
        decision = nil
        Task {
            do {
                try await api.answer(id, ans)
                ensureExecCard()
                startFlow(taskId: id)   // need_ask 续跑 / need_confirm 放行后续跑
            } catch {
                appendHarness("应答失败：\(error.localizedDescription)", view: TaskView(status: "canceled"))
            }
        }
    }

    // MARK: - 撤销（rollback）

    func rollback() {
        guard let id = currentTaskId else { return }
        Task {
            do {
                let restored = try await api.rollback(id)
                appendHarness("已撤销：\(restored ?? "已恢复")",
                              view: TaskView(status: "done", reversible: false))
            } catch {
                appendHarness("撤销失败：\(error.localizedDescription)",
                              view: TaskView(status: "canceled"))
            }
        }
    }

    // MARK: - 打断：说"停" → 红色系统条
    //
    /**
     * 【伪代码逻辑层】（必写：停止→已生效/未执行/可继续或撤销）
     *   maybeInterrupt(taskId, view):
     *     1. systemBar = VSLogic.interruptSystemBar(view)
     *        — active: 已生效（有 receipt 列动作，否则"尚未变更"）
     *        — blocked: 未执行（后续阶段中止）
     *        — actions: ['撤销'(若可逆), '继续']
     *     2. POST /v1/tasks/{taskId}/cancel 真正停（404/405 回退 legacy /v1/cancel）
     *     3. SSE interrupt 事件到达后刷新 systemBar 三语义
     *     异常：cancel 端点失败 → 仅显示系统条，不阻塞用户。
     */
    private func maybeInterrupt(taskId: String, view: TaskView) {
        systemBar = VSLogic.interruptSystemBar(view)
        sseTask?.cancel()
        stopPolling()
        Task {
            do {
                try await api.cancel(taskId)
            } catch {
                // 仅 UI 条，不阻塞用户
            }
        }
    }

    func closeSystemBar() { systemBar = nil }

    // MARK: - 角色折叠

    private func syncRole(_ statusOrRole: String) {
        // stage 事件直接给 role；状态词则经 roleForStatus 裁决。
        let roleId: String
        if ["planner", "executor", "verifier"].contains(statusOrRole) {
            roleId = statusOrRole
        } else {
            roleId = VSLogic.roleForStatus(statusOrRole)
        }
        activeRole = roleId
        for i in roles.indices { roles[i].active = (roles[i].id == roleId) }
    }

    // MARK: - 设置页

    func testConnection() {
        statusLine = "连接中…"
        Task {
            do {
                let s = try await api.status()
                statusLine = "OK · v\(s.version ?? "?") · tasks=\(s.tasks ?? -1)"
            } catch {
                statusLine = "失败：\(error.localizedDescription)"
            }
        }
    }

    // MARK: - 对话流渲染助手

    private func appendUser(_ text: String, fromVoice: Bool) {
        rows.append(.user(Bubble(text: text, fromVoice: fromVoice)))
    }

    private func appendHarness(_ text: String, view: TaskView) {
        rows.append(.harness(Bubble(text: text, badges: VSLogic.compressBadges(view))))
    }

    private func removeTyping() {
        rows.removeAll {
            if case .typing = $0 { return true }
            return false
        }
    }

    private func ensureExecCard() {
        guard execCardRowId == nil else { return }
        let state = ExecCardState(stages: VSLogic.execStages.map { StageState(name: $0) })
        execCardRowId = state.id
        rows.append(.execCard(state))
    }

    /// 收到 stage step 名 → 把该阶段点亮并把此前的标 done（P2 逐项高亮）。
    private func advanceExec(toStage step: String) {
        guard let idx = VSLogic.execIndex(ofStep: step) else { return }
        for i in rows.indices {
            if case .execCard(var st) = rows[i] {
                for j in st.stages.indices {
                    st.stages[j].done = (j < idx)
                    st.stages[j].active = (j == idx)
                }
                rows[i] = .execCard(st)
            }
        }
    }

    private func closeExecCard() {
        execCardRowId = nil
    }

    private func renderReceipt(_ view: TaskView) {
        let dp = VSLogic.nextDecisionPoint(view)
        guard dp.kind == .receipt else { return }
        rows.append(.receipt(ReceiptRow(receipt: dp.receipt,
                                        undo: dp.undo,
                                        badges: VSLogic.compressBadges(view))))
    }
}
