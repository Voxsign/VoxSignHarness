package provider

import (
	"context"
	"testing"

	"voicesign-harness/config"
	"voicesign-harness/contract"
)

// testConfig     only refer  providers    (Global give default time,     ). 
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

	// mock   Get / IsMock
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
		// onfacealreadydisconnectlang mock;   patchdisconnectlang openai  is mock
	}
	if r.IsMock("center") {
		t.Error("IsMock(center) = true, want false")
	}

	// mock Chat:   in ,  use ,    
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
	// returnbackin   is   ActionPlan JSON
	if _, err := contract.ParseActionPlan(resp.Content); err != nil {
		t.Errorf("mock 内容不是合法 ActionPlan: %v", err)
	}

	// Names bynote   
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
	// config.validate pos  block   kind;    connect     edverify  Config, 
	//    NewRegistry    prevent   . 
	bad := testConfig(config.Provider{Name: "weird", Kind: "anthropic"})
	if _, err := NewRegistry(bad); err == nil {
		t.Fatal("NewRegistry 对未知 kind 应报错, got nil")
	}
}

func TestMockCtxCancel(t *testing.T) {
	p := &mockProvider{name: "mock"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() //  i.e.cancel
	if _, err := p.Chat(ctx, ChatRequest{}); err == nil {
		t.Error("ctx 已取消时 mock Chat 应返回 ctx.Err(), got nil")
	}
}
