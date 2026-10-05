//
//  AuthService.swift
//  VoiceSign
//
//  Google 登录（云端模式）：OIDC Authorization Code + PKCE，iOS 类型 OAuth Client。
//  链路（对齐 docs/云端谷歌登录与发布架构-20261004.md §4，修订为 iOS client）：
//    1. 生成 code_verifier + code_challenge(S256)
//    2. ASWebAuthenticationSession 打开 accounts.google.com 授权页
//       （redirect_uri = com.googleusercontent.apps.<client-id>:// 自定义 scheme，
//        iOS 100% 可靠截获——Web client 的 http://127.0.0.1 回调节截获在真机不可靠）
//    3. 用户授权 → 回调截获 ?code=
//    4. iOS 公开客户端 + PKCE 换 id_token（无需 client_secret，iOS 零机密）
//    5. POST 云端 /v1/auth/google {id_token} → 服务端 JWKS RS256 验签 → 租户 → 会话 JWT
//  零第三方依赖：纯 AuthenticationServices + CryptoKit。
//

import Foundation
import AuthenticationServices
import CryptoKit
import UIKit

/// Google OAuth 参数（iOS 类型 client；公开安全，可内嵌）。
enum GoogleOAuth {
    /// Google Cloud Console 创建（iOS 类型，Bundle ID ai.voxsign.ios）。
    static var clientID: String {
        "914563065668-50u3b19qin911rqg1p2msrg661n8v4pp.apps.googleusercontent.com"
    }
    /// iOS 类型 client 的授权回调：反向域名 scheme。
    /// 注意：Console 注册的是**不含** `.apps.googleusercontent.com` 后缀的部分（已验证：带后缀 → redirect_uri_mismatch）。
    private static var schemePrefix: String {
        "com.googleusercontent.apps.\(clientID.replacingOccurrences(of: ".apps.googleusercontent.com", with: ""))"
    }
    static var redirectURI: String { "\(schemePrefix)://" }
    /// ASWebAuthenticationSession 的 callbackURLScheme（去掉 ://）。
    static var callbackScheme: String { schemePrefix }
    static let scope = "openid email profile"
    static let authorizationEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"
    static let tokenEndpoint = "https://oauth2.googleapis.com/token"
}

/// POST /v1/auth/google 的返回（会话 JWT + 租户信息）。
struct GoogleLoginResult: Decodable, Equatable {
    let token: String
    let tenant: String
    let email: String
    let tier: String
    let trialUntil: String?
    let quota: GoogleQuota?

    struct GoogleQuota: Decodable, Equatable {
        let used: Int?
        let limit: Int?
        let resetsAt: String?
        enum CodingKeys: String, CodingKey {
            case used, limit
            case resetsAt = "resets_at"
        }
    }

    enum CodingKeys: String, CodingKey {
        case token, tenant, email, tier, quota
        case trialUntil = "trial_until"
    }
}

/// GET /v1/me 的返回（登录态刷新）。
struct MeResult: Decodable, Equatable {
    let tenant: String
    let email: String
    let tier: String
    let trialUntil: String?
    let quota: GoogleLoginResult.GoogleQuota?
    enum CodingKeys: String, CodingKey {
        case tenant, email, tier, quota
        case trialUntil = "trial_until"
    }
}

/// Google 登录编排：PKCE + ASWebAuthenticationSession + iOS 换 id_token + 云端验签。
final class AuthService: NSObject {
    static let shared = AuthService()

    private var session: ASWebAuthenticationSession?

    /// 发起 Google 授权，返回 authorization code + PKCE verifier。
    func authorize(base: String) async throws -> (code: String, verifier: String) {
        let verifier = Self.generateVerifier()
        let challenge = Self.s256Challenge(verifier)
        guard var comps = URLComponents(string: GoogleOAuth.authorizationEndpoint) else {
            throw APIError.transport("OAuth 端点无效")
        }
        comps.queryItems = [
            URLQueryItem(name: "client_id", value: GoogleOAuth.clientID),
            URLQueryItem(name: "redirect_uri", value: GoogleOAuth.redirectURI),
            URLQueryItem(name: "response_type", value: "code"),
            URLQueryItem(name: "scope", value: GoogleOAuth.scope),
            URLQueryItem(name: "code_challenge", value: challenge),
            URLQueryItem(name: "code_challenge_method", value: "S256"),
            URLQueryItem(name: "access_type", value: "offline"),
        ]
        guard let url = comps.url else {
            throw APIError.transport("OAuth URL 构造失败")
        }
        let code = try await startSession(url: url, callbackScheme: GoogleOAuth.callbackScheme)
        return (code, verifier)
    }

