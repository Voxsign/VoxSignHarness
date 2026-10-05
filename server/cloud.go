package server

// cloud.go — 云端模式（VHS_MODE=cloud）：
// 谷歌登录（OIDC Authorization Code + PKCE；测试可传 id_token 直验）、
// 会话 JWT（HS256 自签短效）、租户命名空间隔离、三档配额。
// 设计：docs/云端谷歌登录与发布架构-20261004.md（ADR-001/002/004/006）。
//
// 零第三方依赖：JWT 手写（标准库 HMAC/base64url），Google id_token 用
// 本地缓存的 JWKS（RSA RS256）验签（crypto/rsa + x509），不引入外部库。

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"voicesign-harness/config"
)

// ---------- 会话 JWT（HS256 自签，零依赖） ----------

const jwtLeeway = 60 // 秒

type sessionClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Tier  string `json:"tier"`
	IAT   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
}

func b64u(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
func b64uDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// signJWT 签发 HS256 JWT（header.payload.sig）。
func signJWT(secret []byte, claims sessionClaims) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	seg := b64u(header) + "." + b64u(payload)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(seg))
	return seg + "." + b64u(mac.Sum(nil)), nil
}

// verifyJWT 校验签名 + 时效，返回 claims。
func verifyJWT(secret []byte, token string) (*sessionClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("jwt 段数错误")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := b64u(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return nil, fmt.Errorf("jwt 签名无效")
	}
	payload, err := b64uDecode(parts[1])
	if err != nil {
		return nil, err
	}
	var c sessionClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	if c.Exp < now-jwtLeeway {
		return nil, fmt.Errorf("jwt 已过期")
	}
	if c.Sub == "" {
		return nil, fmt.Errorf("jwt 缺 sub")
	}
	return &c, nil
}

// ---------- Google JWKS（RS256）验证 ----------

const googleCertsURL = "https://www.googleapis.com/oauth2/v3/certs"

type jwk struct {
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksDoc struct {
	Keys []jwk `json:"keys"`
}

// googleIDToken 是 Google id_token 的解码态（仅取验签所需字段 + 声明）。
type googleIDToken struct {
	Iss   string `json:"iss"`
	Aud   string `json:"aud"`
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
	Iat   int64  `json:"iat"`
}

// jwksCache 本地缓存 Google 公钥（5 分钟 TTL），避免每次登录都拉取。
type jwksCache struct {
	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey // kid → key
	fetched time.Time
}

func (c *jwksCache) get(kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.fetched) < 5*time.Minute {
		if k, ok := c.keys[kid]; ok {
			return k, nil
		}
	}
	doc, err := fetchJWKS()
	if err != nil {
		return nil, err
	}
	m := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nb, err := b64uDecode(k.N)
		if err != nil {
			continue
		}
		eb, err := b64uDecode(k.E)
		if err != nil {
			continue
		}
		e := 0
		for _, b := range eb {
			e = e<<8 | int(b)
		}
		m[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: e}
	}
	c.keys = m
	c.fetched = time.Now()
	k, ok := m[kid]
	if !ok {
		return nil, fmt.Errorf("JWKS 无此 kid: %s", kid)
	}
	return k, nil
}

func fetchJWKS() (*jwksDoc, error) {
	req, _ := http.NewRequest(http.MethodGet, googleCertsURL, nil)
	req.Header.Set("Accept", "application/json")
	cli := robustClient(tFast)
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS HTTP %d", resp.StatusCode)
	}
	var doc jwksDoc
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// acceptsAud 判断 id_token 的 aud 是否为受信 client：优先 VHS_GOOGLE_CLIENT_IDS（逗号分隔，
// 支持 iOS 类型 client 与 Web client 并存），未配置时退回单个 VHS_GOOGLE_CLIENT_ID。
func (c *cloudAuth) acceptsAud(aud string) bool {
	ids := c.cfg.Cloud.GoogleClientIDs
	if ids == "" {
		return aud == c.cfg.Cloud.GoogleClientID
	}
	for _, id := range strings.Split(ids, ",") {
		if aud == strings.TrimSpace(id) {
			return true
		}
	}
	return false
}

