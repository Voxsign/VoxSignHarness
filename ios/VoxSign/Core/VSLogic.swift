//
//  VSLogic.swift
//  VoxSign
//
//  纯逻辑层（无网络、无 UI、无 DispatchQueue）——1:1 移植自 web/logic.js（VSLogic）。
//  所有"判断/裁决/状态机"集中在这里，便于 XCTest 断言与后续复用。
//  纯渲染/样式豁免（不在此文件）。
//

import Foundation

enum VSLogic {

    /// 执行卡七阶段（对齐 pipeline 用户可见决策/执行点）。
    static let execStages: [String] = [
        "意图分类", "域裁决", "风险分级", "确认闸", "执行", "校验", "归因"
    ]

    static let roleLabels: [String: String] = [
        "planner": "Planner",
        "executor": "Executor",
        "verifier": "Verifier"
    ]

    // MARK: - 终态 / 决策点状态词（对齐 INTERACT-v1）

    /// 终态：done/canceled/interrupted（轮询到此停）。
    static func isTerminal(_ status: String?) -> Bool {
        guard let s = status else { return false }
        return s == "done" || s == "canceled" || s == "interrupted"
    }

    /// 决策点：need_ask/need_confirm（挂起，暂停轮询等用户 answer）。
    static func isDecision(_ status: String?) -> Bool {
        guard let s = status else { return false }
        return s == "need_ask" || s == "need_confirm"
    }

    // MARK: - request_id（M4 幂等键）

    /// 客户端生成；重试同一任务复传 → server 去重（deduped:true）。
    static func genRequestId() -> String {
        return "req-" + UUID().uuidString.lowercased()
    }

    // MARK: - D0 契约：outcome.reply 规范化（Phase 1）
    //
    /**
     * 【伪代码逻辑层】（必写：reply 是服务端下发的"回答正文"，客户端唯一可信来源）
     *   wire 形态 Phase1 = 纯文本字符串：  "reply": "今天天气晴"
     *   防御性兼容对象形态：              "reply": {"text": "今天天气晴"}
     *   规则：
     *     - 字符串：非空即直取；
     *     - 字典：取 ["text"] 字符串；
     *     - 其余 / 空串 → nil（上层按"无回答"诚实渲染，禁止伪造完成）。
     *   注意：本函数不做任何"补全/润色/造文案"——服务端没给回答就是没给。
     */
    static func normalizeReply(_ any: Any?) -> String? {
        if let s = any as? String {
            // 纯空白按"无回答"处理（trim 后判空；非空则原样返回，不改写正文）。
            let t = s.trimmingCharacters(in: .whitespacesAndNewlines)
            return t.isEmpty ? nil : s
        }
        if let dict = any as? [String: Any],
           let s = dict["text"] as? String {
            let t = s.trimmingCharacters(in: .whitespacesAndNewlines)
            return t.isEmpty ? nil : s
        }
        return nil
    }

    /// 【对牛弹琴修复】harness UNKNOWN 回问若返回英文 needs-clarification（旧版云端模板），
    /// 自动中文化显示/朗读——用户说中文就绝不该看到英文套话。
    /// 匹配特征：I am not sure / not sure what you meant / be explicit（+ you said [ 前缀残留）。
    static func localizeReply(_ reply: String) -> String {
        let lower = reply.lowercased()
        let isEnglishClarif = lower.contains("not sure what you meant")
            || lower.contains("i am not sure")
            || lower.contains("be explicit")
            || (lower.contains("you said") && lower.contains("not sure"))
        guard isEnglishClarif else { return reply }
        return "我不太确定你想让我做什么。请说得更具体一点，比如「查一下今天的天气」「记个想法」「跑一下测试」——我会直接去办。"
    }

