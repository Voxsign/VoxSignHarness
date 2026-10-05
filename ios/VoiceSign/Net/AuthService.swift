//
//  AuthService.swift
//  VoiceSign
//
//  Google 登录（P1 云端模式）：OIDC Authorization Code + PKCE。
//  链路（对齐 docs/云端谷歌登录与发布架构-20261004.md §4）：
//    1. 生成 code_verifier + code_challenge(S256)
//    2. ASWebAuthenticationSession 打开 accounts.google.com 授权页
//    3. 用户授权 → 回调截获 ?code=
//    4. POST 云端 /v1/auth/google {code, code_verifier} → 会话 JWT
//  零第三方依赖：纯 AuthenticationServices + CryptoKit。
//

import Foundation
import AuthenticationServices
import CryptoKit
import UIKit

/// Google OAuth 参数（Web application 类型 client，与云端 harness 共用）。
enum GoogleOAuth {
    /// Google Cloud Console 创建（Web application）；公开安全，可内嵌。
    static var clientID: String {
        "914563065668-ap9ik9bs7r3pmo993ragoqktle92nvof.apps.googleusercontent.com"
    }
    /// 正式域名回调（Console 已注册）。公网必须 HTTPS（ATS）。
    static let productionRedirectURI = "https://voxsign.ai/auth/callback"
    /// 本地模拟回调（Console 已注册；iOS 15.5+ 支持 http://127.0.0.1）。
    static let localRedirectURI = "http://127.0.0.1"
    static let scope = "openid email profile"
    static let authorizationEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"
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

/// Google 登录编排：PKCE + ASWebAuthenticationSession + 服务端换码。
final class AuthService: NSObject {
    static let shared = AuthService()

    private var session: ASWebAuthenticationSession?

    /// 根据当前服务器地址选择回调：本地模拟（127.0.0.1/localhost）走 http://127.0.0.1，其余走正式域名。
    func redirectURI(for base: String) -> String {
        if base.contains("127.0.0.1") || base.contains("localhost") {
            return GoogleOAuth.localRedirectURI
        }
        return GoogleOAuth.productionRedirectURI
    }

    /// 发起 Google 授权，返回 authorization code（已带 PKCE verifier 供后续换码）。
    func authorize(base: String) async throws -> (code: String, verifier: String) {
        let verifier = Self.generateVerifier()
        let challenge = Self.s256Challenge(verifier)
        let redirect = redirectURI(for: base)
        guard var comps = URLComponents(string: GoogleOAuth.authorizationEndpoint) else {
            throw APIError.transport("OAuth 端点无效")
        }
        comps.queryItems = [
            URLQueryItem(name: "client_id", value: GoogleOAuth.clientID),
            URLQueryItem(name: "redirect_uri", value: redirect),
            URLQueryItem(name: "response_type", value: "code"),
            URLQueryItem(name: "scope", value: GoogleOAuth.scope),
            URLQueryItem(name: "code_challenge", value: challenge),
            URLQueryItem(name: "code_challenge_method", value: "S256"),
            URLQueryItem(name: "access_type", value: "offline"),
        ]
        guard let url = comps.url else {
            throw APIError.transport("OAuth URL 构造失败")
        }
        let scheme = redirect.hasPrefix("https") ? "https" : "http"
        let code = try await startSession(url: url, callbackScheme: scheme)
        return (code, verifier)
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
            auth?.start()
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
        // 顶层 window（设置页所在场景）；无窗口时回退 main window。
        let scene = UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .first { $0.activationState == .foregroundActive }
        return scene?.windows.first ?? UIApplication.shared.windows.first ?? ASPresentationAnchor()
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
