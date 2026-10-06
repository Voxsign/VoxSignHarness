package router

import (
	"testing"

	"voicesign-harness/config"
	"voicesign-harness/contract"
)

// usedefault  (   §6.1 routebytable)  class in. 

func TestResolveByIntentFileList(t *testing.T) {
	cfg := config.Default()
	r, err := Resolve(&cfg, contract.Intent{Intent: contract.IntentFileList}, "列出文件")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Name != "file" || r.Provider != "center" || r.MaxTurns != 1 {
		t.Errorf("FILE_LIST 应命中 file→center/1, 实际 %+v", r)
	}
}

func TestResolveByKeywordComplex(t *testing.T) {
	cfg := config.Default()
	// use UNKNOWN intent open intent routeby,  " code"close word in complex
	r, err := Resolve(&cfg, contract.Intent{Intent: contract.IntentUnknown}, "帮我写段代码")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Name != "complex" || r.Provider != "strong" || r.MaxTurns != 2 {
		t.Errorf("含代码应命中 complex→strong/2, 实际 %+v", r)
	}
}

func TestResolveDefaultFallback(t *testing.T) {
	cfg := config.Default()
	r, err := Resolve(&cfg, contract.Intent{Intent: contract.IntentAppLaunch}, "打开微信")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Name != "default" || r.Provider != "center" {
		t.Errorf("应兜底到 default→center, 实际 %+v", r)
	}
}

func TestResolveForcedRoute(t *testing.T) {
	cfg := config.Default()
	cfg.ForcedRoute = "time"
	r, err := Resolve(&cfg, contract.Intent{Intent: contract.IntentInfo}, "随便")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Name != "time" || r.Provider != config.LocalProvider || r.MaxTurns != 0 {
		t.Errorf("强制路由 time 应 local/0, 实际 %+v", r)
	}
}

func TestResolveForcedRouteMissing(t *testing.T) {
	cfg := config.Default()
	cfg.ForcedRoute = "不存在的路由"
	if _, err := Resolve(&cfg, contract.Intent{Intent: contract.IntentInfo}, "x"); err == nil {
		t.Error("缺失的强制路由应报错")
	}
}

func TestResolveForcedProviderOverride(t *testing.T) {
	cfg := config.Default()
	cfg.ForcedProvider = "mock" // overwrite file routebyorigbase  center
	r, err := Resolve(&cfg, contract.Intent{Intent: contract.IntentFileList}, "列出")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Name != "file" || r.Provider != "mock" || r.MaxTurns != 1 {
		t.Errorf("应命中 file 但 provider 被覆盖为 mock/1, 实际 %+v", r)
	}
}

func TestResolveUnknownProviderErrors(t *testing.T) {
	//       referto   provider  routeby(   config.Load verify,  connect  Resolve)
	cfg := config.Config{
		Global: config.Global{MaxTurnsDefault: 2},
		Providers: []config.Provider{
			{Name: "center", Kind: config.OpenAIKind, Endpoint: "https://x", Model: "m"},
		},
		Routes: []config.Route{
			{Name: "default", Provider: "ghost", Default: true},
		},
	}
	if _, err := Resolve(&cfg, contract.Intent{Intent: contract.IntentInfo}, "x"); err == nil {
		t.Error("未知 provider 应报错")
	}
}

func TestResolveTimeLocalMaxTurnsZero(t *testing.T) {
	cfg := config.Default()
	r, err := Resolve(&cfg, contract.Intent{Intent: contract.IntentTime}, "现在几点")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Provider != config.LocalProvider || r.MaxTurns != 0 {
		t.Errorf("TIME 应 local 直通 MaxTurns=0, 实际 %+v", r)
	}
}
