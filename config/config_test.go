package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadIsolated      ( time HOME nouseuser  file)under Load. 
func loadIsolated(t *testing.T, path string) (Config, error) {
	t.Helper()
	t.Setenv("VHS_CONFIG", path)
	t.Setenv("VHS_API_KEY", "")
	t.Setenv("VHS_PROVIDER", "")
	t.Setenv("VHS_ROUTE", "")
	t.Setenv("VHS_LOG_DIR", "")
	t.Setenv("VHS_ADDR", "")
	t.Setenv("VHS_MAX_TURNS", "")
	t.Setenv("VHS_ALLOW_HIGH_RISK", "")
	t.Setenv("VHS_INTENT_CONF", "")
	t.Setenv("VHS_LOW_CONF_ACTION", "")
	return Load(path)
}

func TestDefaultSane(t *testing.T) {
	c := Default()
	if len(c.Providers) < 5 || c.Global.MaxTurnsDefault != 2 || c.Input.IntentConf != 0.6 {
		t.Fatalf("default not sane: %+v", c)
	}
	if !c.IsMockProvider("mock") {
		t.Fatal("mock provider kind 应为 mock")
	}
	if _, ok := c.ProviderByName("center"); !ok {
		t.Fatal("默认 providers 应含 center")
	}
	if c.DictionaryPath() == "" {
		t.Fatal("DictionaryPath 不应为空")
	}
}

func TestLoadMissingExplicitPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")
	if _, err := loadIsolated(t, missing); err == nil {
		t.Fatal("显式指定但文件不存在应报错")
	}
}

func TestLoadFileMergeDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "harness.json")
	content := `{
		"global": {"max_turns_default": 3},
		"providers": [{"name":"mock","kind":"mock"}],
		"routes": [{"name":"default","provider":"mock","default":true}],
		"input": {"intent_conf": 0.7},
		"memory": {"dir": "` + filepath.ToSlash(dir) + `/mem"}
	}`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := loadIsolated(t, p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Global.MaxTurnsDefault != 3 {
		t.Fatalf("file value should win over default: %d", c.Global.MaxTurnsDefault)
	}
	if c.Input.IntentConf != 0.7 {
		t.Fatalf("intent_conf: %v", c.Input.IntentConf)
	}
	if c.Global.LLMTimeoutMs != 60000 {
		t.Fatalf("absent field should keep default: %d", c.Global.LLMTimeoutMs)
	}
	if c.Memory.Dir != filepath.Join(dir, "mem") {
		t.Fatalf("memory.dir: %s", c.Memory.Dir)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	p := filepath.Join(t.TempDir(), "harness.json")
	if err := os.WriteFile(p, []byte(`{"providers":[{"name":"mock","kind":"mock"}],"routes":[{"name":"default","provider":"mock","default":true}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VHS_CONFIG", p)
	t.Setenv("VHS_MAX_TURNS", "4")
	t.Setenv("VHS_INTENT_CONF", "0.9")
	t.Setenv("VHS_ROUTE", "myroute")
	t.Setenv("VHS_PROVIDER", "myp")
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Global.MaxTurnsDefault != 4 || c.Input.IntentConf != 0.9 {
		t.Fatalf("env should win: %+v %+v", c.Global, c.Input)
	}
	if c.ForcedRoute != "myroute" || c.ForcedProvider != "myp" {
		t.Fatalf("forced env: %+v", c)
	}
}

func TestLoadAPIKeyEnvOverrides(t *testing.T) {
	p := filepath.Join(t.TempDir(), "harness.json")
	if err := os.WriteFile(p, []byte(`{"providers":[{"name":"a","kind":"openai","endpoint":"https://x","model":"m","api_key":"filekey"},{"name":"mock","kind":"mock"}],"routes":[{"name":"default","provider":"a","default":true}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VHS_CONFIG", p)
	t.Setenv("VHS_API_KEY", "envkey")
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a, _ := c.ProviderByName("a")
	if a.APIKey != "envkey" {
		t.Fatalf("VHS_API_KEY 应覆盖文件 key: %q", a.APIKey)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{"duplicate provider", `{"providers":[{"name":"a","kind":"mock"},{"name":"a","kind":"mock"}],"routes":[{"name":"default","provider":"a","default":true}]}`, "duplicate"},
		{"未知 kind", `{"providers":[{"name":"a","kind":"weird"}],"routes":[{"name":"default","provider":"a","default":true}]}`, "unsupported kind"},
		{"openai 缺 endpoint", `{"providers":[{"name":"a","kind":"openai","model":"m"}],"routes":[{"name":"default","provider":"a","default":true}]}`, "endpoint"},
		{"路由引用未知 provider", `{"providers":[{"name":"mock","kind":"mock"}],"routes":[{"name":"default","provider":"ghost","default":true}]}`, "not defined in providers table"},
		{"无 default 路由", `{"providers":[{"name":"mock","kind":"mock"}],"routes":[{"name":"a","provider":"mock"}]}`, "default"},
	}
	for _, tc := range cases {
		p := filepath.Join(t.TempDir(), "harness.json")
		if err := os.WriteFile(p, []byte(tc.json), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := loadIsolated(t, p)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: want error containing %q, got %v", tc.name, tc.want, err)
		}
	}
}

func TestEffectiveHelpers(t *testing.T) {
	c := Default()
	rt := Route{Provider: LocalProvider}
	if c.EffectiveMaxTurns(rt) != 0 {
		t.Fatal("local 应 0 turns")
	}
	rt2 := Route{Provider: "center", MaxTurns: 0}
	if c.EffectiveMaxTurns(rt2) != 2 {
		t.Fatal("default max_turns 应为 2")
	}
	p, _ := c.ProviderByName("center")
	if !c.EffectiveResponseFormat(p) {
		t.Fatal("response_format 默认 true")
	}
	if c.EffectiveTimeoutMs(p) != 60000 {
		t.Fatal("timeout 默认 60000")
	}
	if !strings.Contains(c.DictionaryPath(), "dictionary.json") {
		t.Fatalf("dictionary path: %s", c.DictionaryPath())
	}
}

// TestDerivedDirsFollowLogDirEnv(M3 config bug back ): VHS_LOG_DIR overwriteafter, 
//    occurobj (memory/spaces/contracts/cache)      log_dir under, 
//     referto default ~/.voicesign/harness/*. 
func TestDerivedDirsFollowLogDirEnv(t *testing.T) {
	sandbox := t.TempDir()
	t.Setenv("VHS_LOG_DIR", sandbox)
	t.Setenv("VHS_CONFIG", "")
	t.Setenv("VHS_API_KEY", "")
	t.Setenv("VHS_PROVIDER", "")
	t.Setenv("VHS_ROUTE", "")
	t.Setenv("VHS_ADDR", "")
	t.Setenv("VHS_TOKEN", "")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Global.LogDir != sandbox {
		t.Fatalf("LogDir 应被 env 覆盖为 %q, got %q", sandbox, cfg.Global.LogDir)
	}
	for name, d := range map[string]string{
		"memory": cfg.Memory.Dir, "spaces": cfg.Spaces.Dir,
		"contracts": cfg.Contracts.Dir, "cache": cfg.Cache.Dir,
	} {
		if d == "" {
			t.Fatalf("%s.Dir 不应为空", name)
		}
		if !strings.HasPrefix(d, sandbox) {
			t.Fatalf("%s.Dir 必须在 %q 下, got %q（旧默认未跟随）", name, sandbox, d)
		}
		if strings.Contains(d, ".voicesign") {
			t.Fatalf("%s.Dir 绝不能回退到 ~/.voicesign: %q", name, d)
		}
	}
}

// TestDerivedDirsDefaultWhenNoEnv: no env time occurobj   default ~/.voicesign/harness/*. 
func TestDerivedDirsDefaultWhenNoEnv(t *testing.T) {
	t.Setenv("VHS_LOG_DIR", "")
	t.Setenv("VHS_CONFIG", "")
	t.Setenv("VHS_API_KEY", "")
	t.Setenv("VHS_PROVIDER", "")
	t.Setenv("VHS_ROUTE", "")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".voicesign", "harness")
	if !strings.HasPrefix(cfg.Global.LogDir, want) {
		t.Fatalf("无 env 默认 LogDir 应含 %q, got %q", want, cfg.Global.LogDir)
	}
	for name, d := range map[string]string{
		"memory": cfg.Memory.Dir, "spaces": cfg.Spaces.Dir,
		"contracts": cfg.Contracts.Dir, "cache": cfg.Cache.Dir,
	} {
		if !strings.HasPrefix(d, want) {
			t.Fatalf("无 env 时 %s.Dir 应在 %q 下, got %q", name, want, d)
		}
	}
}
