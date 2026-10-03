// model_planner.go —— 模型式规划器：**模型做规划，本机做边界**。
//
// 对齐架构第一原则：事实在本机（能力清单、域门禁、边界复核），推理在外（模型）。
//
// 硬要求（Lead 裁决）：
//   - 走 modelcenter 的 `default` 通道（通道名固定，模型从配置读）；
//   - 模型只产出**候选**计划，边界与拒绝由**本机复核**（PM-1/PM-4）；
//   - PM-2：模型不可用/超时/格式错/幻觉 → **一律降级**（规则式 → 只读兜底），
//     不抛错、不卡死，并在计划里标 `Degraded`（默认行为，不是例外路径）；
//   - PM-5：每步必须带 `Why`（依据），否则视为不可解释 → 降级。
//
// 判据在 `plan/model_criteria_test.go`（tag `vhsplanmodel`），**不混进** PL/RV 那 15 条。
package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PlanModel 是"候选计划生成器"（模型侧）。本机只把它当建议。
type PlanModel interface {
	Propose(ctx context.Context, goal string, m Manifest) (string, error)
}

// ModelPlanner 组合"模型建议 + 本机边界"。
type ModelPlanner struct {
	Model    PlanModel
	Fallback Planner       // 默认 LocalPlanner{}
	Timeout  time.Duration // 默认 3s（需求 Δ7：模型兜底超时）
}

// modelPlanStep 是模型候选计划里的一步。
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

// Plan 产出计划。**永不返回错误**：任何模型侧问题都降级（PM-2）。
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

	// ---- 本机边界复核（模型不得越界）----
	steps, violations := reviewSteps(cand.Steps, m)
	if len(violations) > 0 {
		return degrade(goal, m, fb, "本机复核拒绝模型计划："+strings.Join(violations, "; ")), nil
	}
	// 目标本身超出清单能力 → 拒绝（与规则式同源的 PM-1/PL-2 口径）。
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

// parseModelPlan 解析模型输出（容忍 ```json 围栏与前后噪声）。
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

// reviewSteps 逐条复核：工具在清单内、cap 属于该工具、域在 AllowedSpaces 内、
// 参数/产出/依据齐备（PM-1/PM-4/PM-5）。任一条不合规即记录 violation。
func reviewSteps(in []modelPlanStep, m Manifest) ([]Step, []string) {
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
	return out, bad
}

// degrade 走"规则式 → 只读兜底"，并标明降级（PM-2 默认行为）。
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
