//
//  AppModel.swift
//  VoxSign
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
    /// UI v3：语音消息录音秒数（气泡内显示 "3″"）。
    var voiceSeconds: Int? = nil
    /// v2.4：随本条消息提交的资料附件。
    var attachments: [Attachment] = []
    /// v2.4：本条回复消耗的 token（harness 气泡可显示）。
    var costTokens: Int? = nil
    /// v2.4：消息时间戳（会话历史排序/显示用）。
    var timestamp: Date = Date()
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

// MARK: - Harness 状态（UI v3 豆包式顶栏 5pt 状态点，真实推导）

enum HarnessState: Equatable {
    case idle       // 灰：无任务
    case busy       // 蓝呼吸：任务执行中
    case decision   // 橙：等待用户确认/选择
}

// MARK: - AppModel

@MainActor
final class AppModel: ObservableObject {

    // 对话流
    @Published var rows: [ChatRow] = []
    @Published var decision: DecisionPoint? = nil
    @Published var systemBar: SystemBarInfo? = nil

    /// UI v3：顶栏状态点数据源（busy / decision / idle）。
    @Published var harnessState: HarnessState = .idle

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

    // v2.4 多会话（默认隐藏，不占主界面）
    @Published var showSessions: Bool = false
    @Published var currentSessionID: String = ""
    var sessions: [ChatSession] { SessionStore.shared.sessions }

    // v2.4 附件（资料）：输入条待提交的附件
    @Published var pendingAttachments: [Attachment] = []

    /// 诊断行（M7 真机排障）：上屏显示最近一次轮询状态/错误，避免黑盒"正在处理…"。
    @Published var diagLine: String = ""

    /// T2 滚动修复：每次追加/替换气泡后 +1，RootView 监听它滚动到底。
    /// 原来监听 rows.count 在"移除三点→追加新行"连续变化时会漏触发，
    /// 导致用户说完话看不到 Harness 的后续内容。
    @Published var scrollTick: Int = 0

    /// T2 语音残留修复：语音提交后 1.5s 冷却期内，抑制识别 partial 回写输入框。
    /// 否则旧音频尾音/新段残留会被识别成字，再次出现在输入框（用户看到"前面输入带进来"）。
    @Published var voiceCooldown: Bool = false

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
    /// v2.3：本轮提交时刻（渲染"已处理 X 秒"用）。
    private var lastSubmitAt = Date()
    /// T2 连接感知：网络恢复自动补投的订阅（取消时清理）。
    private var connSub: AnyCancellable?

    init() {
        // v2.4 多会话：确保至少一个会话，并把当前对话流 rows 还原成该会话的历史消息。
        // UI v3（豆包式空态）：新会话只有一行灰字空态（"说点什么，或按住下方按钮说话"）。
        SessionStore.shared.ensureInitialSession()
        currentSessionID = SessionStore.shared.currentSessionID
        rows = Self.messagesToRows(SessionStore.shared.loadMessages(for: currentSessionID))

        // T2 豆包式交互：网络恢复 → 自动补投离线队列（不丢语音指令）。
        connSub = ConnectivityService.shared.onOnline { [weak self] in
            guard let self = self else { return }
            Task { await self.flushQueue() }
        }
        // T2 豆包式交互：常听语音识别到完整一句话 → 自动提交（开口即达，无需按按钮）。
        #if canImport(Speech)
        SpeechRecognizer.shared.onFinalSegment = { [weak self] text in
            guard let self = self, !text.isEmpty else { return }
            Task { @MainActor in self.sendVoice(text) }
        }
        #endif
    }

