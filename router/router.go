// Package router by"intent + close word"pipe   requireroutebyto (provider, max_turns)(   §6.1). 
// this packageonlydependency contract/config,       /orchestrate  . 
package router

import (
	"fmt"
	"strings"

	"voicesign-harness/config"
	"voicesign-harness/contract"
)

// Route isresolve after routebyclose : provider nameor local, max_turns=0 tableshow local   . 
type Route struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	MaxTurns int    `json:"max_turns"`
}

// Resolve by resolve routeby: 
//
//	① cfg.ForcedRoute  empty -> byname (    ); 
//	②  defaultroutebyby : route.Intent   intent.Intent  in;  then route.Match   close word 
//	   prompt in(  write     ) in; 
//	③ Default==true  bot; 
//	④ cfg.ForcedProvider  empty -> overwrite inrouteby  Provider; 
//	⑤ verify Provider ∈ providers tableor == config.LocalProvider,     ; 
//	⑥ MaxTurns = cfg.EffectiveMaxTurns(route)(local -> 0). 
func Resolve(cfg *config.Config, intent contract.Intent, prompt string) (Route, error) {
	var hit *config.Route

	// ①  restrictroutebyname
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
		// ②  defaultrouteby: intent in first, its close word in(first infirst )
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
		// ③ default  bot
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

	// ④ provider overwrite(firstgetroutebyvoice   provider, againbe ForcedProvider overwrite)
	provider := hit.Provider
	if cfg.ForcedProvider != "" {
		provider = cfg.ForcedProvider
	}

	// ⑤ verify provider: local     , its     providers tablein
	if provider != config.LocalProvider {
		if _, ok := cfg.ProviderByName(provider); !ok {
			return Route{}, fmt.Errorf("provider %q 未在 providers 表中定义", provider)
		}
	}

	// ⑥ occur  max_turns(local -> 0; routebyvoice  0 -> useglobaldefault)
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

// contains    list is   target. 
func contains(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// matchAny    keywords in  close wordis  as  write     outnow  prompt in. 
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
