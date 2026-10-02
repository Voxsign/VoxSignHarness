//
//  Models.swift
//  VoiceSign
//
//  Core value types shared by the pure logic layer and the network layer.
//  Zero third-party dependencies (Foundation only).
//

import Foundation

/// GET /v1/tasks/{id} 的响应视图（对齐 INTERACT-v1；字段缺失时按缺省处理，不得因缺字段崩溃）。
struct TaskView: Equatable {
    var taskId: String?
    var status: String?          // running/need_ask/need_confirm/done/canceled/interrupted
    var question: String?
    var options: [TaskOption]?
    var receipt: String?         // 四行文本字符串
    var attribution: String?
    var reversible: Bool?
    var error: String?

    /// 容错构造：把任意字典合成为视图（测试与 SSE done 事件复用）。
    init(taskId: String? = nil,
         status: String? = nil,
         question: String? = nil,
         options: [TaskOption]? = nil,
         receipt: String? = nil,
         attribution: String? = nil,
         reversible: Bool? = nil,
         error: String? = nil) {
        self.taskId = taskId
        self.status = status
        self.question = question
        self.options = options
        self.receipt = receipt
        self.attribution = attribution
        self.reversible = reversible
        self.error = error
    }
}

/// need_ask 候选按钮：{id,label}。
struct TaskOption: Equatable, Codable {
    let id: String
    let label: String
}

/// 回执四行解析结果（动作/文件/结果/撤销）。
struct Receipt: Equatable {
    var action: String = ""
    var files: String = ""
    var result: String = ""
    var undo: String = ""
}

/// 撤销按钮裁决结果。
struct UndoInfo: Equatable {
    var show: Bool = false
    var backup: String = ""
    var irreversible: Bool = false
}

/// 轻标签徽章（kind: state/intent/domain/risk；tone: blue/green/red/amber/gray）。
struct Badge: Equatable {
    let kind: String
    let label: String
    let tone: String
}

/// 一屏一个决策点。
enum DecisionKind: String, Equatable {
    case confirm     // 红色强确认条（answer:"执行"）
    case ask         // 回问候选按钮（answer:option.id）
    case error       // canceled/interrupted 系统错误条
    case receipt     // 绿色回执卡
    case running     // 执行卡滚动
    case idle
}

struct DecisionPoint: Equatable {
    var kind: DecisionKind
    var question: String = ""
    var options: [TaskOption] = []
    var message: String = ""
    var receipt: Receipt = Receipt()
    var undo: UndoInfo = UndoInfo()
}

/// 打断红色系统条三语义（已生效/未执行/可动作）。
struct SystemBarInfo: Equatable {
    var title: String = ""
    var active: [String] = []
    var blocked: [String] = []
    var actions: [String] = []
    var closable: Bool = true
}

/// 角色（planner/executor/verifier）。
struct RoleInfo: Equatable {
    let id: String
    let label: String
    var active: Bool
}
