//
//  SSEParser.swift
//  VoiceSign
//
//  SSE 事件流解析（纯逻辑，无网络）——逐字对齐《SSE-v1-事件流契约.md》。
//  - 标准 SSE 分帧：事件以空行分隔；`event:` 给类型（缺省 message）；`data:` 多行以 \n 拼接。
//  - data 内必含 `seq`（单调递增，同任务内不重复，从 1 起）；缺字段按缺省处理不崩溃。
//  - 断线重连：客户端保存 lastSeq，重连请求带 `?after=<lastSeq>`，server 重放 seq > after。
//

import Foundation

/// 归一化后的 SSE 事件类型。
enum SSEEvent: Equatable {
    case stage(seq: Int, role: String?, phase: String?, step: String?)
    case ask(seq: Int, question: String, options: [TaskOption])
    case confirm(seq: Int, question: String)
    case done(seq: Int, receipt: String?, attribution: String?, reversible: Bool?, role: String?)
    case failed(seq: Int, error: String?)
    case interrupt(seq: Int, applied: [String], notApplied: [String], canRollback: Bool)
    case canceled(seq: Int)
    case unknown(type: String, seq: Int?)

    /// 事件的 seq（unknown 可能缺）。
    var seq: Int? {
        switch self {
        case .stage(let s, _, _, _), .ask(let s, _, _), .confirm(let s, _),
             .done(let s, _, _, _, _), .failed(let s, _), .interrupt(let s, _, _, _),
             .canceled(let s):
            return s
        case .unknown(_, let s):
            return s
        }
    }

    /// 是否为终态事件（连接应关闭）：done/failed/canceled。
    var isTerminal: Bool {
        switch self {
        case .done, .failed, .canceled: return true
        default: return false
        }
    }
}

/// 把 raw data JSON 字典按事件类型解码为强类型 SSEEvent。
/// data 字段缺失时消费者按缺省处理，不得因缺字段崩溃。
enum SSEDecoder {

    private static let jsonDecoder: JSONDecoder = {
        let d = JSONDecoder()
        d.keyDecodingStrategy = .useDefaultKeys
        return d
    }()

    static func decode(type rawType: String, data raw: String) -> SSEEvent {
        let type = rawType.isEmpty ? "message" : rawType
        // data 可能是多行拼接的 JSON 字符串。
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let data = trimmed.data(using: .utf8),
              let dict = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] else {
            return .unknown(type: type, seq: nil)
        }
        let seq = (dict["seq"] as? NSNumber)?.intValue ?? (dict["seq"] as? Int) ?? -1
        switch type {
        case "stage":
            return .stage(seq: seq,
                          role: dict["role"] as? String,
                          phase: dict["phase"] as? String,
                          step: dict["step"] as? String)
        case "need_ask":
            let options = decodeOptions(dict["options"])
            return .ask(seq: seq,
                        question: dict["question"] as? String ?? "",
                        options: options)
        case "need_confirm":
            return .confirm(seq: seq, question: dict["question"] as? String ?? "")
        case "done":
            return .done(seq: seq,
                         receipt: dict["receipt"] as? String,
                         attribution: dict["attribution"] as? String,
                         reversible: dict["reversible"] as? Bool,
                         role: dict["role"] as? String)
        case "failed":
            return .failed(seq: seq, error: dict["error"] as? String)
        case "interrupt":
            return .interrupt(seq: seq,
                             applied: toStringArray(dict["applied"]),
                             notApplied: toStringArray(dict["notApplied"]),
                             canRollback: (dict["canRollback"] as? Bool) ?? false)
        case "canceled":
            return .canceled(seq: seq)
        default:
            return .unknown(type: type, seq: seq < 0 ? nil : seq)
        }
    }

    private static func decodeOptions(_ any: Any?) -> [TaskOption] {
        guard let arr = any as? [[String: Any]] else { return [] }
        return arr.compactMap { o in
            guard let id = o["id"] as? String, let label = o["label"] as? String else { return nil }
            return TaskOption(id: id, label: label)
        }
    }

    private static func toStringArray(_ any: Any?) -> [String] {
        guard let arr = any as? [Any] else { return [] }
        return arr.compactMap { $0 as? String }
    }
}

/// 流式 SSE 帧解析器：喂入任意字节段（chunks），吐已凑完整的事件。
/**
 * 【伪代码逻辑层】（必写：SSE 状态机 / lastSeq / 重连）
 *   feed(chunk):
 *     buffer += chunk
 *     while buffer 含 "\n\n"（或 "\r\n\r\n"）:
 *        取一个 block（到首个空行为止）
 *        解析 block 内多行:
 *          event: <type>        → eventType（缺省 "message"）
 *          data:  <line>        → dataLines += line（多行 data 用 \n 拼接）
 *        event = SSEDecoder.decode(eventType, dataLines.joined("\n"))
 *        lastSeq = max(lastSeq, event.seq)
 *        yield(event)
 *   断线重连：用 lastSeq 拼 URL?after=<lastSeq>；server 重放 seq > after，不重复不丢。
 *   异常：半截事件（buffer 末尾无空行）留在 buffer 等下一段；JSON 解析失败 → unknown 不炸。
 */
final class SSEParser {
    private var buffer = ""
    private(set) var lastSeq: Int = 0

    init() {}

    /// 喂入一段文本，返回所有在本段内凑完整的事件。
    func feed(_ text: String) -> [SSEEvent] {
        buffer += text
        var events: [SSEEvent] = []
        // 反复切出以空行（\n\n 或 \r\n\r\n）结尾的 block。
        while let range = buffer.range(of: "\n\n") ?? buffer.range(of: "\r\n\r\n") {
            let block = String(buffer[buffer.startIndex..<range.lowerBound])
            buffer.removeSubrange(buffer.startIndex..<range.upperBound)
            if let ev = parseBlock(block) {
                if let s = ev.seq, s > lastSeq { lastSeq = s }
                events.append(ev)
            }
        }
        return events
    }

    /// 把一个完整 block 解析成事件；空 block 返回 nil。
    private func parseBlock(_ block: String) -> SSEEvent? {
        var eventType = ""
        var dataLines: [String] = []
        // 行分隔兼容 \n 与 \r\n
        for line in block.components(separatedBy: "\n") {
            let trimmedCR = line.hasSuffix("\r") ? String(line.dropLast()) : line
            if trimmedCR.hasPrefix("event:") {
                eventType = String(trimmedCR.dropFirst("event:".count))
                    .trimmingCharacters(in: .whitespaces)
            } else if trimmedCR.hasPrefix("data:") {
                var d = String(trimmedCR.dropFirst("data:".count))
                // SSE 规范：冒号后若有一个空格，剥掉。
                if d.hasPrefix(" ") { d.removeFirst() }
                dataLines.append(d)
            }
            // 其他行（id:/retry:/注释:）忽略——契约只用 event/data。
        }
        if dataLines.isEmpty && eventType.isEmpty { return nil }
        return SSEDecoder.decode(type: eventType, data: dataLines.joined(separator: "\n"))
    }

    /// 重连 URL：在事件路径后追加 ?after=<lastSeq>（契约：或标准 Last-Event-ID 头）。
    static func reconnectURL(base: URL, after lastSeq: Int) -> URL {
        if var comps = URLComponents(url: base, resolvingAgainstBaseURL: false) {
            var items = comps.queryItems ?? []
            items = items.filter { $0.name != "after" }
            items.append(URLQueryItem(name: "after", value: String(lastSeq)))
            comps.queryItems = items
            return comps.url ?? base
        }
        return base
    }
}
