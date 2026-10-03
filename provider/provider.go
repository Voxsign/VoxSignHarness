// Package provider 是模型接入层（架构 §6 / §14.2）：
// 对上层（agent）暴露统一的 Provider 接口，对内封装 OpenAI 兼容多端点客户端与内置 mock。
// 本包只依赖 contract / config 两个包，零外部依赖（仅标准库）。
//
// 扩展方式：新增端点 = config providers 表加一行；新增 kind = 在 NewRegistry 中实现本接口并注册。
package provider

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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

	// 模型调度（2026-10-03）：health 记录每个 provider 的健康状态，
	// 供 ChatWithFallback 跳过 down 模型、ProbeDownLoop 定期探测恢复。
	health map[string]*providerHealth
	hmu    sync.Mutex
}

// NewRegistry 逐个构建 config 中声明的 provider：
// kind=mock → 内置离线 mock；kind=openai → OpenAI 兼容客户端；
// 未知 kind 在此二次防御报错（正常情况下 config.validate 已拦截）。
func NewRegistry(cfg *config.Config) (*Registry, error) {
	r := &Registry{
		providers: map[string]Provider{},
		mockSet:   map[string]bool{},
		health:    map[string]*providerHealth{},
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

// ────────────────────────────────────────────────────────────────────────────
// 模型调度（2026-10-03，见 docs/模型调度-设计.md）
// ────────────────────────────────────────────────────────────────────────────

// 健康状态常量。
const (
	healthOK      = iota // 正常
	healthDown           // down：请求跳过，等待定期探测恢复
	healthProbing        // 正在探测恢复（防并发重复探测）
)

// providerHealth 是单个模型的调度状态（进程内内存态，重启后自动重新探测收敛）。
type providerHealth struct {
	status      int
	consecFail  int       // 连续业务失败次数（<2 时仍可尝试；>=2 或 ErrModelDown → down）
	nextProbeAt time.Time // 下一次探测时间（down 后 T 分钟）
}

// downThreshold 连续业务失败多少次标记 down（ErrModelDown 一次即 down）。
const downThreshold = 2

// probeInterval 是 down 模型的自动探测间隔。
const probeInterval = 5 * time.Minute

// DownThreshold 供测试覆盖（一次即 down vs 连续 N 次）。
var DownThreshold = downThreshold

// ChatWithFallback 是模型调度入口：按 pref 顺序尝试**健康**的 provider，
// 失败自动降级到下一个；模型级故障（ErrModelDown）一次即 down 并快速短路。
// 返回 (响应, 实际使用的 provider 名, 错误)。全部候选不可用 → 返回明确错误（含已试列表）。
//
// 设计要点（对齐 docs/模型调度-设计.md §2）：
//   - down 模型直接跳过，不消耗业务重试预算；
//   - 网络超时/5xx 由 Chat 内部重试（最多 3 次），此处只做跨模型降级；
//   - 402/欠费（ErrModelDown）→ 立即标记 down，下一请求直接跳过该模型。
func (r *Registry) ChatWithFallback(ctx context.Context, pref []string, req ChatRequest) (ChatResponse, string, error) {
	var tried []string
	for _, name := range pref {
		p, ok := r.providers[name]
		if !ok {
			continue // 配置里没有这个名字 → 跳过（配置与链允许有差异）
		}
		if r.healthOf(name) != healthOK {
			continue // down/probing → 跳过
		}
		resp, err := p.Chat(ctx, req)
		if err == nil {
			r.markOK(name)
			return resp, name, nil
		}
		tried = append(tried, name)
		r.markFail(name, err)
		if errors.Is(err, ErrModelDown) {
			continue // 模型级故障：一次即 down，快速短路，不再尝试该模型
		}
		// 其他错误（Chat 内部已重试）：降级到下一个模型
	}
	return ChatResponse{}, "", fmt.Errorf("全部 %d 个候选模型不可用（tried=%v）", len(pref), tried)
}

// healthOf 返回某 provider 当前健康状态（未记录 = ok）。
func (r *Registry) healthOf(name string) int {
	r.hmu.Lock()
	defer r.hmu.Unlock()
	h, ok := r.health[name]
	if !ok {
		return healthOK
	}
	return h.status
}

// markOK 成功调用 → 复位健康状态。
func (r *Registry) markOK(name string) {
	r.hmu.Lock()
	defer r.hmu.Unlock()
	r.health[name] = &providerHealth{status: healthOK}
}

// markFail 失败调用 → 累计连续失败；ErrModelDown 或连续失败达阈值 → down + 排期探测。
func (r *Registry) markFail(name string, err error) {
	r.hmu.Lock()
	defer r.hmu.Unlock()
	h, ok := r.health[name]
	if !ok {
		h = &providerHealth{status: healthOK}
		r.health[name] = h
	}
	h.consecFail++
	if errors.Is(err, ErrModelDown) || h.consecFail >= DownThreshold {
		h.status = healthDown
		h.nextProbeAt = time.Now().Add(probeInterval)
	}
}

// ProbeDownLoop 是常驻探测协程：每 60s 扫一遍 down 且到探测时间的模型，
// 发最小 1-token ping；成功 → ok（复位），失败 → 顺延下一个探测周期。
// 由调用方以 goroutine 启动（`go r.ProbeDownLoop(ctx)`）。
func (r *Registry) ProbeDownLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.probeDue(ctx)
		}
	}
}

// probeDue 对到期模型执行最小 ping（1-token）。
func (r *Registry) probeDue(ctx context.Context) {
	var due []string
	r.hmu.Lock()
	for name, h := range r.health {
		if h.status == healthDown && time.Now().After(h.nextProbeAt) {
			h.status = healthProbing
			due = append(due, name)
		}
	}
	r.hmu.Unlock()
	for _, name := range due {
		p, ok := r.providers[name]
		if !ok {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err := p.Chat(probeCtx, ChatRequest{
			Messages:  []contract.Message{{Role: "user", Content: "ping"}},
			MaxTokens: 1,
		})
		cancel()
		r.hmu.Lock()
		if err == nil {
			r.health[name] = &providerHealth{status: healthOK}
		} else if h, ok := r.health[name]; ok {
			h.status = healthDown
			h.nextProbeAt = time.Now().Add(probeInterval)
		}
		r.hmu.Unlock()
	}
}
