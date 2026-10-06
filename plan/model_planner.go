// model_planner.go --  typeformrule  : ** type rule , base   boundary**. 
//
// to     origthen:    base (  list, domain forbid,  boundary  ),    out( type). 
//
//  needrequire(Lead  decide): 
//   -   modelcenter   `default`   (  name  ,  typefrom  read); 
//   -  typeonlyproduceout**  **  ,  boundaryandrejectby**base   **(PM-1/PM-4); 
//   - PM-2:  type  use/ time/ form /   -> **    **(ruleform -> read-only bot), 
//        ,    , and    tgt `Degraded`(default as,  isexampleoutpath); 
//   - PM-5:       `Why`( data),  then as  resolve  ->   . 
//
//  data  `plan/model_criteria_test.go`(tag `vhsplanmodel`), **   ** PL/RV   15  . 
package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PlanModel is"    occurbecome "( typeside). base onlypipe cur  . 
type PlanModel interface {
	Propose(ctx context.Context, goal string, m Manifest) (string, error)
}

// ModelPlanner   " type   + base  boundary". 
type ModelPlanner struct {
	Model    PlanModel
	Fallback Planner       // default LocalPlanner{}
	Timeout  time.Duration // default 3s(needrequire Δ7:  type bot time)
}

// modelPlanStep is type        . 
type modelPlanStep struct {
	Tool   string            `json:"tool"`
	Caps   []string          `json:"caps"`
	Params map[string]string `json:"params"`
	Action string            `json:"action"`
	Output string            `json:"output"`
	Why    string            `json:"why"`
	Domain string            `json:"domain"`
}

type modelPlanJSON struct {
	Steps   []modelPlanStep `json:"steps"`
	Refused bool            `json:"refused"`
	Missing []string        `json:"missing"`
	Reason  string          `json:"reason"`
}

// Plan produceout  . **  returnbackerror**:    typeside  all  (PM-2). 
func (mp ModelPlanner) Plan(goal string, m Manifest) (Plan, error) {
	fb := mp.Fallback
	if fb == nil {
		fb = LocalPlanner{}
	}
	if mp.Model == nil {
		return degrade(goal, m, fb, "未配置模型"), nil
	}
	timeout := mp.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	raw, err := mp.Model.Propose(ctx, goal, m)
	if err != nil {
		return degrade(goal, m, fb, "模型不可用/超时："+err.Error()), nil
	}
	cand, err := parseModelPlan(raw)
	if err != nil {
		return degrade(goal, m, fb, "模型输出不是合法计划 JSON："+err.Error()), nil
	}

	// ---- base  boundary  ( type  out-of-scope)----
	steps, violations := reviewStepsFor(cand.Steps, m, goal)
	if len(violations) > 0 {
		return degrade(goal, m, fb, "本机复核拒绝模型计划："+strings.Join(violations, "; ")), nil
	}
	// objtgtbase  outlist   -> reject(andruleformsame   PM-1/PL-2  path). 
	if missing, ok := unmetRequirement(strings.ToLower(goal), m); !ok {
		return Plan{Goal: goal, Source: "model", Refused: true, Missing: missing,
			Reason: "目标需要清单外能力（本机复核）"}, nil
	}
	if cand.Refused || len(steps) == 0 {
		return Plan{Goal: goal, Source: "model", Refused: true, Missing: cand.Missing,
			Reason: cand.Reason}, nil
	}
	return Plan{Goal: goal, Steps: steps, Source: "model"}, nil
}

// parseModelPlan resolve  type out(   ```json   andbeforeafter voice). 
func parseModelPlan(raw string) (modelPlanJSON, error) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 && j+1 <= len(s) {
		s = s[:j+1]
	}
	var out modelPlanJSON
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return modelPlanJSON{}, err
	}
	if !out.Refused && len(out.Steps) == 0 {
		return modelPlanJSON{}, fmt.Errorf("既未拒绝也没有步骤")
	}
	return out, nil
}

// reviewSteps     :    listin, cap  at   , domain  AllowedSpaces in, 
//  num/produceout/ data  (PM-1/PM-4/PM-5).      rulei.e.   violation. 
func reviewSteps(in []modelPlanStep, m Manifest) ([]Step, []string) {
	return reviewStepsFor(in, m, "") // noobjtgt base ⇒  ed PM-6  objtgt close disconnect
}

// reviewStepsFor is reviewSteps  finish  :   goal time   PM-6(  risk   first). 
func reviewStepsFor(in []modelPlanStep, m Manifest, goal string) ([]Step, []string) {
	tools := map[string]Capability{}
	for _, c := range m.Tools {
		tools[c.Name] = c
	}
	var out []Step
	var bad []string
	for i, s := range in {
		c, ok := tools[s.Tool]
		if !ok {
			bad = append(bad, fmt.Sprintf("第 %d 步工具 %q 不在能力清单内（幻觉）", i, s.Tool))
			continue
		}
		for _, cap := range s.Caps {
			if !contains(c.Caps, cap) {
				bad = append(bad, fmt.Sprintf("第 %d 步 cap %q 不属于工具 %s", i, cap, s.Tool))
			}
		}
		if s.Domain != "" && !contains(c.AllowedSpaces, s.Domain) {
			bad = append(bad, fmt.Sprintf("第 %d 步越域：%s 不允许在 %s（本机域门禁优先）", i, s.Tool, s.Domain))
		}
		if len(s.Params) == 0 || s.Output == "" || s.Why == "" {
			bad = append(bad, fmt.Sprintf("第 %d 步不可执行或不可解释（params/output/why 缺）", i))
		}
		out = append(out, Step{
			Tool: s.Tool, Caps: s.Caps, Params: s.Params,
			Action: s.Action, Output: s.Output, Why: s.Why, Domain: s.Domain,
		})
	}
	if goal != "" {
		bad = append(bad, reviewMinRisk(goal, out, m)...) // PM-6(base    )
	}
	return out, bad
}

// degrade  "ruleform -> read-only bot", andtgt   (PM-2 default as). 
func degrade(goal string, m Manifest, fb Planner, reason string) Plan {
	p, err := fb.Plan(goal, m)
	if err != nil || (len(p.Steps) == 0 && !p.Refused) {
		p = Plan{Goal: goal, Steps: searchSteps(), Source: "readonly-fallback"}
		if missing := missingTools(m, p.Steps); len(missing) > 0 {
			p = Plan{Goal: goal, Refused: true, Missing: missing, Reason: "清单内无只读能力可用"}
		}
	}
	p.Degraded = true
	p.DegradedReason = reason
	if p.Source == "" {
		p.Source = "rule"
	}
	return p
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