// verifyGoogleIDToken 校验 Google id_token：RS256 验签 + iss/aud/exp。
func (c *cloudAuth) verifyGoogleIDToken(idToken string) (*googleIDToken, error) {	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("id_token 段数错误")
	}
	// 头部取 kid
	hdr, err := b64uDecode(parts[0])
	if err != nil {
		return nil, err
	}
	var h struct {
		Kid string `json:"kid"`
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(hdr, &h); err != nil {
		return nil, err
	}
	if h.Alg != "RS256" {
		return nil, fmt.Errorf("仅支持 RS256，实际 %s", h.Alg)
	}
	pub, err := c.jwks.get(h.Kid)
	if err != nil {
		return nil, err
	}
	sig, err := b64uDecode(parts[2])
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hash[:], sig); err != nil {
		return nil, fmt.Errorf("id_token 验签失败")
	}
	payload, err := b64uDecode(parts[1])
	if err != nil {
		return nil, err
	}
	var tok googleIDToken
	if err := json.Unmarshal(payload, &tok); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	if tok.Iss != "accounts.google.com" && tok.Iss != "https://accounts.google.com" {
		return nil, fmt.Errorf("iss 非法: %s", tok.Iss)
	}
	if !c.acceptsAud(tok.Aud) {
		return nil, fmt.Errorf("aud 非法（非本应用签发的 token）: aud=%s", tok.Aud)
	}
	if tok.Exp < now {
		return nil, fmt.Errorf("id_token 已过期")
	}
	return &tok, nil
}

// ---------- 谷歌 code 交换（OAuth token 端点） ----------

