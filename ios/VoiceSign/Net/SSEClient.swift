//
//  SSEClient.swift
//  VoiceSign
//
//  SSE 事件流客户端（GET /v1/tasks/{id}/events，Bearer 认证）。
//  用 URLSession.bytes(for:) 逐行读流，喂入 SSEParser；断线按 ?after=lastSeq 重连。
//  连接保持到任务终态（done/failed/canceled/interrupted）后关闭。
//

import Foundation

/// 把 SSE 字节流转为 AsyncThrowingStream<SSEEvent>。
final class SSEClient {
    static let shared = SSEClient()
    var settings: SettingsStore = .shared

    private let session: URLSession = {
        let cfg = URLSessionConfiguration.ephemeral
        cfg.timeoutIntervalForRequest = 60
        cfg.waitsForConnectivity = true
        return URLSession(configuration: cfg)
    }()

    /// 订阅某任务的事件流。
    /// - Parameter taskId: 任务 id
    /// - Parameter after: 断线重连时的 lastSeq（首次为 0 = 从首事件起）
    /// - Returns: 事件 AsyncThrowingStream；收到终态事件后自然结束。
    func events(taskId: String, after: Int = 0) -> AsyncThrowingStream<SSEEvent, Error> {
        AsyncThrowingStream { [weak self] continuation in
            guard let self = self else {
                continuation.finish()
                return
            }
            let basePath = "/v1/tasks/\(taskId)/events"
            guard let baseURL = self.settings.url(basePath) else {
                continuation.finish(throwing: APIError.transport("server 地址无效"))
                return
            }
            let url = after > 0 ? SSEParser.reconnectURL(base: baseURL, after: after) : baseURL
            var req = URLRequest(url: url)
            req.setValue("text/event-stream", forHTTPHeaderField: "Accept")
            if !self.settings.token.isEmpty {
                req.setValue("Bearer \(self.settings.token)", forHTTPHeaderField: "Authorization")
            }
            // SSE 规范：断线重连也可用 Last-Event-ID 头（这里走 ?after=query，与契约双兼容）。

            let parser = SSEParser()
            let task = Task {
                do {
                    let (bytes, resp) = try await self.session.bytes(for: req)
                    guard let http = resp as? HTTPURLResponse, (200...299).contains(http.statusCode) else {
                        continuation.finish(throwing: APIError.http((resp as? HTTPURLResponse)?.statusCode ?? 0,
                                                                   "SSE 连接失败"))
                        return
                    }
                    for try await line in bytes.lines {
                        // bytes.lines 给的是单行；SSE 事件以空行分隔，
                        // 我们把每行（含末尾换行）喂回 parser 以正确分帧。
                        let events = parser.feed(line + "\n")
                        for ev in events {
                            continuation.yield(ev)
                            if ev.isTerminal {
                                continuation.finish()
                                return
                            }
                        }
                    }
                    continuation.finish()
                } catch {
                    continuation.finish(throwing: error)
                }
            }
            continuation.onTermination = { _ in
                task.cancel()
            }
        }
    }
}