    /// T2 语音专用发送：与键盘 send 同链路，但气泡标记 fromVoice + 清一轮识别缓冲。
    /// v2.1：决策点存在时语音优先口答（I06/I13/I17）；极短噪声词不提交（先进理念6）。
    func sendVoice(_ text: String) {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !t.isEmpty else { return }
        #if canImport(Speech)
        SpeechRecognizer.shared.resetRound()
        #endif
        // T3 豆包式：用户开口时立即打断上一段朗读（先听用户说）。
        VoiceOutputService.shared.stop()

        // v2.3 用户指令（2026-10-04）"所有拦截都去掉，不要替别人做决定"：
        // 删除本地口答决策拦截层（确认/选项/撤销/否定词判断）——语音一律作为新指令直通后台，
        // 不再弹"我听到的是「不要」…"类确认，避免含"不要/不用"的正常长句被误拦导致无反馈。
        // isNoiseWord 哼哈词过滤保留：防止"好/嗯"这类词提交后触发后台"你想让我做什么"回问。
        if VSLogic.isNoiseWord(t) {
            DiagLogger.shared.log("ASR", "噪声词已忽略: \(t)")
            return
        }
        appendUser(t, fromVoice: true)

        if VSLogic.isInterruptPhrase(t),
           let id = currentTaskId, let view = currentView,
           !VSLogic.isTerminal(view.status) {
            maybeInterrupt(taskId: id, view: view)
            return
        }
        submit(t)
    }

