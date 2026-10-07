//
//  Models.swift
//  VoxSign
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
    /// D0 冻结契约（Phase 1）：outcome.reply = 服务端给用户的「回答正文」。
    /// 已在网络层规范化为纯文本（字符串直取 / 对象取 .text）；缺失为 nil。
    /// done 时作为回答内容的**首选渲染源**（替代正则拆『结果：』），receipt 仅作回退。
    var reply: String?

    /// 容错构造：把任意字典合成为视图（测试与 SSE done 事件复用）。
    init(taskId: String? = nil,
         status: String? = nil,
         question: String? = nil,
         options: [TaskOption]? = nil,
         receipt: String? = nil,
         attribution: String? = nil,
         reversible: Bool? = nil,
         error: String? = nil,
         reply: String? = nil) {
        self.taskId = taskId
        self.status = status
        self.question = question
        self.options = options
        self.receipt = receipt
        self.attribution = attribution
        self.reversible = reversible
        self.error = error
        self.reply = reply
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
    /// v2.3（用户需求：微信式反馈"处理完了之后有多少时间"）：本轮处理耗时（秒），UI 显示"已处理 X 秒"。
    var elapsedSec: Double = 0
}

/// 撤销按钮裁决结果。
struct UndoInfo: Equatable {
    var show: Bool = false
    var backup: String = ""
    var irreversible: Bool = false
}

/// 轻标签徽章（kind: state/intent/domain/risk；tone: blue/green/red/amber/gray）。
/// v2.4：增加 Codable 遵循（会话历史持久化需要），字段与成员级初始化器保持不变。
struct Badge: Equatable, Codable {
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

// MARK: - v2.4 附件（资料）

/// 附件种类：文本粘贴 / URL / 图片（相册）/ 文件（Files App）。
enum AttachmentKind: String, Codable {
    case text
    case url
    case image
    case file
}

/// 一条随消息提交给 harness 上下文的资料附件。
struct Attachment: Identifiable, Codable, Equatable {
    var id: String            // UUID().uuidString
    var kind: AttachmentKind
    var title: String
    var text: String?         // text 类=正文；url 类=URL 字符串
    var fileName: String?     // file 类=文件名
    var localPath: String?    // image/file 类=本地路径（预览用）
}

// MARK: - v2.4 多会话持久化模型

/// 会话中一条可持久化消息（用户气泡 / harness 气泡 / 回执行）。
/// typing/execCard 中间态不持久化。
struct StoredMessage: Identifiable, Codable, Equatable {
    var id: String
    var role: String          // "user" | "harness"
    var text: String
    var badges: [Badge] = []
    var fromVoice: Bool = false
    var voiceSeconds: Int? = nil
    var attachments: [Attachment] = []
    var costTokens: Int? = nil
    var timestamp: Date = Date()
    var elapsedSec: Double? = nil   // 回执"已处理 X 秒"
}

/// 一个本地会话（多会话：默认隐藏，大部分时候是单对话流）。
struct ChatSession: Identifiable, Codable, Equatable {
    var id: String
    var title: String
    var createdAt: Date
    var updatedAt: Date
    var serverBase: String? = nil
    var messages: [StoredMessage] = []
    // V6.2 会话归属：每个会话挂在「角色」或「域」之一（nil=未分组，兼容旧数据）。
    var containerKind: ContainerKind? = nil
    var containerID: String? = nil
}

// MARK: - V6.2 角色（Role）/ 域（Domain）容器

/// 容器类型：角色（执行者身份）/ 域（话题·项目·空间）。
enum ContainerKind: String, Codable {
    case role, domain
}

/// 容器（文件夹）：角色或域。会话按 containerID 归入容器；容器可折叠、可内化归档。
struct ContainerItem: Identifiable, Codable, Equatable {
    var id: String
    var kind: ContainerKind
    var name: String
    var createdAt: Date = Date()
}

// MARK: - v2.4 顶栏状态点（纯函数，视图与测试共用）

/// 状态点色调。
enum DotTone {
    case blue    // 在线·任务执行中/空闲
    case red     // 离线（无条件）
    case gray    // 重连中 / 未知
    case orange  // 在线·等待用户确认/选择
}

/// 顶栏状态点裁决：连接态 × harness 态 → 色调。
enum TopBarDot {
    /// conn: ConnectivityService.ConnectionState；harness: AppModel.HarnessState。
    /// 规则：offline→.red（无条件）；online: decision→.orange、busy→.blue、idle→.blue；
    /// reconnecting/unknown→.gray。
    static func tone(conn: ConnectionState, harness: HarnessState) -> DotTone {
        switch conn {
        case .offline:
            return .red
        case .online:
            switch harness {
            case .decision: return .orange
            case .busy, .idle: return .blue
            }
        case .reconnecting, .unknown:
            return .gray
        }
    }
}

// MARK: - 消耗用量文案（纯函数，视图与测试共用）

/// 消耗用量文案：服务端不回传（nil）时整行不显示；回传时显示 "消耗 n"。
/// 服务端协议当前从不回传 token 用量字段，costTokens 恒为 nil；
/// 本纯函数把"隐式 if-let 容错"显式化，作为开源化协议解耦依据。
enum CostText {
    static func caption(for tokens: Int?) -> String? {
        guard let tokens else { return nil }
        return "消耗 \(tokens)"
    }
}
