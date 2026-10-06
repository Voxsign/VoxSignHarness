// l2config.go -- L2(  type)  and  (Task A). 
//
// Peter     confirm type, thus**defaultvaluewrite  code +  be config/env overwrite**(modify  i.e.  ). 
// ⚠️ defaultvalue `deepseek-v4-pro` by Lead refer ; **live  data   env   **(VHS_PLAN_L2_LIVE=1). 
package plan

import (
	"context"
	"encoding/json"
	"os"
	"time"
)

// DefaultL2Model is L2 default type(Lead refer ; ** be config/plan.json or env overwrite**). 
const DefaultL2Model = "deepseek-v4-pro"

// DefaultResearchModel is  default type. 
const DefaultResearchModel = "gpt-6-luna"

// L2PlanTimeout is L2 rule calluse  time. 
//
// ⚠️ **UNVALIDATED**:       typerule  ~=59.6s(2026-10-03    time), thusget 120s    . 
const L2PlanTimeout = 120 * time.Second

// EnvPlanL2Model / EnvResearchModel isoverwriteuse  change . 
const (
	EnvPlanL2Model   = "VHS_PLAN_L2_MODEL"
	EnvResearchModel = "VHS_RESEARCH_MODEL"
)

// L2Config is config/plan.json   status. 
type L2Config struct {
	L2Model       string `json:"l2_model"`
	ResearchModel string `json:"research_model"`
}

// LoadL2Config readget  ;   /   ⇒ usedefaultvalue(**  **). 
func LoadL2Config(path string) L2Config {
	cfg := L2Config{L2Model: DefaultL2Model, ResearchModel: DefaultResearchModel}
	if b, err := os.ReadFile(path); err == nil {
		var c L2Config
		if json.Unmarshal(b, &c) == nil {
			if c.L2Model != "" {
				cfg.L2Model = c.L2Model
			}
			if c.ResearchModel != "" {
				cfg.ResearchModel = c.ResearchModel
			}
		}
	}
	return cfg
}

// L2ModelID returnbackoccur   L2  type id: env >   file >  codedefault. 
func L2ModelID(cfgPath string) string {
	if v := os.Getenv(EnvPlanL2Model); v != "" {
		return v
	}
	return LoadL2Config(cfgPath).L2Model
}

// L2Enabled  disconnect L2 is startuse(empty / "off" ⇒  startuse,  asetcsame ruleform). 
func L2Enabled(modelID string) bool {
	return modelID != "" && modelID != "off" && modelID != "disabled"
}

// PlanWithL2 is L2 in : model as nil or startuse ⇒ **finishsafety ruleform**(and  as charseg  ). 
//
//   chain: L2   use/produceoutbebase   reject ⇒ ModelPlanner in  degrade toruleformandtgt Degraded. 
func PlanWithL2(ctx context.Context, goal string, m Manifest, model PlanModel, modelID string) (Plan, error) {
	if model == nil || !L2Enabled(modelID) {
		return LocalPlanner{}.Plan(goal, m) //  startuse: **  because"   "butmodifychangedefault as**
	}
	// ⚠️   (2026-10-03):   type   rule   **59.6s**(  showword +   ), 
	// but ModelPlanner  default timeis 3s ⇒ ** however time**, tablenowason  502/ time,     to source=model. 
	//    formgive  time; **UNVALIDATED**( tgt :     needneedby    split  ). 
	mp := ModelPlanner{Model: model, Fallback: LocalPlanner{}, Timeout: L2PlanTimeout}
	return mp.Plan(goal, m)
}
