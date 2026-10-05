//
//  SSEClient.swift
//  VoxSign
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
        // 修复卡死根源（T2）：waitsForConnectivity=true 时，网络不可达会让
        // URLSession 无限期等待连接建立 —— 手机连不上 server 时 UI 就"卡死"。
        // 改走显式竞速超时（connectTimeout），连不上 10s 内抛错给上层重连决策。
        cfg.waitsForConnectivity = false
        // 60s 仅作 idle 超时（流上长时间无任何字节），SSE keepalive 足够。
        cfg.timeoutIntervalForRequest = 60
        return URLSession(configuration: cfg)
    }()

    /// 建立连接的最长等待（秒）：TCP/HTTP 握手阶段竞速超时，超时按"连接失败"处理。
    /// 上层（AppModel.openSSE）会按退避策略重连，不会无限挂起。
    private let connectTimeout: TimeInterval = 10

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
                    // T2 连接竞速超时：URLSession 的 timeoutIntervalForRequest 只覆盖
                    // idle 超时，不覆盖"连不上"——用 withThrowingTaskGroup 加一个
                    // connectTimeout 的哨兵任务，谁先失败/完成即裁决。
                    let (bytes, resp): (URLSession.AsyncBytes, URLResponse) = try await withThrowingTaskGroup(of: (URLSession.AsyncBytes, URLResponse).self) { group in
                        group.addTask {
                            let (b, r) = try await self.session.bytes(for: req)
                            return (b, r)
                        }
                        group.addTask {
                            try await Task.sleep(nanoseconds: UInt64(self.connectTimeout * 1_000_000_000))
                            throw APIError.transport("SSE 连接超时（\(Int(self.connectTimeout))s，服务不可达？）")
                        }
                        let first = try await group.next()!
                        group.cancelAll()
                        return first
                    }
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
