// config_test.go —— 模型中心配置与通道的默认门禁（无网络）。
package modelcenter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func goodConfig() Config {
	return Config{
		ContractVersion: "1",
		Gateway:         GatewayConfig{BaseURL: "https://aiops.example", ChatPath: "/api/model/chat", APIKeyEnv: "VHS_TEST_KEY"},
		Channels: map[string]ChannelConfig{
			"default":  {Enabled: true, Provider: "aiops", ModelID: "deepseek-flash", TimeoutMS: 5000, MaxConcurrency: 4, WriteBack: false},
			"diagnose": {Enabled: false, Provider: "aiops", ModelID: "TBD", TimeoutMS: 5000, MaxConcurrency: 2, WriteBack: false},
			"learn":    {Enabled: false, Provider: "aiops", ModelID: "TBD", TimeoutMS: 5000, MaxConcurrency: 1, WriteBack: true},
		},
	}
}

func TestValidateAcceptsTemplate(t *testing.T) {
	if err := goodConfig().Validate(); err != nil {
		t.Fatalf("合法配置被拒: %v", err)
	}
}

func TestValidateC1ChannelSetFixed(t *testing.T) {
	c := goodConfig()
	delete(c.Channels, "diagnose")
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "C1") {
		t.Fatalf("C1 未生效: %v", err)
	}
	c = goodConfig()
	c.Channels["extra"] = ChannelConfig{}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "C1") {
		t.Fatalf("C1 未拦住多余通道: %v", err)
	}
}

func TestValidateC2OnlyLearnWritesBack(t *testing.T) {
	c := goodConfig()
	d := c.Channels["default"]
	d.WriteBack = true
	c.Channels["default"] = d
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "C2") {
		t.Fatalf("C2 未拦住 default 写回: %v", err)
	}
	c = goodConfig()
	l := c.Channels["learn"]
	l.WriteBack = false
	c.Channels["learn"] = l
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "C2") {
		t.Fatalf("C2 未要求 learn 写回: %v", err)
	}
}

func TestValidateC3EnabledNeedsModelID(t *testing.T) {
	c := goodConfig()
	d := c.Channels["diagnose"]
	d.Enabled = true // model_id 仍是 TBD
	c.Channels["diagnose"] = d
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "C3") {
		t.Fatalf("C3 未拦住 TBD 的启用通道: %v", err)
	}
}

func TestValidateC5LearnSingleWriter(t *testing.T) {
	c := goodConfig()
	l := c.Channels["learn"]
	l.MaxConcurrency = 3
	c.Channels["learn"] = l
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "C5") {
		t.Fatalf("C5 未拦住多写者: %v", err)
	}
}

func TestLoadRejectsYAMLAndBadJSON(t *testing.T) {
	dir := t.TempDir()
	yaml := filepath.Join(dir, "model-center.yaml")
	if err := os.WriteFile(yaml, []byte("channels: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(yaml); err == nil || !strings.Contains(err.Error(), "C4") {
		t.Fatalf("C4 未拒绝 YAML: %v", err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Fatal("坏 JSON 未报错")
	}
}

func TestLoadRealTemplate(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "config", "model-center.json"))
	if err != nil {
		t.Fatalf("模板加载失败: %v", err)
	}
	if !cfg.Channels["default"].Enabled || cfg.Channels["default"].ModelID != "deepseek-flash" {
		t.Errorf("default 通道配置不符: %+v", cfg.Channels["default"])
	}
	if cfg.Channels["diagnose"].Enabled || cfg.Channels["learn"].Enabled {
		t.Error("diagnose/learn 在模型未定前必须 enabled:false")
	}
	if cfg.Gateway.ChatPath != "/api/model/chat" {
		t.Errorf("chat_path 必须是实测路径: %q", cfg.Gateway.ChatPath)
	}
}

func TestNewRegistryRequiresEnvKey(t *testing.T) {
	t.Setenv("VHS_TEST_KEY", "")
	if _, err := NewRegistry(goodConfig()); err == nil || !strings.Contains(err.Error(), "VHS_TEST_KEY") {
		t.Fatalf("缺 key 未 fail-closed: %v", err)
	}
}

func TestRegistryOnlyEnablesConfiguredChannels(t *testing.T) {
	t.Setenv("VHS_TEST_KEY", "dummy-not-a-real-key")
	r, err := NewRegistry(goodConfig())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Enabled(ChannelDefault) {
		t.Error("default 应启用")
	}
	if r.Enabled(ChannelDiagnose) || r.Enabled(ChannelLearn) {
		t.Error("未配置的通道不得启用")
	}
	if _, err := r.Invoke(context.Background(), ChannelDiagnose, "hi"); err == nil {
		t.Error("未启用通道被调用却未报错")
	}
	if _, ok := r.WriteBackToken(ChannelLearn); ok {
		t.Error("learn 未启用却签发了写回令牌（L2）")
	}
	if _, ok := r.WriteBackToken(ChannelDefault); ok {
		t.Error("default 永不签发写回令牌（L2）")
	}
}

// TestInvokeUsesMeasuredPath：请求必须打到实测路径，鉴权头正确，响应按 OpenAI 格式解析。
func TestInvokeUsesMeasuredPath(t *testing.T) {
	var gotPath, gotAuth, gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			Model string `json:"model"`
		}
		_ = decodeJSON(r, &req)
		gotModel = req.Model
		fmt.Fprint(w, `{"object":"chat.completion","model":"deepseek-flash","choices":[{"message":{"role":"assistant","content":"收到"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
	}))
	defer srv.Close()

	cfg := goodConfig()
	cfg.Gateway.BaseURL = srv.URL
	t.Setenv("VHS_TEST_KEY", "dummy-key")
	r, err := NewRegistry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.Invoke(context.Background(), ChannelDefault, "只回复两个字：收到")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/model/chat" {
		t.Errorf("请求路径 %q，期望 /api/model/chat（A5 不猜路径）", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("鉴权头缺失: %q", gotAuth)
	}
	if gotModel != "deepseek-flash" {
		t.Errorf("请求 model=%q", gotModel)
	}
	if resp.Content != "收到" || resp.Channel != ChannelDefault || resp.ModelID != "deepseek-flash" {
		t.Errorf("响应解析不符: %+v", resp)
	}
	if resp.TotalTokens != 5 {
		t.Errorf("usage 未解析: %+v", resp)
	}
}

func TestLoadDotEnvSetsOnlyMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\nVHS_DOTENV_A=one\nVHS_DOTENV_B=\"two\"\nVHS_DOTENV_KEEP=should-not-override\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VHS_DOTENV_KEEP", "original")
	t.Setenv("VHS_DOTENV_A", "")
	t.Setenv("VHS_DOTENV_B", "")
	n, err := LoadDotEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("应填充 2 条，实际 %d", n)
	}
	if os.Getenv("VHS_DOTENV_KEEP") != "original" {
		t.Error("已存在的环境变量被覆盖")
	}
	if os.Getenv("VHS_DOTENV_A") != "one" || os.Getenv("VHS_DOTENV_B") != "two" {
		t.Error("未正确读取")
	}
}

func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(r.Body).Decode(v)
}