    /// iOS 公开客户端 + PKCE 换 id_token（无需 client_secret；token 不经云端，iOS 零机密）。
    func exchangeIDToken(code: String, verifier: String) async throws -> String {
        var comps = URLComponents()
        comps.queryItems = [
            URLQueryItem(name: "grant_type", value: "authorization_code"),
            URLQueryItem(name: "code", value: code),
            URLQueryItem(name: "client_id", value: GoogleOAuth.clientID),
            URLQueryItem(name: "redirect_uri", value: GoogleOAuth.redirectURI),
            URLQueryItem(name: "code_verifier", value: verifier),
        ]
        guard let endpoint = URL(string: GoogleOAuth.tokenEndpoint),
              let body = comps.percentEncodedQuery else {
            throw APIError.transport("Google token 端点无效")
        }
        var req = URLRequest(url: endpoint)
        req.httpMethod = "POST"
        req.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        req.httpBody = body.data(using: .utf8)
        let (data, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse else {
            throw APIError.transport("Google token 交换无响应")
        }
        guard (200...299).contains(http.statusCode),
              let j = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let idToken = j["id_token"] as? String, !idToken.isEmpty else {
            let body = String(decoding: data, as: UTF8.self)
            throw APIError.http(http.statusCode, body)
        }
        return idToken
    }

    // MARK: - ASWebAuthenticationSession

    @MainActor
    private func startSession(url: URL, callbackScheme: String) async throws -> String {
        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<String, Error>) in
            var auth: ASWebAuthenticationSession?
            let completion: ASWebAuthenticationSession.CompletionHandler = { callbackURL, error in
                defer { self.session = nil }
                if let error = error {
                    cont.resume(throwing: Self.mapAuthError(error))
                    return
                }
                guard let callbackURL = callbackURL,
                      let comps = URLComponents(url: callbackURL, resolvingAgainstBaseURL: false),
                      let code = comps.queryItems?.first(where: { $0.name == "code" })?.value,
                      !code.isEmpty else {
                    cont.resume(throwing: APIError.transport("未从 Google 回调取到 code（用户可能取消）"))
                    return
                }
                cont.resume(returning: code)
            }
            auth = ASWebAuthenticationSession(url: url, callbackURLScheme: callbackScheme, completionHandler: completion)
            self.session = auth
            auth?.presentationContextProvider = self
            auth?.prefersEphemeralWebBrowserSession = false
            // start() 返回 false 表示弹窗失败（如无可呈现的 window），此时 completion 不会被调用——
            // 必须立即抛错，否则 continuation 永不恢复、用户无任何反馈。
            if auth?.start() != true {
                auth = nil
                self.session = nil
                cont.resume(throwing: APIError.transport("无法弹出 Google 授权窗口，请重试"))
            }
        }
    }

    private static func mapAuthError(_ error: Error) -> Error {
        if let e = error as? ASWebAuthenticationSessionError {
            switch e.code {
            case .canceledLogin:
                return APIError.transport("登录已取消")
            default:
                return APIError.transport("Google 登录失败：\(e.localizedDescription)")
            }
        }
        return APIError.transport("Google 登录失败：\(error.localizedDescription)")
    }

    // MARK: - PKCE（RFC 7636）

    /// 生成 32 字节随机 verifier → base64url（43 字符）。
    static func generateVerifier() -> String {
        var bytes = [UInt8](repeating: 0, count: 32)
        _ = SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes)
        return Data(bytes).base64URLEncodedString()
    }

    /// SHA-256(verifier) → base64url。
    static func s256Challenge(_ verifier: String) -> String {
        let digest = SHA256.hash(data: Data(verifier.utf8))
        return Data(digest).base64URLEncodedString()
    }
}

extension AuthService: ASWebAuthenticationPresentationContextProviding {
    @MainActor
    func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor {
        // 优先取前台 scene 的 keyWindow；无 keyWindow 时取第一个 window。
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        for scene in scenes where scene.activationState == .foregroundActive {
            if let key = scene.windows.first(where: { $0.isKeyWindow }) ?? scene.windows.first {
                return key
            }
        }
        if let w = scenes.first?.windows.first { return w }
        return ASPresentationAnchor()
    }
}

extension Data {
    /// base64url（RFC 4648 §5，无 padding）。
    func base64URLEncodedString() -> String {
        var s = base64EncodedString()
        s = s.replacingOccurrences(of: "+", with: "-")
        s = s.replacingOccurrences(of: "/", with: "_")
        s = s.replacingOccurrences(of: "=", with: "")
        return s
    }
}
