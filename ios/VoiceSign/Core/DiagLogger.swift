//
//  DiagLogger.swift
//  VoiceSign
//
//  App 端结构化诊断日志：OSLog（subsystem com.voicesign）+ 本地文件
//  Caches/VoxSign/diag-<日期>.log（可导出）。覆盖：请求/轮询/SSE/状态转换/UI 渲染分支决策。
//  零第三方依赖。
//

import Foundation
import os.log

final class DiagLogger {
    static let shared = DiagLogger()

    private let log = OSLog(subsystem: "com.voicesign", category: "voxsign")
    private let queue = DispatchQueue(label: "com.voicesign.diag")
    private var fileHandle: FileHandle?
    private let dateFmt: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm:ss.SSS"
        return f
    }()

    private init() {
        queue.async { self.openFile() }
    }

    private func openFile() {
        let fm = FileManager.default
        guard let caches = try? fm.url(for: .cachesDirectory, in: .userDomainMask,
                                       appropriateFor: nil, create: true) else { return }
        let dir = caches.appendingPathComponent("VoxSign", isDirectory: true)
        try? fm.createDirectory(at: dir, withIntermediateDirectories: true)
        let day = ISO8601DateFormatter().string(from: Date()).prefix(10)
        let file = dir.appendingPathComponent("diag-\(day).log")
        if !fm.fileExists(atPath: file.path) {
            fm.createFile(atPath: file.path, contents: nil)
        }
        fileHandle = try? FileHandle(forWritingTo: file)
        try? fileHandle?.seekToEnd()
    }

    /// 打一条：同时进 OSLog 与本地文件。tag 如 "POLL"/"SSE"/"ROUTE"/"UI"/"NET"。
    func log(_ tag: String, _ message: String) {
        let line = "[\(dateFmt.string(from: Date()))] [\(tag)] \(message)\n"
        os_log("%{public}@", log: log, type: .info, line)
        queue.async { [weak self] in
            try? self?.fileHandle?.write(contentsOf: Data(line.utf8))
        }
    }
}
