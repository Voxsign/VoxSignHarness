//
//  APIClient.swift
//  VoiceSign
//
//  INTERACT-v1 端点集对接（只读契约，不改 server）。全来源带 Bearer 认证。
//  零第三方依赖：纯 URLSession + async/await。
//

import Foundation

enum APIError: LocalizedError, Equatable {
    case http(Int, String)
    case decode(String)
    case transport(String)

    var errorDescription: String? {
        switch self {
        case .http(let code, let body): return "HTTP \(code)：\(body)"
        case .decode(let m): return "解析失败：\(m)"
        case .transport(let m): return "网络错误：\(m)"
        }
    }
}

/// POST /v1/tasks 的返回。
struct CreateTaskResponse: Equatable {
    let taskId: String
    let status: String?
    let deduped: Bool?
}

/// GET /v1/status 的返回。
struct StatusResponse: Equatable {
    let ok: Bool
    let version: String?
    let tasks: Int?
}

/// GET /v1/roles 的返回。
struct RolesResponse: Equatable {
    let roles: [RoleInfo]
    let taskId: String?
    let role: String?
}

final class APIClient {
    static let shared = APIClient()
    var settings: SettingsStore = .shared

    private let session: URLSession = {
        let cfg = URLSessionConfiguration.ephemeral
        cfg.timeoutIntervalForRequest = 15
        return URLSession(configuration: cfg)
    }()

    // MARK: - 通用请求

    private func request(_ method: String, _ path: String, body: [String: Any]? = nil) async throws -> (Int, Data) {
        guard let url = settings.url(path) else {
            DiagLogger.shared.log("NET", "\(method) \(path) 失败：server 地址无效 base=\(settings.base)")
            throw APIError.transport("server 地址无效（检查设置页）")
        }
        var req = URLRequest(url: url)
        req.httpMethod = method
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if !settings.token.isEmpty {
            req.setValue("Bearer \(settings.token)", forHTTPHeaderField: "Authorization")
        }
        DiagLogger.shared.log("NET", "\(method) \(url.absoluteString) token=\(settings.token.isEmpty ? "无" : "有(\(settings.token))")")
        if let body = body {
            req.httpBody = try? JSONSerialization.data(withJSONObject: body)
        }
        do {
            let (data, resp) = try await session.data(for: req)
            guard let http = resp as? HTTPURLResponse else {
                throw APIError.transport("无 HTTP 响应")
            }
            DiagLogger.shared.log("NET", "\(method) \(path) → \(http.statusCode) bytes=\(data.count)")
            return (http.statusCode, data)
        } catch let e as APIError {
            DiagLogger.shared.log("NET", "\(method) \(path) 抛错：\(e.localizedDescription)")
            throw e
        } catch {
            DiagLogger.shared.log("NET", "\(method) \(path) 传输失败：\(error.localizedDescription)")
            throw APIError.transport(error.localizedDescription)
        }
    }

    private func decodeJSON(_ data: Data) -> [String: Any] {
        return (try? JSONSerialization.jsonObject(with: data) as? [String: Any]) ?? [:]
    }

    // MARK: - 端点

    /// POST /v1/tasks {text, space?, request_id?} → 202 {task_id,status}；同 request_id → 200 deduped。
    func submitTask(text: String, space: String? = nil, requestId: String) async throws -> CreateTaskResponse {
        var body: [String: Any] = ["text": text, "request_id": requestId]
        if let space = space { body["space"] = space }
        let (code, data) = try await request("POST", "/v1/tasks", body: body)
        let j = decodeJSON(data)
        guard (200...299).contains(code), let tid = j["task_id"] as? String else {
            throw APIError.http(code, String(decoding: data, as: UTF8.self))
        }
        return CreateTaskResponse(taskId: tid,
                                  status: j["status"] as? String,
                                  deduped: j["deduped"] as? Bool)
    }

    /// GET /v1/tasks/{id} → 轮询视图。
    func fetchTask(_ id: String) async throws -> TaskView {
        let (code, data) = try await request("GET", "/v1/tasks/\(id)")
        let j = decodeJSON(data)
        if let st = j["status"] as? String, st == "done" || st == "need_ask" {
            DiagLogger.shared.log("POLL", "原始body[\(st)]: \(String(decoding: data, as: UTF8.self).prefix(300))")
        }
        guard (200...299).contains(code) else {
            throw APIError.http(code, String(decoding: data, as: UTF8.self))
        }
        let options = (j["options"] as? [[String: Any]])?.compactMap { o -> TaskOption? in
            guard let i = o["id"] as? String, let l = o["label"] as? String else { return nil }
            return TaskOption(id: i, label: l)
        }
        return TaskView(taskId: id,
                        status: j["status"] as? String,
                        question: j["question"] as? String,
                        options: options,
                        receipt: j["receipt"] as? String,
                        attribution: j["attribution"] as? String,
                        reversible: j["reversible"] as? Bool,
                        error: j["error"] as? String)
    }

    /// POST /v1/tasks/{id}/answer {answer}（候选 id 或 "执行"）。409 = 当前无待回答决策点。
    func answer(_ id: String, _ ans: String) async throws {
        let (code, data) = try await request("POST", "/v1/tasks/\(id)/answer", body: ["answer": ans])
        guard (200...299).contains(code) else {
            throw APIError.http(code, String(decoding: data, as: UTF8.self))
        }
    }

