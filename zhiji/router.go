// router.go —— 知己 · 元层路由（架构 v1.0 §6.4 / ADR-005 落地）。
//
// 演进路径（Phase 0 → Phase 2）：
//   规则表（LiteLLM complexity_router 式）→ 影子模式（只记录不改变线上）
//   → BERT 路由器（保 95% 质量、砍 50%+ 强模型调用；路由开销 <0.4%）。
// 本文件实现前两档：规则表 + 影子模式（影子推荐全部落 shadow_log，
// 是 Phase 2 路由器自举训练集的一部分）。
package zhiji

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TaskProfile 任务画像（路由输入）。
type TaskProfile struct {
	Complexity        float64 `json:"complexity"` // 0–1 复杂度（规则路由主信号）
	RequiredStrength  string  `json:"required_strength,omitempty"` // 强项要求（如 "reasoning"|"extraction"）
	Domain            string  `json:"domain,omitempty"`
	EstimatedTokens   int     `json:"estimated_tokens,omitempty"`
}

// RouteRule 一条规则（区间匹配，取第一条命中）。
type RouteRule struct {
	MinComplexity float64 `json:"min"`
	MaxComplexity float64 `json:"max"`
	ModelID       string  `json:"model_id"`
	Reason        string  `json:"reason,omitempty"`
}

// ShadowDecision 影子模式决策记录（Phase 2 训练集原料）。
type ShadowDecision struct {
	At         time.Time   `json:"at"`
	Task       TaskProfile `json:"task"`
	Chosen     string      `json:"chosen"` // 影子推荐
	Production string      `json:"production"` // 线上实际
	Hit        string      `json:"hit"`
}

// Router 规则表路由器（线程安全）。
type Router struct {
	registry *Registry
	rules    []RouteRule
	shadow   bool

	mu         sync.Mutex
	shadowLog  []ShadowDecision
	decisions  int64
	shadowFile string
}

// NewRouter 构造规则表路由器（默认三档规则）。
func NewRouter(reg *Registry, dir string) (*Router, error) {
	if reg == nil {
		return nil, errors.New("zhiji: 路由器需要注册表")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	r := &Router{
		registry:   reg,
		shadow:     false, // 默认线上模式；M1 影子模式显式开启
		shadowFile: filepath.Join(dir, "shadow_log.json"),
		rules: []RouteRule{
			{MinComplexity: 0.0, MaxComplexity: 0.3, ModelID: "cheap", Reason: "低复杂度走弱模型（规则表 v1）"},
			{MinComplexity: 0.3, MaxComplexity: 0.7, ModelID: "mid", Reason: "中复杂度走中档模型"},
			{MinComplexity: 0.7, MaxComplexity: 1.01, ModelID: "premium", Reason: "高复杂度走强模型"},
		},
	}
	return r, nil
}

// SetShadow 切换影子模式（影子只记录推荐，不改变线上决策）。
func (r *Router) SetShadow(on bool) { r.shadow = on }

// SetRules 覆盖规则表（Phase 1 由经验数据调参）。
func (r *Router) SetRules(rules []RouteRule) {
	r.rules = rules
}

// Decide 路由决策：task_profile → 模型选择。
// 开销：纯规则表查表 + 一次注册表读（<0.4% 目标，微秒级）。
func (r *Router) Decide(p TaskProfile) (RouteDecision, error) {
	model := ""
	hit := ""
	for _, rule := range r.rules {
		if p.Complexity >= rule.MinComplexity && p.Complexity < rule.MaxComplexity {
			model = rule.ModelID
			hit = rule.Reason
			break
		}
	}
	if model == "" {
		return RouteDecision{}, errors.New("zhiji: 无规则命中")
	}
	// 注册表确认模型存在；不存在则回落默认模型。
	if _, err := r.registry.Get(model); err != nil {
		def, derr := r.registry.Default()
		if derr != nil {
			return RouteDecision{}, derr
		}
		model = def.ID
		hit = "规则命中模型未注册 → 回落默认"
	}
	production := model
	if r.shadow {
		production = r.productionModel(p) // 影子模式：线上仍走原模型，影子只推荐
		r.record(p, model, production, hit)
	}
	r.mu.Lock()
	r.decisions++
	r.mu.Unlock()
	return RouteDecision{ModelID: model, Production: production, RuleHit: hit, Shadow: r.shadow}, nil
}

// RouteDecision 路由结果。
type RouteDecision struct {
	ModelID    string `json:"model_id"`     // 推荐模型
	Production string `json:"production"`   // 线上实际模型（影子模式下与推荐不同）
	RuleHit    string `json:"rule_hit"`
	Shadow     bool   `json:"shadow"`
}

// productionModel 影子模式下线上实际模型（默认：未启用路由时走默认/原策略）。
func (r *Router) productionModel(p TaskProfile) string {
	def, err := r.registry.Default()
	if err != nil {
		return ""
	}
	return def.ID
}

func (r *Router) record(p TaskProfile, chosen, production, hit string) {
	r.mu.Lock()
	r.shadowLog = append(r.shadowLog, ShadowDecision{
		At: time.Now(), Task: p, Chosen: chosen, Production: production, Hit: hit,
	})
	r.mu.Unlock()
}

// ShadowLog 影子决策记录（Phase 2 训练集）。
func (r *Router) ShadowLog() []ShadowDecision {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ShadowDecision, len(r.shadowLog))
	copy(out, r.shadowLog)
	return out
}

// Decisions 决策计数。
func (r *Router) Decisions() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.decisions
}

// FlushShadow 落盘影子日志（JSONL 追加）。
func (r *Router) FlushShadow() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.shadowLog) == 0 {
		return nil
	}
	f, err := os.OpenFile(r.shadowFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	for _, d := range r.shadowLog {
		b, err := json.Marshal(d)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	r.shadowLog = r.shadowLog[:0]
	return nil
}