    // MARK: - 回执四行解析（contract.RenderReceipt 的反向解析）
    //
    /// server 渲染恰好四行：动作/文件/结果/撤销（兼容英文 Action/Files/Result/Undo）。
    /// 容错：行缺失 / 全半角冒号 / 多余行 / 前后空白 都不炸，按行首标签归位。
    static func parseReceipt(_ text: String?) -> Receipt {
        var out = Receipt()
        guard let text = text else { return out }
        // 匹配行首标签：动作/文件/结果/撤销（含英文 Action/Files/Result/Undo），后接全/半角冒号。
        // (?i) 忽略大小写：云端/本地 harness 语言输出不稳定（中英混合），双标签都认。
        let pattern = #"(?i)^\s*(动作|文件|结果|撤销|action|files|result|undo)\s*[:：]\s*(.*)$"#
        guard let regex = try? NSRegularExpression(pattern: pattern) else { return out }
        for raw in text.components(separatedBy: CharacterSet.newlines) {
            let ns = NSRange(raw.startIndex..., in: raw)
            guard let m = regex.firstMatch(in: raw, range: ns) else { continue }
            // NSRange → Swift Range<String.Index>，再用 String 下标。
            guard let labelRange = Range(m.range(at: 1), in: raw),
                  let valueRange = Range(m.range(at: 2), in: raw) else { continue }
            let label = String(raw[labelRange]).lowercased()
            let value = String(raw[valueRange]).trimmingCharacters(in: .whitespaces)
            switch label {
            case "动作", "action": out.action = value
            case "文件", "files": out.files = value
            case "结果", "result": out.result = value
            case "撤销", "undo": out.undo = value
            default: break
            }
        }
        return out
    }

    // MARK: - 撤销行解析（回执卡第 4 行 → 撤销按钮）
    //
    /**
     * 【伪代码逻辑层】（必写：撤销按钮显隐属裁决）
     *   show = server.reversible === true  且  undo 行不含"不可撤销/不可逆/禁止回滚"。
     *   backup：undo 行里提取 .bak 文件名（兼容 VHS_BACKUP_PATH: 前缀契约）。
     *   异常：undo 为空 → show=false，backup=''（不可撤销，不给按钮）。
     */
    static func extractUndo(_ receipt: Receipt, _ reversible: Bool?) -> UndoInfo {
        let undo = receipt.undo
        let irreversible = undo.range(of: "不可撤销|不可逆|禁止回滚", options: .regularExpression) != nil
        var info = UndoInfo()
        info.irreversible = irreversible
        info.show = (reversible == true) && !irreversible
        // 提取 .bak 文件名（兼容 VHS_BACKUP_PATH: 前缀）
        let bakPattern = #"(?:VHS_BACKUP_PATH\s*[:：]\s*)?([^\s，,]+\.bak)"#
        if let r = try? NSRegularExpression(pattern: bakPattern),
           let m = r.firstMatch(in: undo, range: NSRange(undo.startIndex..., in: undo)),
           let rr = Range(m.range(at: 1), in: undo) {
            info.backup = String(undo[rr])
        }
        return info
    }

    // MARK: - 意图关键词 → 轻标签（从 receipt 动作行压缩）

    private static let intentWords: [(NSRegularExpression, String)] = [
        (try! NSRegularExpression(pattern: "NOTE|记一下|笔记", options: .caseInsensitive), "笔记"),
        (try! NSRegularExpression(pattern: "EDIT|改文件|编辑|删除", options: .caseInsensitive), "改文件"),
        (try! NSRegularExpression(pattern: "QUERY|查代码|查询|看看", options: .caseInsensitive), "查询"),
        (try! NSRegularExpression(pattern: "COMMIT|提交|commit", options: .caseInsensitive), "提交"),
        (try! NSRegularExpression(pattern: "DEPLOY|部署", options: .caseInsensitive), "部署")
    ]

    static func intentBadge(_ actionText: String?) -> String {
        let text = actionText ?? ""
        for (re, label) in intentWords {
            if re.firstMatch(in: text, range: NSRange(text.startIndex..., in: text)) != nil {
                return label
            }
        }
        return ""
    }