// exchangeGoogleCode 用 authorization code + PKCE verifier 换 id_token。
func (c *cloudAuth) exchangeGoogleCode(code, verifier string) (*googleIDToken, error) {
	if c.cfg.Cloud.GoogleClientID == "" || c.cfg.Cloud.GoogleClientSecret == "" {
		return nil, fmt.Errorf("未配置 VHS_GOOGLE_CLIENT_ID / VHS_GOOGLE_CLIENT_SECRET")
	}
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", c.cfg.Cloud.GoogleClientID)
	form.Set("client_secret", c.cfg.Cloud.GoogleClientSecret)
	form.Set("redirect_uri", c.redirectURI())
	form.Set("grant_type", "authorization_code")
	if verifier != "" {
		form.Set("code_verifier", verifier)
	}
	req, _ := http.NewRequest(http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cli := robustClient(tMid)
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google token 交换 HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.IDToken == "" {
		return nil, fmt.Errorf("token 响应缺 id_token")
	}
	return c.verifyGoogleIDToken(out.IDToken)
}

func (c *cloudAuth) redirectURI() string {
	if c.cfg.Cloud.RedirectURI != "" {
		return c.cfg.Cloud.RedirectURI
	}
	return "https://voicesign.ai/auth/callback"
}

// ---------- 租户（命名空间 + 档位 + 配额） ----------

type tenantState struct {
	Sub        string `json:"sub"`
	Email      string `json:"email,omitempty"`
	Tier       string `json:"tier"`                  // free | prime | enterprise
	TrialUntil int64  `json:"trial_until,omitempty"` // 体验会员到期 unix；未过期按 prime 计
	CreatedAt  int64  `json:"created_at"`
}

type quotaState struct {
	Date  string `json:"date"` // YYYY-MM-DD（按服务器本地日）
	Tasks int    `json:"tasks"`
}

// effectiveTier 返回实际生效档位（体验会员期内按 prime）。
func (t *tenantState) effectiveTier() string {
	if t.Tier == "enterprise" {
		return "enterprise"
	}
	if t.Tier == "prime" {
		return "prime"
	}
	if t.TrialUntil > time.Now().Unix() {
		return "prime"
	}
	return "free"
}

// limitFor 免费档额度；prime/enterprise 不限额（返回 -1）。
func (c *cloudAuth) limitFor(effTier string) int {
	if effTier == "free" {
		n := c.cfg.Cloud.FreeDailyTasks
		if n <= 0 {
			n = 30
		}
		return n
	}
	return -1
}

// ---------- cloudAuth ----------

type cloudAuth struct {
	cfg      *config.Config
	jwtKey   []byte
	jwks     *jwksCache
	tenants  string // <log_dir>/tenants
	initOnce sync.Once
	initErr  error
}

func newCloudAuth(cfg *config.Config) *cloudAuth {
	c := &cloudAuth{cfg: cfg, jwks: &jwksCache{}}
	c.tenants = filepath.Join(cfg.Global.LogDir, "tenants")
	return c
}

// ensureInit 懒初始化：JWT secret（env 或持久化文件）、目录。
func (c *cloudAuth) ensureInit() error {
	c.initOnce.Do(func() {
		key := []byte(c.cfg.Cloud.JWTSecret)
		if len(key) == 0 {
			// 首次启动自动生成并持久化 <log_dir>/cloud/jwt-secret
			dir := filepath.Join(c.cfg.Global.LogDir, "cloud")
			secPath := filepath.Join(dir, "jwt-secret")
			if b, err := os.ReadFile(secPath); err == nil && len(b) >= 16 {
				key = b
			} else {
				buf := make([]byte, 32)
				if _, err := rand.Read(buf); err != nil {
					c.initErr = err
					return
				}
				key = buf
				if err := os.MkdirAll(dir, 0o755); err != nil {
					c.initErr = err
					return
				}
				if err := os.WriteFile(secPath, key, 0o600); err != nil {
					c.initErr = err
					return
				}
			}
		}
		c.jwtKey = key
		if err := os.MkdirAll(c.tenants, 0o755); err != nil {
			c.initErr = err
		}
	})
	return c.initErr
}

func (c *cloudAuth) tenantPath(sub string) string {
	return filepath.Join(c.tenants, sanitizeSub(sub))
}

// sanitizeSub 防目录穿越：租户 sub 只允许安全字符。
func sanitizeSub(sub string) string {
	var b strings.Builder
	for _, r := range sub {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	s := b.String()
	if s == "" {
		return "_anon"
	}
	return s
}

// loadTenant 读租户状态；不存在则创建（免费档 + 15 天体验会员）。
func (c *cloudAuth) loadTenant(sub, email string) (*tenantState, error) {
	if err := c.ensureInit(); err != nil {
		return nil, err
	}
	dir := c.tenantPath(sub)
	f := filepath.Join(dir, "tenant.json")
	if b, err := os.ReadFile(f); err == nil {
		var t tenantState
		if json.Unmarshal(b, &t) == nil && t.Sub == sub {
			return &t, nil
		}
	}
	t := &tenantState{
		Sub:        sub,
		Email:      email,
		Tier:       "free",
		TrialUntil: time.Now().Add(time.Duration(trialDays(c.cfg)) * 24 * time.Hour).Unix(),
		CreatedAt:  time.Now().Unix(),
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	b, _ := json.MarshalIndent(t, "", "  ")
	if err := os.WriteFile(f, b, 0o600); err != nil {
		return nil, err
	}
	return t, nil
}

func trialDays(cfg *config.Config) int {
	if cfg.Cloud.TrialDays > 0 {
		return cfg.Cloud.TrialDays
	}
	return 15
}

// quotaFile 配额计数路径（按租户 + 日期重置）。
func (c *cloudAuth) quotaFile(sub string) string {
	return filepath.Join(c.tenantPath(sub), "quota.json")
}

// readQuota 读当日配额（无则零值）。
func (c *cloudAuth) readQuota(sub string) (*quotaState, error) {
	today := time.Now().Format("2006-01-02")
	var q quotaState
	if b, err := os.ReadFile(c.quotaFile(sub)); err == nil {
		if json.Unmarshal(b, &q) == nil && q.Date == today {
			return &q, nil
		}
	}
	return &quotaState{Date: today}, nil
}

// consumeTask 原子计数 +1（临时文件 + rename，避免并发写坏）。
func (c *cloudAuth) consumeTask(sub string) (*quotaState, error) {
	q, err := c.readQuota(sub)
	if err != nil {
		return nil, err
	}
	q.Tasks++
	f := c.quotaFile(sub)
	tmp := f + ".tmp"
	b, _ := json.Marshal(q)
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, f); err != nil {
		return nil, err
	}
	return q, nil
}

// checkAndConsume 配额检查 + 消费：返回 (剩余额度, err)；超限 err=quotaExceeded。
// effTier 由调用方先取（effectiveTier）。
func (c *cloudAuth) checkAndConsume(sub string) (int, error) {
	t, err := c.loadTenant(sub, "")
	if err != nil {
		return 0, err
	}
	eff := t.effectiveTier()
	if c.limitFor(eff) < 0 {
		return -1, nil // prime/enterprise 不限额
	}
	q, err := c.consumeTask(sub)
	if err != nil {
		return 0, err
	}
	limit := c.limitFor("free")
	left := limit - q.Tasks
	if left < 0 {
		left = 0
	}
	if q.Tasks > limit {
		return left, quotaExceededErr
	}
	return left, nil
}

var quotaExceededErr = fmt.Errorf("quota_exceeded")

// verifyJWT 校验会话 JWT（方法包装，供 auth 中间件用）。
func (c *cloudAuth) verifyJWT(token string) (*sessionClaims, error) {
	if err := c.ensureInit(); err != nil {
		return nil, err
	}
	return verifyJWT(c.jwtKey, token)
}

// signSession 签发会话 JWT（10h 默认）。
func (c *cloudAuth) signSession(t *tenantState) (string, error) {
	if err := c.ensureInit(); err != nil {
		return "", err
	}
	ttl := c.cfg.Cloud.TokenTTLHours
	if ttl <= 0 {
		ttl = 10
	}
	now := time.Now().Unix()
	return signJWT(c.jwtKey, sessionClaims{
		Sub: t.Sub, Email: t.Email, Tier: t.effectiveTier(),
		IAT: now, Exp: now + int64(ttl)*3600,
	})
}

// ---------- HTTP 端点 ----------

// authLoginReq 登录请求：真实链路传 code(+verifier)；测试链路可直传 id_token。
type authLoginReq struct {
	Code         string `json:"code,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// handleAuthGoogle POST /v1/auth/google → 换/验 Google token → 租户 → 会话 JWT。
func (s *Server) handleAuthGoogle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "仅 POST"})
		return
	}
	var req authLoginReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体解析失败"})
		return
	}
	cloud := s.cloud
	if cloud == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "云端模式未启用（VHS_MODE=cloud）"})
		return
	}
	var gtok *googleIDToken
	var err error
	switch {
	case req.IDToken != "":
		gtok, err = cloud.verifyGoogleIDToken(req.IDToken)
	case req.Code != "":
		gtok, err = cloud.exchangeGoogleCode(req.Code, req.CodeVerifier)
	default:
		err = fmt.Errorf("需提供 code 或 id_token")
	}
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "谷歌登录失败: " + err.Error()})
		return
	}
	t, err := cloud.loadTenant(gtok.Sub, gtok.Email)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "租户初始化失败"})
		return
	}
	token, err := cloud.signSession(t)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "会话签发失败"})
		return
	}
	eff := t.effectiveTier()
	q, _ := cloud.readQuota(gtok.Sub)
	limit := cloud.limitFor(eff)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":  token,
		"tenant": gtok.Sub,
		"email":  gtok.Email,
		"tier":   eff,
		"quota": map[string]any{
			"used":      q.Tasks,
			"limit":     limit,
			"resets_at": q.Date,
		},
		"trial_until": t.TrialUntil,
	})
}

// handleMe GET /v1/me（cloud 模式，JWT 已由 auth 中间件解析注入 ctx）。
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	sub := ctxTenant(r.Context())
	if sub == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "无租户上下文"})
		return
	}
	cloud := s.cloud
	t, err := cloud.loadTenant(sub, "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "租户读取失败"})
		return
	}
	eff := t.effectiveTier()
	q, _ := cloud.readQuota(sub)
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant":      sub,
		"email":       t.Email,
		"tier":        eff,
		"trial_until": t.TrialUntil,
		"quota": map[string]any{
			"used":      q.Tasks,
			"limit":     cloud.limitFor(eff),
			"resets_at": q.Date,
		},
	})
}

// ---------- ctx 租户注入 ----------

type ctxKey int

const tenantCtxKey ctxKey = 0

func withTenant(ctx context.Context, sub string) context.Context {
	return context.WithValue(ctx, tenantCtxKey, sub)
}

func ctxTenant(ctx context.Context) string {
	v, _ := ctx.Value(tenantCtxKey).(string)
	return v
}

// 设备凭证（机器码）身份注入：设备 token 命中本机注册表后，把机器码放进上下文。
type machineCtxKey struct{}

func withMachine(ctx context.Context, code string) context.Context {
	return context.WithValue(ctx, machineCtxKey{}, code)
}

func ctxMachine(ctx context.Context) string {
	v, _ := ctx.Value(machineCtxKey{}).(string)
	return v
}

// truncate 日志脱敏用截断。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
