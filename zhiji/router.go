// router.go --    ·   routeby(   v1.0 §6.4 / ADR-005  ly). 
//
//   path(Phase 0 -> Phase 2): 
//   ruletable(LiteLLM complexity_router form)->    form(only   modifychangelineon)
//   -> BERT routeby (keep 95%   ,   50%+   typecalluse; routebyopen  <0.4%). 
// basefile nowbefore  : ruletable +    form(    safety   shadow_log, 
// is Phase 2 routeby         split). 
package zhiji

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TaskProfile task  (routeby in). 
type TaskProfile struct {
	Complexity        float64 `json:"complexity"` // 0–1    (rulerouteby signal)
	RequiredStrength  string  `json:"required_strength,omitempty"` //   needrequire(e.g. "reasoning"|"extraction")
	Domain            string  `json:"domain,omitempty"`
	EstimatedTokens   int     `json:"estimated_tokens,omitempty"`
}

// RouteRule   rule( time  , get    in). 
type RouteRule struct {
	MinComplexity float64 `json:"min"`
	MaxComplexity float64 `json:"max"`
	ModelID       string  `json:"model_id"`
	Reason        string  `json:"reason,omitempty"`
}

// ShadowDecision    formdecide   (Phase 2    orig ). 
type ShadowDecision struct {
	At         time.Time   `json:"at"`
	Task       TaskProfile `json:"task"`
	Chosen     string      `json:"chosen"` //     
	Production string      `json:"production"` // lineon  
	Hit        string      `json:"hit"`
}

// Router ruletablerouteby (line safesafety). 
type Router struct {
	registry *Registry
	rules    []RouteRule
	shadow   bool

	mu         sync.Mutex
	shadowLog  []ShadowDecision
	decisions  int64
	shadowFile string
}

// NewRouter   ruletablerouteby (default  rule). 
func NewRouter(reg *Registry, dir string) (*Router, error) {
	if reg == nil {
		return nil, errors.New("zhiji: 路由器需要注册表")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	r := &Router{
		registry:   reg,
		shadow:     false, // defaultlineon form; M1    form formopenstart
		shadowFile: filepath.Join(dir, "shadow_log.json"),
		rules: []RouteRule{
			{MinComplexity: 0.0, MaxComplexity: 0.3, ModelID: "cheap", Reason: "低复杂度走弱模型（规则表 v1）"},
			{MinComplexity: 0.3, MaxComplexity: 0.7, ModelID: "mid", Reason: "中复杂度走中档模型"},
			{MinComplexity: 0.7, MaxComplexity: 1.01, ModelID: "premium", Reason: "高复杂度走强模型"},
		},
	}
	return r, nil
}

// SetShadow      form(  only    ,  modifychangelineondecide ). 
func (r *Router) SetShadow(on bool) { r.shadow = on }

// SetRules overwriteruletable(Phase 1 by  numdatacall ). 
func (r *Router) SetRules(rules []RouteRule) {
	r.rules = rules
}

// Decide routebydecide : task_profile ->  type  . 
// open :  ruletable table +   note tableread(<0.4% objtgt,  sec ). 
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
	// note tableconfirm typestore ;  store thenback default type. 
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
		production = r.productionModel(p) //    form: lineon  orig type,   only  
		r.record(p, model, production, hit)
	}
	r.mu.Lock()
	r.decisions++
	r.mu.Unlock()
	return RouteDecision{ModelID: model, Production: production, RuleHit: hit, Shadow: r.shadow}, nil
}

// RouteDecision routebyclose . 
type RouteDecision struct {
	ModelID    string `json:"model_id"`     //    type
	Production string `json:"production"`   // lineon   type(   formunderand   same)
	RuleHit    string `json:"rule_hit"`
	Shadow     bool   `json:"shadow"`
}

// productionModel    formunderlineon   type(default:  startuseroutebytime default/orig  ). 
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

// ShadowLog   decide   (Phase 2    ). 
func (r *Router) ShadowLog() []ShadowDecision {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ShadowDecision, len(r.shadowLog))
	copy(out, r.shadowLog)
	return out
}

// Decisions decide  num. 
func (r *Router) Decisions() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.decisions
}

// FlushShadow     day (JSONL   ). 
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
