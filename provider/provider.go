// Package provider 是模型接入层（架构 §6 / §14.2）：
// 对上层（agent）暴露统一的 Provider 接口，对内封装 OpenAI 兼容多端点客户端与内置 mock。
// 本包只依赖 contract / config 两个包，零外部依赖（仅标准库）。
//
// 扩展方式：新增端点 = config providers 表加一行；新增 kind = 在 NewRegistry 中实现本接口并注册。
package provider

import (
	"context"
	"fmt"

	"voicesign-harness/config"
	"voicesign-harness/contract"
)

// Provider 是可插拔的模型端点（架构 §6.3）。实现需保证：
//   - 永不向日志/错误信息中泄露 API key；
//   - ctx 取消时立即返回 ctx.Err()。
type Provider interface {
	// Name 返回注册名（与 config providers 表中的 name 一致）。
	Name() string
	// Chat 发一次 OpenAI 兼容聊天补全请求。MaxTokens 为 0 时不发送该字段。
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// ChatRequest 是一次模型调用的入参。
type ChatRequest struct {
	Messages  []contract.Message // 完整消息历史（system/user/assistant）
	MaxTokens int                // 0 = 不在请求体中携带 max_tokens
	// ResponseFormat 覆盖 provider 级 response_format 设置：
	//   true  = 本次调用强制 json_object；false = 本次调用不带 response_format；
	//   nil   = 用 provider 配置（EffectiveResponseFormat）。
	// 用途：QUERY 自然语言回答层要纯文本，prompt 已禁 JSON，必须关掉 json_object，
	// 否则模型在矛盾指令下输出无意义 JSON 壳（如 {"x":0}）→ 误判降级（M7 实测 2026-10-03）。
	ResponseFormat *bool
}

// ChatResponse 是模型调用的出参（与上游协议解耦的最小信封）。
type ChatResponse struct {
	Content      string         // choices[0].message.content（模型原始文本，通常是 ActionPlan JSON）
	FinishReason string         // choices[0].finish_reason
	Usage        contract.Usage // token 用量
}

// Registry 是 provider 名 → 实例 的注册表。由 config 一次性构建，运行期只读。
type Registry struct {
	providers map[string]Provider
	mockSet   map[string]bool // mock 名集合，供 IsMock 判定
	order     []string        // 注册顺序，保证 Names() 确定性输出
}

// NewRegistry 逐个构建 config 中声明的 provider：
// kind=mock → 内置离线 mock；kind=openai → OpenAI 兼容客户端；
// 未知 kind 在此二次防御报错（正常情况下 config.validate 已拦截）。
func NewRegistry(cfg *config.Config) (*Registry, error) {
	r := &Registry{
		providers: map[string]Provider{},
		mockSet:   map[string]bool{},
	}
	for _, p := range cfg.Providers {
		var prov Provider
		switch p.Kind {
		case config.MockKind:
			prov = &mockProvider{name: p.Name}
			r.mockSet[p.Name] = true
		case config.OpenAIKind:
			prov = newOpenAIClient(p, cfg)
		default:
			// 二次防御：config.validate 已拒绝未知 kind，这里兜底防止被绕过。
			return nil, fmt.Errorf("provider %q: 不支持的 kind %q（仅 %s/%s）",
				p.Name, p.Kind, config.OpenAIKind, config.MockKind)
		}
		r.providers[p.Name] = prov
		r.order = append(r.order, p.Name)
	}
	return r, nil
}

// Get 按名取 provider；未注册的名字返回 error。
func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider 未注册: %q", name)
	}
	return p, nil
}

// Names 按注册顺序返回全部 provider 名（确定性，便于轨迹/调试输出）。
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// IsMock 报告该 provider 是否为内置离线 mock（不发网络请求，无 key 即可跑通闭环）。
func (r *Registry) IsMock(name string) bool {
	return r.mockSet[name]
}

// mockProvider 内置离线端点：不发任何网络请求，返回固定的合法 ActionPlan JSON，
// 供本地闭环开发/演示与单元测试使用。唯一会失败的情况是 ctx 被取消。
type mockProvider struct {
	name string
}

// mockContent 是 mock 固定返回的内容（合法 ActionPlan，可直接被 contract.ParseActionPlan 解析）。
const mockContent = `{"actions":[{"tool":"get_time"}],"final":"（本地闭环测试）已获取系统时间"}`

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	select {
	case <-ctx.Done():
		return ChatResponse{}, ctx.Err()
	default:
	}
	return ChatResponse{
		Content:      mockContent,
		FinishReason: "stop",
		Usage:        contract.Usage{}, // 本地 mock，用量全 0
	}, nil
}