    static func tone(forStatus s: String?) -> String {
        guard let s = s else { return "blue" }
        switch s {
        case "done": return "green"
        case "need_confirm": return "red"
        case "need_ask": return "amber"
        case "canceled", "interrupted": return "gray"
        default: return "blue"
        }
    }

    // MARK: - 轻标签压缩（意图/域/风险/状态）
    //
    /**
     * 【伪代码逻辑层】（必写：徽章从"状态/回执/归因"压缩，内部细节不放大）
     *   输入 view = GET /v1/tasks/{id} 的响应（不含完整 Outcome，只有 receipt/attribution/reversible）。
     *   输出 badges[] = 2~4 个小徽章：
     *     state  ← status 直接映射（执行中/待回问/待确认/完成/已取消/已中断）
     *     intent ← receipt.动作行关键词（笔记/改文件/查询/提交/部署）
     *     domain ← receipt.文件行压缩：notes.md→笔记域；有真实对象路径→项目域；无→省略
     *     risk   ← need_confirm→高风险·待放行；reversible→可逆；done 且 !reversible→不可逆
     *   原则：绝不展示置信度分数/ASR 原文/纠正明细——只留用户需要的掌控感。
     */
    static func compressBadges(_ view: TaskView) -> [Badge] {
        var badges: [Badge] = []
        let stateMap: [String: String] = [
            "running": "执行中", "need_ask": "待回问", "need_confirm": "待确认",
            "done": "完成", "canceled": "已取消", "interrupted": "已中断"
        ]
        if let st = view.status, let label = stateMap[st] {
            badges.append(Badge(kind: "state", label: label, tone: tone(forStatus: st)))
        }
        let r = parseReceipt(view.receipt)
        let it = intentBadge(r.action)
        if !it.isEmpty {
            badges.append(Badge(kind: "intent", label: it, tone: "blue"))
        }
        if r.files.range(of: "notes\\.md|笔记", options: .regularExpression) != nil
            || r.action.range(of: "笔记|NOTE", options: .regularExpression) != nil {
            badges.append(Badge(kind: "domain", label: "笔记域", tone: "gray"))
        } else if !r.files.isEmpty && r.files != "—" {
            badges.append(Badge(kind: "domain", label: "项目域", tone: "gray"))
        }
        if view.status == "need_confirm" {
            badges.append(Badge(kind: "risk", label: "高风险·待放行", tone: "red"))
        } else if view.reversible == true {
            badges.append(Badge(kind: "risk", label: "可逆", tone: "green"))
        } else if view.status == "done" && view.reversible != true {
            badges.append(Badge(kind: "risk", label: "不可逆", tone: "red"))
        }
        return badges
    }

    // MARK: - 一屏一个决策点（渲染路由）
    //
    /**
     * 【伪代码逻辑层】（必写：决策点优先级，一次只渲染一个）
     *   优先级 高→低：
     *     1. need_confirm → confirm        红色确认条（answer:"执行"）
     *     2. need_ask     → ask            候选按钮（点选 answer:option.id）
     *     3. canceled/interrupted → error  系统错误条
     *     4. done         → receipt        绿色回执卡（撤销按钮按 undo.show）
     *     5. running      → running        执行卡滚动步骤
     *     6. 其他/idle    → idle
     *   原则：上一个 done 的回执卡作为历史气泡留在对话流里，底部不再叠加第二个决策控件。
     */
    static func nextDecisionPoint(_ view: TaskView) -> DecisionPoint {
        switch view.status {
        case "need_confirm":
            return DecisionPoint(kind: .confirm,
                                 question: view.question ?? "人工放行（不可逆）操作")
        case "need_ask":
            return DecisionPoint(kind: .ask,
                                 question: view.question ?? "你想让我做什么？",
                                 options: view.options ?? [])
        case "canceled", "interrupted":
            let msg = view.error ?? (view.status == "interrupted"
                ? "任务已中断，请重新提交" : "任务被取消")
            return DecisionPoint(kind: .error, message: msg)
        case "done":
            let r = parseReceipt(view.receipt)
            return DecisionPoint(kind: .receipt,
                                 receipt: r,
                                 undo: extractUndo(r, view.reversible))
        case "running":
            return DecisionPoint(kind: .running)
        default:
            return DecisionPoint(kind: .idle)
        }
    }

