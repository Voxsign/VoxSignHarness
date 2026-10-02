// Package router 按「意图 + 关键词」把一次请求路由到 (provider, max_turns)（架构 §6.1）。
// 本包只依赖 contract/config，不做任何网络/编排动作。
package router

import (
	"fmt"
	"strings"

	"voicesign-harness/config"
	"voicesign-harness/contract"
)

// Route 是解析后的路由结果：provider 名或 local，max_turns=0 表示 local 直通。
type Route struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	MaxTurns int    `json:"max_turns"`
}

// Resolve 按序解析路由：
//
//	① cfg.ForcedRoute 非空 → 按名查（缺失报错）；
//	② 非默认路由按序：route.Intent 含 intent.Intent 命中；否则 route.Match 任一关键词在
//	   prompt 中（大小写不敏感子串）命中；
//	③ Default==true 兜底；
//	④ cfg.ForcedProvider 非空 → 覆盖命中路由的 Provider；
//	⑤ 校验 Provider ∈ providers 表或 == config.LocalProvider，未知报错；
//	⑥ MaxTurns = cfg.EffectiveMaxTurns(route)（local → 0）。
func Resolve(cfg *config.Config, intent contract.Intent, prompt string) (Route, error) {
	var hit *config.Route

	// ① 强制路由名
	if cfg.ForcedRoute != "" {
		for i := range cfg.Routes {
			if cfg.Routes[i].Name == cfg.ForcedRoute {
				hit = &cfg.Routes[i]
				break
			}
		}
		if hit == nil {
			return Route{}, fmt.Errorf("强制路由 %q 未在路由表中定义", cfg.ForcedRoute)
		}
	} else {
		// ② 非默认路由：意图命中优先，其次关键词命中（先命中先得）
		for i := range cfg.Routes {
			r := &cfg.Routes[i]
			if r.Default {
				continue
			}
			if contains(r.Intent, intent.Intent) {
				hit = r
				break
			}
			if matchAny(r.Match, prompt) {
				hit = r
				break
			}
		}
		// ③ default 兜底
		if hit == nil {
			for i := range cfg.Routes {
				if cfg.Routes[i].Default {
					hit = &cfg.Routes[i]
					break
				}
			}
		}
		if hit == nil {
			return Route{}, fmt.Errorf("无命中路由且配置中缺少 default 兜底路由")
		}
	}

	// ④ provider 覆盖（先取路由声明的 provider，再被 ForcedProvider 覆盖）
	provider := hit.Provider
	if cfg.ForcedProvider != "" {
		provider = cfg.ForcedProvider
	}

	// ⑤ 校验 provider：local 直通合法，其余必须在 providers 表中
	if provider != config.LocalProvider {
		if _, ok := cfg.ProviderByName(provider); !ok {
			return Route{}, fmt.Errorf("provider %q 未在 providers 表中定义", provider)
		}
	}

	// ⑥ 生效 max_turns（local → 0；路由声明 0 → 用全局默认）
	eff := config.Route{
		Name:     hit.Name,
		Intent:   hit.Intent,
		Match:    hit.Match,
		Provider: provider,
		MaxTurns: hit.MaxTurns,
		Default:  hit.Default,
	}
	maxTurns := cfg.EffectiveMaxTurns(eff)

	return Route{Name: hit.Name, Provider: provider, MaxTurns: maxTurns}, nil
}

// contains 报告 list 是否含 target。
func contains(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// matchAny 报告 keywords 中任一关键词是否作为大小写不敏感子串出现在 prompt 中。
func matchAny(keywords []string, prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, k := range keywords {
		if k == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(k)) {
			return true
		}
	}
	return false
}
