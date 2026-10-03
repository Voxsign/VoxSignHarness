package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

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

// ────────────────────────────────────────────────────────────────────────────
// 模型调度（ChatWithFallback / 健康状态）单元测试 —— 2026-10-03
// ────────────────────────────────────────────────────────────────────────────

// failProvider 是注入故障的 mock：可配置始终失败（模拟 402 欠费模型）或成功。
type failProvider struct {
	name string
	fail bool
}

func (f *failProvider) Name() string { return f.name }
func (f *failProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if ctx.Err() != nil {
		return ChatResponse{}, ctx.Err()
	}
	if f.fail {
		return ChatResponse{}, ErrModelDown // 模型级故障（等价 402/欠费）
	}
	return ChatResponse{Content: "pong", FinishReason: "stop"}, nil
}

// TestChatWithFallbackSkipsDownModel：down 模型被跳过，请求落到下一个健康模型。
func TestChatWithFallbackSkipsDownModel(t *testing.T) {
	reg := &Registry{providers: map[string]Provider{
		"bad":  &failProvider{name: "bad", fail: true},
		"good": &failProvider{name: "good"},
	}, health: map[string]*providerHealth{}}

	// bad 第一次失败 → ErrModelDown → 一次即 down；第二次请求应直接跳过 bad。
	ctx := context.Background()
	_, _, err := reg.ChatWithFallback(ctx, []string{"bad", "good"}, ChatRequest{})
	if err != nil {
		t.Fatalf("第一调用应降级成功: %v", err)
	}
	if reg.healthOf("bad") != healthDown {
		t.Fatal("ErrModelDown 一次失败后 bad 应标记 down")
	}
	if reg.healthOf("good") != healthOK {
		t.Fatal("good 应保持 ok")
	}
}

// TestChatWithFallbackConsecFailThreshold：连续业务失败达阈值才 down，且 down 后跳过。
func TestChatWithFallbackConsecFailThreshold(t *testing.T) {
	old := DownThreshold
	DownThreshold = 2
	defer func() { DownThreshold = old }()

	var calls int
	flaky := &countingFailProvider{name: "flaky", failTimes: 3, calls: &calls}
	reg := &Registry{providers: map[string]Provider{
		"flaky": flaky,
		"good":  &failProvider{name: "good"},
	}, health: map[string]*providerHealth{}}

	ctx := context.Background()
	// 第 1 次：flaky 失败（consecFail=1，未达阈值）→ 降级 good 成功。
	if _, _, err := reg.ChatWithFallback(ctx, []string{"flaky", "good"}, ChatRequest{}); err != nil {
		t.Fatalf("第1次应降级成功: %v", err)
	}
	if reg.healthOf("flaky") != healthOK {
		t.Fatal("连续 1 次失败不应 down")
	}
	// 第 2 次：flaky 失败（consecFail=2，达阈值）→ down。
	if _, _, err := reg.ChatWithFallback(ctx, []string{"flaky", "good"}, ChatRequest{}); err != nil {
		t.Fatalf("第2次应降级成功: %v", err)
	}
	if reg.healthOf("flaky") != healthDown {
		t.Fatal("连续 2 次失败应标记 down")
	}
	// 第 3 次：flaky 已 down → 直接跳过，只调 good。
	before := calls
	if _, name, err := reg.ChatWithFallback(ctx, []string{"flaky", "good"}, ChatRequest{}); err != nil || name != "good" {
		t.Fatalf("down 后应跳过 flaky 直接用 good: name=%s err=%v", name, err)
	}
	if calls != before {
		t.Fatal("down 模型不应再被调用")
	}
}

// countingFailProvider 前 failTimes 次失败，之后成功。
type countingFailProvider struct {
	name      string
	failTimes int
	calls     *int
}

func (c *countingFailProvider) Name() string { return c.name }
func (c *countingFailProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	*c.calls++
	if *c.calls <= c.failTimes {
		return ChatResponse{}, fmt.Errorf("HTTP 500: upstream transient error")
	}
	return ChatResponse{Content: "pong", FinishReason: "stop"}, nil
}

// TestProbeRecoversDownModel：probeDue 探测成功 → down 恢复 ok。
func TestProbeRecoversDownModel(t *testing.T) {
	oldT := DownThreshold
	DownThreshold = 1
	defer func() { DownThreshold = oldT }()
	reg := &Registry{providers: map[string]Provider{
		"flaky": &countingFailProvider{name: "flaky", failTimes: 1, calls: new(int)},
	}, health: map[string]*providerHealth{}}

	ctx := context.Background()
	reg.ChatWithFallback(ctx, []string{"flaky"}, ChatRequest{}) // 失败 1 次 → down
	if reg.healthOf("flaky") != healthDown {
		t.Fatal("应已 down")
	}
	reg.hmu.Lock()
	reg.health["flaky"].nextProbeAt = time.Now().Add(-time.Minute) // 提前到期
	reg.hmu.Unlock()
	reg.probeDue(ctx) // 探测（下次调用已成功）
	if reg.healthOf("flaky") != healthOK {
		t.Fatal("探测成功后应恢复 ok")
	}
}