    // MARK: - 角色映射（镜像 server.roleForStatus）
    //
    /**
     * 【伪代码逻辑层】（必写：阶段→角色裁决）
     *   planner  = 分类/域裁决/风险分级/确认闸/回问（决策）→ need_ask/need_confirm
     *   executor = 工具动作执行                              → running
     *   verifier = 校验/归因/回执                            → done
     *   其他/canceled/interrupted → 落回 planner。
     */
    static func roleForStatus(_ status: String?) -> String {
        guard let s = status else { return "planner" }
        switch s {
        case "need_ask", "need_confirm": return "planner"
        case "done": return "verifier"
        case "running": return "executor"
        default: return "planner"
        }
    }

    // MARK: - 执行卡高亮索引（P2，镜像 web advanceExec）
    //
    /**
     * 【伪代码逻辑层】（必写：执行卡按状态/阶段推进的高亮裁决）
     *   返回 (doneCount, activeIndex)：
     *     doneCount  = 已完成（打 ✓）的行数；activeIndex = 当前高亮行（-1 = 无高亮）。
     *   need_ask/need_confirm → 停在"确认闸"（第 4 行 index 3，此前 4 行打 ✓）。
     *   done                  → 全部行打 ✓。
     *   running               → (0,-1)：具体高亮由 SSE stage.step 事件驱动（见 advanceExec(toStage:)）。
     */
    static func execProgress(forStatus status: String?) -> (doneCount: Int, activeIndex: Int) {
        switch status {
        case "need_ask", "need_confirm": return (4, -1)
        case "done": return (execStages.count, -1)
        default: return (0, -1)
        }
    }

    /// stage.step 名 → 执行卡行索引（找不到返回 nil）。
    static func execIndex(ofStep step: String) -> Int? {
        execStages.firstIndex(of: step)
    }

    // MARK: - 语音一轮一清（P1）
    //
    /**
     * 【伪代码逻辑层】（必写：一轮一清）
     *   final 语音结果 = 整段替换输入框（绝不与旧文本拼接）；
     *   send 后清空识别缓冲，下一轮从空白开始。
     */
    static func voiceReplace(previous: String, final: String) -> String { final }

    // MARK: - 打断状态机：说"停" → 红色系统条
    //
    /**
     * 【伪代码逻辑层】（必写：停止→已生效/未执行/可继续或撤销）
     *   输入 view = 当前任务视图（可能 running/need_ask/need_confirm/done）。
     *   控制流：
     *     hasEffect = 已有 receipt 且动作/文件非空（done 前已落盘的执行结果）。
     *     active   = hasEffect ? ['已生效：<action>（<files>）']
     *                          : ['已生效：尚未产生文件变更']
     *     blocked  = ['未执行：后续阶段已中止']
     *     actions  = ['继续'] + (undo.show ? ['撤销'] : [])   // 撤销在最前
     *   异常：view 为空 → active='没有进行中的任务'，actions=[]。
     *   注意：系统条只是 UI 呈现；真正的停止由 AppModel 调 POST /v1/tasks/{id}/cancel 完成。
     */
    static func interruptSystemBar(_ view: TaskView?) -> SystemBarInfo {
        guard let view = view else {
            return SystemBarInfo(title: "停止", active: ["没有进行中的任务"],
                                 blocked: [], actions: [], closable: true)
        }
        // 完全空视图：无状态且无回执
        if view.status == nil && view.receipt == nil {
            return SystemBarInfo(title: "停止", active: ["没有进行中的任务"],
                                 blocked: [], actions: [], closable: true)
        }
        let r = parseReceipt(view.receipt)
        let hasEffect = (view.receipt != nil) && (!r.action.isEmpty || !r.files.isEmpty)
        let active: [String]
        if hasEffect {
            let f = r.files.isEmpty || r.files == "—" ? "" : "（\(r.files)）"
            active = ["已生效：\(r.action.isEmpty ? "执行已落盘" : r.action)\(f)"]
        } else {
            active = ["已生效：尚未产生文件变更"]
        }
        let blocked = ["未执行：后续阶段已中止"]
        var actions = ["继续"]
        let und = extractUndo(r, view.reversible)
        if und.show { actions.insert("撤销", at: 0) }
        return SystemBarInfo(title: "已按下停止", active: active,
                             blocked: blocked, actions: actions, closable: true)
    }

