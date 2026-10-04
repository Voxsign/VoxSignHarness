package provider

import (
	"context"
	"testing"

	"voicesign-harness/config"
	"voicesign-harness/contract"
)

// testConfig 构造一个仅含指定 providers 的配置（Global 给个默认超时，避免除零）。
func testConfig(ps ...config.Provider) *config.Config {
	return &config.Config{
		Global:    config.Global{LLMTimeoutMs: 10000},
		Providers: ps,
	}
}

func TestRegistryMockGetAndIsMock(t *testing.T) {
	cfg := testConfig(
		config.Provider{Name: "mock", Kind: config.MockKind},
		config.Provider{Name: "center", Kind: config.OpenAIKind, Endpoint: "https://example.com", Model: "gpt-test"},
	)
	r, err := NewRegistry(cfg)
	if err != nil {
		t.Fatalf("NewRegistry 失败: %v", err)
	}

	// mock 的 Get / IsMock
	p, err := r.Get("mock")
	if err != nil {
		t.Fatalf("Get(mock) 失败: %v", err)
	}
	if p.Name() != "mock" {
		t.Errorf("mock Name() = %q", p.Name())
	}
	if !r.IsMock("mock") {
		t.Error("IsMock(mock) = false, want true")
	}
	if !r.IsMock("mock") || r.IsMock("center") {
		// 上面已断言 mock；这里补断言 openai 不是 mock
	}
	if r.IsMock("center") {
		t.Error("IsMock(center) = true, want false")
	}

	// mock Chat：固定内容、零用量、不报错
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []contract.Message{{Role: contract.RoleUser, Content: "time"}},
	})
	if err != nil {
		t.Fatalf("mock Chat 报错: %v", err)
	}
	if resp.Content == "" {
		t.Error("mock Content 为空")
	}
	if resp.Usage != (contract.Usage{}) {
		t.Errorf("mock Usage 应为全 0, got %+v", resp.Usage)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("mock FinishReason = %q", resp.FinishReason)
	}
	// 返回内容必须是合法 ActionPlan JSON
	if _, err := contract.ParseActionPlan(resp.Content); err != nil {
		t.Errorf("mock 内容不是合法 ActionPlan: %v", err)
	}

	// Names 按注册顺序
	names := r.Names()
	if len(names) != 2 || names[0] != "mock" || names[1] != "center" {
		t.Errorf("Names() = %v", names)
	}
}

func TestRegistryUnknownGetError(t *testing.T) {
	r, err := NewRegistry(testConfig(config.Provider{Name: "mock", Kind: config.MockKind}))
	if err != nil {
		t.Fatalf("NewRegistry 失败: %v", err)
	}
	if _, err := r.Get("no-such-provider"); err == nil {
		t.Error("Get 未知名应报错, got nil")
	}
	if r.IsMock("no-such-provider") {
		t.Error("IsMock 未知名应返回 false")
	}
}

func TestRegistryInvalidKindDefense(t *testing.T) {
	// config.validate 正常会拦截未知 kind；这里直接构造一个绕过校验的 Config，
	// 验证 NewRegistry 的二次防御报错。
	bad := testConfig(config.Provider{Name: "weird", Kind: "anthropic"})
	if _, err := NewRegistry(bad); err == nil {
		t.Fatal("NewRegistry 对未知 kind 应报错, got nil")
	}
}

func TestMockCtxCancel(t *testing.T) {
	p := &mockProvider{name: "mock"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消
	if _, err := p.Chat(ctx, ChatRequest{}); err == nil {
		t.Error("ctx 已取消时 mock Chat 应返回 ctx.Err(), got nil")
	}
}
