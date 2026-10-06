package server

// cloud_test.go —  end form P0   (R15a JWT / R16  user   / R17   ). 
//       (JWKS 200 path) useuser provide VHS_GOOGLE_CLIENT_ID afterconnect  OAuth   . 

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

// R15a: JWT  send / verify / edperiod /  modify. 
func TestCloudJWTSignAndVerify(t *testing.T) {
	dir := t.TempDir()
	cfg := testCloudCfg(t, dir)
	c := newCloudAuth(cfg)
	if err := c.ensureInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	// pos  send + verify
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
	//  modify payload ->     
	bad := tok[:len(tok)-3] + "abc"
	if _, err := c.verifyJWT(bad); err == nil {
		t.Fatal("篡改 token 应验签失败")
	}
	// edperiodreject
	expired, _ := signJWT(c.jwtKey, sessionClaims{Sub: "u", Exp: time.Now().Add(-time.Hour).Unix()})
	if _, err := c.verifyJWT(expired); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("过期 token 应拒绝，err=%v", err)
	}
	// signature    (use  secret)
	other, _ := signJWT([]byte("wrong-secret"), sessionClaims{Sub: "u", Exp: time.Now().Add(time.Hour).Unix()})
	if _, err := c.verifyJWT(other); err == nil {
		t.Fatal("错 secret 签名应拒绝")
	}
	t.Log("R15a JWT 正反用例 PASS")
}

// R16:   user word/   nameemptytime  . 
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
	//    userobj   
	if c.tenantPath("user-a") == c.tenantPath("user-b") {
		t.Fatal("租户目录应隔离")
	}
	//  word    : A write wordafter B obj  outnow
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
	// kind word asnew userdefault word(  e.g. ): B first     kind word
	if !containsStr(wb, "截个图") {
		t.Fatalf("新租户应获得种子词，wb=%v", wb)
	}
	//     : A  asbody edperiod userafter  to limit, B  accept  
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
	// B asnew user(body   periodinby prime, left=-1 i.e. limit ):    no 
	if _, err := c.checkAndConsume("user-b"); err != nil {
		t.Fatalf("B 不应受影响，err=%v", err)
	}
	t.Log("R16 租户隔离 PASS（目录/热词/配额独立）")
}

// R17:    limit 429 semantic + body     . 
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
	//    : 3    ,   4   limit
	for i := 0; i < 3; i++ {
		if _, err := c.checkAndConsume("free-user"); err != nil {
			t.Fatalf("前 3 条应放行: %v", err)
		}
	}
	if _, err := c.checkAndConsume("free-user"); err != quotaExceededErr {
		t.Fatalf("第 4 条应 quota_exceeded, got %v", err)
	}
	// prime   limit
	if _, err := c.checkAndConsume("prime-user"); err != nil {
		t.Fatalf("prime 首次应放行: %v", err)
	}
	// body   periodinby prime(new user trial_until   )
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