    // MARK: - 打断触发词判定（说"停"）

    /// 用户文本命中"停/停止/停下/stop"（大小写不敏感，trim 后全等）。
    static func isInterruptPhrase(_ text: String) -> Bool {
        let t = text.trimmingCharacters(in: .whitespaces).lowercased()
        return ["停", "停止", "停下", "stop"].contains(t)
    }

    // MARK: - v2.1 口答词表（I06 确认口答 / I17 撤销口答 / 先进理念6 噪声过滤 / I13 分险级）

    /// 极短噪声词：无意义哼哈（嗯/哦/好…）不提交、不生成气泡。
    /// 注意：确认口答在 AppModel.sendVoice 里先于本判断执行，"好/是/行"在决策点场景会被口答消费。
    static func isNoiseWord(_ t: String) -> Bool {
        let s = t.trimmingCharacters(in: .whitespacesAndNewlines)
        if s.count <= 1 { return true }
        let noise: Set<String> = ["嗯", "嗯嗯", "哦", "哦哦", "啊", "好的", "好", "OK", "ok", "Ok", "行", "哈", "诶", "哎", "呀", "对", "是", "明白", "知道了"]
        return noise.contains(s)
    }

    /// 确认口答肯定词（need_confirm 场景）。
    static func isAffirmPhrase(_ t: String) -> Bool {
        let s = t.trimmingCharacters(in: .whitespacesAndNewlines)
        let exact: Set<String> = ["执行", "确认", "可以", "好", "好的", "做", "做吧", "继续", "是", "对", "同意", "行", "就这么办", "就做"]
        return exact.contains(s) || s.hasPrefix("执行")
    }

    /// 确认口答否定词（need_confirm 场景）。
    static func isNegativePhrase(_ t: String) -> Bool {
        let s = t.trimmingCharacters(in: .whitespacesAndNewlines)
        let exact: Set<String> = ["取消", "拒绝", "不要", "不做", "不执行", "停", "停止", "算了", "不用", "别"]
        return exact.contains(s) || s.hasPrefix("不")
    }

    /// 撤销口答词（I17 / 先进理念2 语音撤销链）："撤销""撤销刚才那个""撤销上一条"。
    static func isUndoPhrase(_ t: String) -> Bool {
        let s = t.trimmingCharacters(in: .whitespacesAndNewlines)
        return s == "撤销" || s.contains("撤销")
    }

    /// 高风险动作词（I13 分险级：提交/推送/合并/部署/删除等，口答无效、强制按钮确认）。
    static func isHighRiskAction(_ probe: String) -> Bool {
        let a = probe.lowercased()
        let highRisk: [String] = ["提交", "推送", "push", "commit", "merge", "合并", "部署", "发布", "删除", "清空", "覆盖", "drop", "迁移", "rm"]
        return highRisk.contains { a.contains($0) }
    }

    /// V6.2 自动命名规则：取首条用户内容前 12 字 + "…"（本地兜底；云端可用后升级 AI 命名）。
    static func autoTitle(from text: String) -> String {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !t.isEmpty else { return "新会话" }
        let cleaned = t.replacingOccurrences(of: "\n", with: " ")
        let maxLen = 12
        if cleaned.count <= maxLen { return cleaned }
        return String(cleaned.prefix(maxLen)) + "…"
    }
}