    /// v2.1 口答决策：确认/选项/撤销。返回 true=已作为口答消费（不再提交为新任务）。
    private func handleVoiceDecision(_ t: String) -> Bool {
        // I17 / 先进理念2 语音撤销链：说"撤销"回滚最近一次可撤销操作。
        if VSLogic.isUndoPhrase(t) {
            if let row = rows.last, case .receipt(let r) = row, r.undo.show {
                speak("好，撤销刚才的操作。")
                rollback()
                return true
            }
            return false   // 没有可撤销对象 → 不当撤销消费，按普通指令走
        }
        guard let d = decision else { return false }
        switch d.kind {
        case .confirm:
            // I13 分险级：高风险动作（提交/推送/合并/部署/删除…）口答不生效，强制按钮。
            let probe = (currentView?.receipt ?? "") + (currentView?.question ?? "")
            if VSLogic.isHighRiskAction(probe) {
                speak("这是高风险操作，请在下方点击按钮确认。")
                return true
            }
            if VSLogic.isAffirmPhrase(t) { answer("执行"); return true }
            if VSLogic.isNegativePhrase(t) { answer("拒绝"); return true }
            return false
        case .ask:
            if let hit = d.options.first(where: {
                t.contains($0.label) || $0.label.contains(t) || t.contains($0.id) || $0.id.contains(t)
            }) {
                answer(hit.id)
                return true
            }
            // "都可以/随便/听你的" → 第一选项（豆包式自然应答）。
            if VSLogic.isAffirmPhrase(t), let first = d.options.first {
                answer(first.id)
                return true
            }
            return false
        default:
            return false
        }
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
        let atts = pendingAttachments
        appendUser(text, fromVoice: false, attachments: atts)

        if VSLogic.isInterruptPhrase(text),
           let id = currentTaskId, let view = currentView,
           !VSLogic.isTerminal(view.status) {
            maybeInterrupt(taskId: id, view: view)
            return
        }
        submit(text, attachments: atts)
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
    func submit(_ text: String, attachments: [Attachment] = []) {
        let reqId = VSLogic.genRequestId()
        // v2.3（用户需求：微信式"已处理 X 秒"）：记录提交时刻，done 时算耗时。
        lastSubmitAt = Date()
        // UI v3：任务开始 → 顶栏状态点 busy（蓝呼吸）。
        harnessState = .busy
        // v2.2：先清上一轮残留中间态（typing/execCard），再开始本轮——连续说话不堆积"正在思考"。
        closeExecCard()
        rows.append(.typing)
        armLongTaskTimers()
        Task {
            do {
                // T2 连接感知：提交前探测——不在线不傻等 15s 超时，直接进离线队列。
                guard await ConnectivityService.shared.isReachable() else {
                    removeTyping()
                    if DeliveryQueue.shared.enqueue(PendingSubmission(requestId: reqId, text: text, mode: "text")) {
                        DiagLogger.shared.log("QUEUE", "离线直接入队 reqId=\(reqId) 队列=\(DeliveryQueue.shared.count)条")
                        appendHarness("当前不在线（\(ConnectivityService.shared.lastError)），指令已进入离线队列（\(DeliveryQueue.shared.count) 条待投递），联网后自动补投。",
                                      view: TaskView(status: "canceled"))
                    } else {
                        appendHarness("当前不在线，且离线队列已满，请联网后重试。",
                                      view: TaskView(status: "canceled"))
                    }
                    return
                }
                let res = try await api.submitTask(text: text, requestId: reqId, attachments: attachments)
                // v2.4：提交成功后清空待提交附件（断网入队/失败分支不清，附件随在线提交链路）。
                pendingAttachments = []
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
                // T1 后台能力：提交失败（断网/服务不可达/后台挂起）→ 入投递队列，
                // 网络恢复后按 request_id 幂等补投；不丢语音输入。
                removeTyping()
                if DeliveryQueue.shared.enqueue(PendingSubmission(requestId: reqId, text: text, mode: "text")) {
                    DiagLogger.shared.log("QUEUE", "提交失败已入队 reqId=\(reqId) 队列=\(DeliveryQueue.shared.count)条 err=\(error.localizedDescription)")
                    appendHarness("网络暂不可达，已进入离线队列（\(DeliveryQueue.shared.count) 条待投递），恢复后自动补投。",
                                  view: TaskView(status: "canceled"))
                } else {
                    appendHarness("提交失败：\(error.localizedDescription)（检查右上角 ⚙ server 地址/token）",
                                  view: TaskView(status: "canceled"))
                }
            }
        }
    }

    /// T1 后台能力：手动触发队列补投（网络恢复回调/设置页"补投"按钮共用）。
    /// - Returns: 本次补投条数。
    @discardableResult
    func flushQueue() async -> Int {
        let n = await DeliveryQueue.shared.flush()
        if n > 0 {
            DiagLogger.shared.log("QUEUE", "补投成功 \(n) 条，剩余 \(DeliveryQueue.shared.count)")
            appendHarness("离线队列已补投 \(n) 条。", view: TaskView(status: "done"))
        }
        return n
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
                        // T2 UI 简化：正常轮询不再写诊断行（不再刷屏"轮询: running"），
                        // 只有到达终态/决策点才留痕；失败走下面的 catch 单独显示。
                        if view.status != "running" {
                            self.diagLine = "轮询: \(view.status ?? "?")"
                        }
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
            harnessState = .busy
            syncRole(VSLogic.roleForStatus(view.status))
            applyExecProgress(forStatus: view.status)
        case "need_ask", "need_confirm":
            DiagLogger.shared.log("ROUTE", "\(view.status) → 渲染决策点 question=\(view.question ?? "-") options=\(view.options?.count ?? 0)")
            stopPolling()
            applyExecProgress(forStatus: view.status)
            closeExecCard()
            // UI v3：等待确认/选择 → 顶栏状态点 decision（橙）。
            harnessState = .decision
            decision = VSLogic.nextDecisionPoint(view)
            // T3 豆包式：需要用户确认/选择时朗读问题（不看屏也能应答）。
            if let q = view.question, !q.isEmpty {
                speak(q)
            }
            DiagLogger.shared.log("ROUTE", "decision.kind=\(String(describing: decision?.kind)) options=\(decision?.options.count ?? 0)")
        case "done":
            DiagLogger.shared.log("ROUTE", "done → 回执")
            stopPolling()
            sseTask?.cancel()
            // UI v3：任务完成 → 顶栏状态点回 idle（灰）。
            harnessState = .idle
            syncRole(VSLogic.roleForStatus(view.status))
            closeExecCard()
            decision = nil
            renderReceipt(view)
            // 兜底：QUERY 等无四行 receipt 的任务，receipt 卡可能为空 → 补一条可见完成气泡，杜绝"done 了但界面无反应"。
            if view.receipt == nil || view.receipt?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == true {
                let text = view.question?.isEmpty == false ? "完成：\(view.question!)" : "完成（server 已返回 done）"
                appendHarness(text, view: view, spoken: true)
            }
        case "canceled", "interrupted":
            DiagLogger.shared.log("ROUTE", "\(view.status) → 错误条")
            stopPolling()
            closeExecCard()
            // UI v3：任务终止 → 顶栏状态点回 idle（灰）。
            harnessState = .idle
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

    /// SSE 连续失败计数（T2：指数退避 + 上限，杜绝无限"重连中"刷屏）。
    private var reconnectFailures = 0

    private func openSSE(taskId: String) {
        sseTask?.cancel()
        reconnectFailures = 0
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
                    // T2：指数退避重连（0.8s→1.6s→…上限 4s），连续失败 5 次后停止。
                    // v2.2 修复（用户反馈"根本啥也没有"）：重连提示**不再进对话流**——
                    // 只在诊断日志留痕，顶部胶囊由 ConnectivityService 实时反映，避免刷屏顶掉回复。
                    reconnectFailures += 1
                    DiagLogger.shared.log("SSE", "重连中 failures=\(reconnectFailures) err=\(error.localizedDescription)")
                    if reconnectFailures >= 5 {
                        DiagLogger.shared.log("SSE", "重连失败5次停止，task=\(taskId)")
                        stopPolling()
                        return
                    }
                    let delay = min(Double(reconnectFailures) * 0.8, 4.0)
                    try? await Task.sleep(nanoseconds: UInt64(delay * 1_000_000_000))
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
            // T1：后台/锁屏时本地通知提醒（前台由 UI 呈现）。
            NotificationService.shared.routeEvent("need_ask", taskId: taskId, seq: 0,
                                                  payload: ["question": question ?? ""])

        case .confirm(_, let question):
            currentView = TaskView(taskId: taskId, status: "need_confirm", question: question)
            closeExecCard()
            decision = VSLogic.nextDecisionPoint(currentView!)
            NotificationService.shared.routeEvent("need_confirm", taskId: taskId, seq: 0,
                                                  payload: ["question": question ?? ""])

        case .done(_, let receipt, let attribution, let reversible, let role):
            let view = TaskView(taskId: taskId, status: "done",
                                 receipt: receipt, attribution: attribution,
                                 reversible: reversible)
            currentView = view
            if let role = role { syncRole(role) }
            closeExecCard()
            decision = nil
            renderReceipt(view)
            NotificationService.shared.routeEvent("done", taskId: taskId, seq: 0, payload: [:])

        case .failed(_, let error):
            currentView = TaskView(taskId: taskId, status: "canceled", error: error)
            closeExecCard()
            decision = DecisionPoint(kind: .error, message: error ?? "执行失败")
            NotificationService.shared.routeEvent("failed", taskId: taskId, seq: 0,
                                                  payload: ["error": error ?? ""])

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
            NotificationService.shared.routeEvent("canceled", taskId: taskId, seq: 0, payload: [:])

        case .unknown:
            break
        }
    }

    // MARK: - 应答 / 续跑

    func answer(_ ans: String) {
        guard let id = currentTaskId else { return }
        decision = nil
        // UI v3：应答后任务续跑 → 顶栏状态点 busy（蓝）。
        harnessState = .busy
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

    private func appendUser(_ text: String, fromVoice: Bool, attachments: [Attachment] = []) {
        var bubble = Bubble(text: text, fromVoice: fromVoice, attachments: attachments)
        #if canImport(Speech)
        if fromVoice, SpeechRecognizer.shared.lastHoldSeconds > 0 {
            bubble.voiceSeconds = SpeechRecognizer.shared.lastHoldSeconds
        }
        #endif
        rows.append(.user(bubble))
        scrollTick += 1
    }

    /// T3 豆包式：Harness 的"真回复"（完成/回执/决策点）朗读；系统提示不读。
    /// v2.1 自适应朗读（先进理念3）：短文本全文读；长文本读摘要+提示看屏（治 EC 推演的 TTS 瓶颈）。
    private func speak(_ text: String) {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !t.isEmpty else { return }
        if t.count > 80 {
            let head = String(t.prefix(80))
            VoiceOutputService.shared.speak(head + "。内容较长，详情可以看屏幕。")
        } else {
            VoiceOutputService.shared.speak(t)
        }
    }

    private func appendHarness(_ text: String, view: TaskView, spoken: Bool = false) {
        rows.append(.harness(Bubble(text: text, badges: VSLogic.compressBadges(view))))
        scrollTick += 1
        if spoken { speak(text) }
    }

    private func removeTyping() {
        longTaskTimer?.cancel()
        typingText = "正在思考…"
        rows.removeAll {
            if case .typing = $0 { return true }
            return false
        }
    }

    // MARK: - v2.1 I18 长任务提示升级（5s → 10s）

    /// 思考态动态文案（TypingView 显示）。
    @Published var typingText = "正在思考…"
    private var longTaskTimer: Task<Void, Never>?

    /// 提交后启动：5s 升级"还在思考，马上好…"，10s 改"任务比较久，完成了我通知你"（通知兜底由后台完成）。
    private func armLongTaskTimers() {
        longTaskTimer?.cancel()
        typingText = "正在思考…"
        longTaskTimer = Task {
            try? await Task.sleep(nanoseconds: 5_000_000_000)
            guard !Task.isCancelled else { return }
            await MainActor.run { self.typingText = "还在思考，马上好…" }
            try? await Task.sleep(nanoseconds: 5_000_000_000)
            guard !Task.isCancelled else { return }
            await MainActor.run { self.typingText = "任务比较久，完成了我通知你" }
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
        longTaskTimer?.cancel()
        typingText = "正在思考…"
        execCardRowId = nil
        // v2.2 修复：终态到达时删除 rows 里残留的 execCard/typing 中间态气泡，
        // 只留"用户气泡 + 最终回复"（豆包式干净对话流，不再一轮轮堆积"正在思考"）。
        rows.removeAll {
            if case .execCard = $0 { return true }
            if case .typing = $0 { return true }
            return false
        }
    }

    private func renderReceipt(_ view: TaskView) {
        let dp = VSLogic.nextDecisionPoint(view)
        guard dp.kind == .receipt else { return }
        var receipt = dp.receipt
        // v2.3（用户需求：微信式"已处理 X 秒"）：记录本轮耗时，气泡上方显示。
        receipt.elapsedSec = Date().timeIntervalSince(lastSubmitAt)
        rows.append(.receipt(ReceiptRow(receipt: receipt,
                                        undo: dp.undo,
                                        badges: VSLogic.compressBadges(view))))
        scrollTick += 1
        // T3 豆包式：朗读回复内容（v2.3 去"已完成，动作。"前缀，用户原话：已完成什么东西）。
        let summary = receipt.result
        speak(summary)
    }

    // MARK: - v2.4 附件（资料）

    func addAttachment(_ a: Attachment) {
        pendingAttachments.append(a)
    }

    func removeAttachment(_ id: String) {
        pendingAttachments.removeAll { $0.id == id }
    }

    /// 兼容重载：视图层以附件对象形式调用 removeAttachment(_:)。
    func removeAttachment(_ a: Attachment) {
        pendingAttachments.removeAll { $0.id == a.id }
    }

    // MARK: - v2.4 多会话切换

    /// 切到指定会话：先把当前 rows 回写原会话 → switchTo → 加载新会话 rows → 清理中间态。
    func switchSession(_ id: String) {
        guard id != currentSessionID else { return }
        persistCurrentRows()
        SessionStore.shared.switchTo(id: id)
        currentSessionID = id
        loadCurrentRows()
        resetTransientState()
    }

    /// 新会话：先保存当前会话 → createSession → 加载空 rows → 清理中间态。
    func newSession() {
        persistCurrentRows()
        let s = SessionStore.shared.createSession(title: "新会话")
        currentSessionID = s.id
        loadCurrentRows()
        resetTransientState()
    }

    /// 删除会话：先保存当前 rows；删后若删的是当前会话则切到剩余第一个并加载。
    @discardableResult
    func deleteSession(_ id: String) -> Bool {
        persistCurrentRows()
        let ok = SessionStore.shared.deleteSession(id: id)
        guard ok else { return false }
        currentSessionID = SessionStore.shared.currentSessionID
        loadCurrentRows()
        resetTransientState()
        return true
    }

    // MARK: - v2.4 会话内部辅助

    /// 把当前对话流 rows 回写到当前会话。
    private func persistCurrentRows() {
        guard !currentSessionID.isEmpty else { return }
        SessionStore.shared.saveMessages(Self.rowsToMessages(rows), for: currentSessionID)
    }

    /// 按当前 currentSessionID 从仓库加载对话流 rows。
    private func loadCurrentRows() {
        rows = Self.messagesToRows(SessionStore.shared.loadMessages(for: currentSessionID))
    }

    /// 切/删/新建会话后清理中间态（决策点/系统条/附件/输入/进行中任务）。
    private func resetTransientState() {
        decision = nil
        systemBar = nil
        pendingAttachments = []
        inputText = ""
        sseTask?.cancel()
        pollTask?.cancel()
        sseTask = nil
        pollTask = nil
        currentTaskId = nil
        currentView = nil
        execCardRowId = nil
        longTaskTimer?.cancel()
        scrollTick += 1
    }

    // MARK: - ChatRow ↔ StoredMessage 转换

    /// rows → 可持久化消息（typing/execCard 不持久化；回执行→harness 文本，undo 不持久化）。
    static func rowsToMessages(_ rows: [ChatRow]) -> [StoredMessage] {
        rows.compactMap { row -> StoredMessage? in
            switch row {
            case .user(let b):
                return StoredMessage(id: b.id.uuidString, role: "user", text: b.text,
                                     fromVoice: b.fromVoice, voiceSeconds: b.voiceSeconds,
                                     attachments: b.attachments, timestamp: b.timestamp)
            case .harness(let b):
                return StoredMessage(id: b.id.uuidString, role: "harness", text: b.text,
                                     badges: b.badges, attachments: b.attachments,
                                     costTokens: b.costTokens, timestamp: b.timestamp)
            case .receipt(let r):
                return StoredMessage(id: r.id.uuidString, role: "harness", text: r.receipt.result,
                                     badges: r.badges, elapsedSec: r.receipt.elapsedSec)
            case .typing, .execCard:
                return nil
            }
        }
    }

    /// 持久化消息 → rows（user 还原气泡含附件/语音；harness 还原徽章/token/时间）。
    static func messagesToRows(_ messages: [StoredMessage]) -> [ChatRow] {
        messages.map { m -> ChatRow in
            switch m.role {
            case "user":
                var b = Bubble(text: m.text, fromVoice: m.fromVoice, attachments: m.attachments)
                b.voiceSeconds = m.voiceSeconds
                b.timestamp = m.timestamp
                return .user(b)
            default:
                // harness 消息（含回执结果文本）：还原为普通 harness 气泡。
                var b = Bubble(text: m.text, badges: m.badges, attachments: m.attachments)
                b.costTokens = m.costTokens
                b.timestamp = m.timestamp
                return .harness(b)
            }
        }
    }
}