    /// POST /v1/tasks/{id}/rollback → {ok, restored}；不可逆 409。
    @discardableResult
    func rollback(_ id: String) async throws -> String? {
        let (code, data) = try await request("POST", "/v1/tasks/\(id)/rollback")
        let j = decodeJSON(data)
        guard (200...299).contains(code) else {
            throw APIError.http(code, String(decoding: data, as: UTF8.self))
        }
        return j["restored"] as? String
    }

    /// POST /v1/tasks/{id}/cancel（M6 新端点集；legacy /v1/cancel body{task_id} 兼容）。
    func cancel(_ id: String) async throws {
        // 优先新端点；失败再回退 legacy（契约：legacy /v1/cancel 兼容）。
        do {
            let (code, _) = try await request("POST", "/v1/tasks/\(id)/cancel")
            if (200...299).contains(code) { return }
        } catch let e as APIError {
            // 落到 legacy
            if case .http(let c, _) = e, c == 404 || c == 405 {
                let _ = try await request("POST", "/v1/cancel", body: ["task_id": id])
                return
            }
            throw e
        }
    }

    /// GET /v1/status（设置页"测试连接"）。
    func status() async throws -> StatusResponse {
        let (code, data) = try await request("GET", "/v1/status")
        let j = decodeJSON(data)
        guard (200...299).contains(code) else {
            throw APIError.http(code, String(decoding: data, as: UTF8.self))
        }
        return StatusResponse(ok: j["ok"] as? Bool ?? false,
                              version: j["version"] as? String,
                              tasks: j["tasks"] as? Int)
    }

    // MARK: - Google 登录（云端模式）

    /// POST /v1/auth/google {id_token} → 会话 JWT + 租户/档位/配额（iOS 类型 client 链路：
    /// iOS 已用 PKCE 换好 Google id_token，云端 harness JWKS 验签后签发会话 JWT）。
    func loginGoogleIDToken(_ idToken: String) async throws -> GoogleLoginResult {
        let (code2, data) = try await request("POST", "/v1/auth/google",
                                              body: ["id_token": idToken])
        let j = decodeJSON(data)
        guard (200...299).contains(code2), let token = j["token"] as? String else {
            throw APIError.http(code2, String(decoding: data, as: UTF8.self))
        }
        let quota = (j["quota"] as? [String: Any]).map { q in
            GoogleLoginResult.GoogleQuota(used: q["used"] as? Int,
                                         limit: q["limit"] as? Int,
                                         resetsAt: q["resets_at"] as? String)
        }
        return GoogleLoginResult(token: token,
                                 tenant: j["tenant"] as? String ?? "",
                                 email: j["email"] as? String ?? "",
                                 tier: j["tier"] as? String ?? "",
                                 trialUntil: j["trial_until"] as? String,
                                 quota: quota)
    }

    /// POST /v1/auth/google {code, code_verifier}（保留：Web client 本地模拟链路用）。
    func loginGoogle(code: String, verifier: String) async throws -> GoogleLoginResult {
        let (code2, data) = try await request("POST", "/v1/auth/google",
                                              body: ["code": code, "code_verifier": verifier])
        let j = decodeJSON(data)
        guard (200...299).contains(code2), let token = j["token"] as? String else {
            throw APIError.http(code2, String(decoding: data, as: UTF8.self))
        }
        let quota = (j["quota"] as? [String: Any]).map { q in
            GoogleLoginResult.GoogleQuota(used: q["used"] as? Int,
                                         limit: q["limit"] as? Int,
                                         resetsAt: q["resets_at"] as? String)
        }
        return GoogleLoginResult(token: token,
                                 tenant: j["tenant"] as? String ?? "",
                                 email: j["email"] as? String ?? "",
                                 tier: j["tier"] as? String ?? "",
                                 trialUntil: j["trial_until"] as? String,
                                 quota: quota)
    }

    /// GET /v1/me → 登录态刷新（档位/额度/试用期）。
    func me() async throws -> MeResult {
        let (code, data) = try await request("GET", "/v1/me")
        let j = decodeJSON(data)
        guard (200...299).contains(code) else {
            throw APIError.http(code, String(decoding: data, as: UTF8.self))
        }
        let quota = (j["quota"] as? [String: Any]).map { q in
            GoogleLoginResult.GoogleQuota(used: q["used"] as? Int,
                                         limit: q["limit"] as? Int,
                                         resetsAt: q["resets_at"] as? String)
        }
        return MeResult(tenant: j["tenant"] as? String ?? "",
                        email: j["email"] as? String ?? "",
                        tier: j["tier"] as? String ?? "",
                        trialUntil: j["trial_until"] as? String,
                        quota: quota)
    }

    /// GET /v1/roles → 多角色折叠条数据（active 态）。
    func roles() async throws -> RolesResponse {
        let (code, data) = try await request("GET", "/v1/roles")
        let j = decodeJSON(data)
        guard (200...299).contains(code) else {
            throw APIError.http(code, String(decoding: data, as: UTF8.self))
        }
        let rs = (j["roles"] as? [[String: Any]])?.map { o -> RoleInfo in
            RoleInfo(id: o["id"] as? String ?? "",
                     label: o["label"] as? String ?? "",
                     active: o["active"] as? Bool ?? false)
        } ?? []
        return RolesResponse(roles: rs,
                             taskId: j["task_id"] as? String,
                             role: j["role"] as? String)
    }
}
