package server

// cloud_test.go — 云端模式 P0 验证（R15a JWT / R16 租户隔离 / R17 配额）。
// 真实谷歌登录（JWKS 200 路径）待用户提供 VHS_GOOGLE_CLIENT_ID 后接真 OAuth 验证。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"voicesign-harness/config"
)

func testCloudCfg(t *testing.T, logDir string) *config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Global.LogDir = logDir
	cfg.Global.CloudMode = true
	cfg.Cloud.JWTSecret = "test-secret-0123456789abcdef"
	cfg.Cloud.FreeDailyTasks = 3
	cfg.Cloud.TrialDays = 15
	cfg.Cloud.TokenTTLHours = 10
	return &cfg
}

// R15a：JWT 签发 / 校验 / 过期 / 篡改。
func TestCloudJWTSignAndVerify(t *testing.T) {
	dir := t.TempDir()
	cfg := testCloudCfg(t, dir)
	c := newCloudAuth(cfg)
	if err := c.ensureInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	// 正常签发 + 校验
	claims := sessionClaims{Sub: "user-a", Email: "a@x.com", Tier: "free",
		IAT: time.Now().Unix(), Exp: time.Now().Add(time.Hour).Unix()}
	tok, err := signJWT(c.jwtKey, claims)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := c.verifyJWT(tok)
	if err != nil || got.Sub != "user-a" || got.Tier != "free" {
		t.Fatalf("verify: got=%+v err=%v", got, err)
	}
	// 篡改 payload → 验签失败
	bad := tok[:len(tok)-3] + "abc"
	if _, err := c.verifyJWT(bad); err == nil {
		t.Fatal("篡改 token 应验签失败")
	}
	// 过期拒绝
	expired, _ := signJWT(c.jwtKey, sessionClaims{Sub: "u", Exp: time.Now().Add(-time.Hour).Unix()})
	if _, err := c.verifyJWT(expired); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("过期 token 应拒绝，err=%v", err)
	}
	// 签名不可伪造（用错 secret）
	other, _ := signJWT([]byte("wrong-secret"), sessionClaims{Sub: "u", Exp: time.Now().Add(time.Hour).Unix()})
	if _, err := c.verifyJWT(other); err == nil {
		t.Fatal("错 secret 签名应拒绝")
	}
	t.Log("R15a JWT 正反用例 PASS")
}

// R16：双租户热词/配额命名空间隔离。
func TestCloudTenantIsolation(t *testing.T) {
	dir := t.TempDir()
	cfg := testCloudCfg(t, dir)
	c := newCloudAuth(cfg)
	if err := c.ensureInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	ta, err := c.loadTenant("user-a", "a@x.com")
	if err != nil {
		t.Fatalf("tenant a: %v", err)
	}
	tb, err := c.loadTenant("user-b", "b@x.com")
	if err != nil {
		t.Fatalf("tenant b: %v", err)
	}
	if ta.Sub != "user-a" || tb.Sub != "user-b" {
		t.Fatalf("租户身份错位")
	}
	// 各自租户目录独立
	if c.tenantPath("user-a") == c.tenantPath("user-b") {
		t.Fatal("租户目录应隔离")
	}
	// 热词互不影响：A 写热词后 B 目录不出现
	aDir := c.tenantPath("user-a")
	if err := os.MkdirAll(filepath.Join(aDir, "personal"), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := []string{"九万年就准", "帮我把PPT发群里"}
	b, _ := json.Marshal(custom)
	if err := os.WriteFile(filepath.Join(aDir, "personal", "hotwords.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	wa := loadASRHotwords(aDir)
	wb := loadASRHotwords(c.tenantPath("user-b"))
	if !containsStr(wa, "九万年就准") || containsStr(wb, "九万年就准") {
		t.Fatalf("热词隔离失败 wa=%v wb=%v", wa, wb)
	}
	// 种子词作为新租户默认热词（设计如此）：B 首次加载应含种子词
	if !containsStr(wb, "季总") {
		t.Fatalf("新租户应获得种子词，wb=%v", wb)
	}
	// 配额独立：A 设为体验过期租户后消费到超限，B 不受影响
	ta.TrialUntil = time.Now().Add(-time.Hour).Unix()
	ab, _ := json.Marshal(ta)
	if err := os.WriteFile(filepath.Join(c.tenantPath("user-a"), "tenant.json"), ab, 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < cfg.Cloud.FreeDailyTasks+1; i++ {
		if _, err := c.checkAndConsume("user-a"); err != nil && err != quotaExceededErr {
			t.Fatalf("consume a: %v", err)
		}
	}
	if _, err := c.checkAndConsume("user-a"); err != quotaExceededErr {
		t.Fatalf("A 应超限，err=%v", err)
	}
	// B 为新租户（体验会员期内按 prime，left=-1 即不限额）：消费应无错
	if _, err := c.checkAndConsume("user-b"); err != nil {
		t.Fatalf("B 不应受影响，err=%v", err)
	}
	t.Log("R16 租户隔离 PASS（目录/热词/配额独立）")
}

// R17：免费超限 429 语义 + 体验会员放行。
func TestCloudQuotaTiers(t *testing.T) {
	dir := t.TempDir()
	cfg := testCloudCfg(t, dir)
	c := newCloudAuth(cfg)
	_ = c.ensureInit()
	tn, err := c.loadTenant("free-user", "")
	if err != nil {
		t.Fatalf("租户创建失败: %v", err)
	}
	tn.TrialUntil = time.Now().Add(-time.Hour).Unix()
	fs, _ := json.Marshal(tn)
	if err := os.WriteFile(filepath.Join(c.tenantPath("free-user"), "tenant.json"), fs, 0o644); err != nil {
		t.Fatal(err)
	}
	// 免费档：3 条额度，第 4 条超限
	for i := 0; i < 3; i++ {
		if _, err := c.checkAndConsume("free-user"); err != nil {
			t.Fatalf("前 3 条应放行: %v", err)
		}
	}
	if _, err := c.checkAndConsume("free-user"); err != quotaExceededErr {
		t.Fatalf("第 4 条应 quota_exceeded, got %v", err)
	}
	// prime 档不限
	if _, err := c.checkAndConsume("prime-user"); err != nil {
		t.Fatalf("prime 首次应放行: %v", err)
	}
	// 体验会员期内按 prime（新租户 trial_until 未来）
	tr, _ := c.loadTenant("trial-user", "t@x.com")
	if tr.effectiveTier() != "prime" {
		t.Fatalf("体验会员期应视为 prime, got %s", tr.effectiveTier())
	}
	if _, err := c.checkAndConsume("trial-user"); err != nil {
		t.Fatalf("体验会员应不限量: %v", err)
	}
	t.Log("R17 配额三档 PASS（free 超限 / prime 不限 / 体验会员按 prime）")
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
