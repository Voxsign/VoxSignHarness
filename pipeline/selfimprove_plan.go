package pipeline

import (
	"fmt"
	"strings"
	"unicode"
)

type reactPlanStep struct {
	Step   string `json:"step"`
	Status string `json:"status"`
}

// updatePlan validates a bounded replacement before committing it. The goal is
// owned by the caller and cannot be overwritten by model actions.
func (t *reactThread) updatePlan(steps []reactPlanStep) error {
	if len(steps) == 0 || len(steps) > 8 {
		return fmt.Errorf("plan requires 1–8 ordered steps")
	}
	next := make([]reactPlanStep, len(steps))
	active := 0
	unfinished := false
	seen := map[string]bool{}
	for i, s := range steps {
		for _, r := range s.Step {
			if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
				return fmt.Errorf("plan titles must not contain control characters")
			}
		}
		s.Step = strings.TrimSpace(s.Step)
		key := planStepKey(s.Step)
		if s.Step == "" || len([]rune(s.Step)) > 160 || strings.ContainsAny(s.Step, "\r\n") || seen[key] {
			return fmt.Errorf("plan step %d must have a unique, single-line title of 1–160 characters", i+1)
		}
		seen[key] = true
		switch s.Status {
		case "completed":
			if unfinished {
				return fmt.Errorf("complete earlier plan steps before later steps")
			}
		case "in_progress":
			active++
			if active > 1 || unfinished {
				return fmt.Errorf("only the first unfinished step may be in_progress")
			}
			unfinished = true
		case "pending":
			unfinished = true
		default:
			return fmt.Errorf("invalid plan status %q: use pending/in_progress/completed", s.Status)
		}
		next[i] = s
	}
	for _, old := range t.plan {
		found := false
		for _, updated := range next {
			if planStepKey(old.Step) == planStepKey(updated.Step) {
				found = true
				if old.Status == "completed" && updated.Status != "completed" {
					return fmt.Errorf("completed step cannot regress: %s", old.Step)
				}
			}
		}
		if !found {
			return fmt.Errorf("existing plan step cannot be deleted: %s", old.Step)
		}
	}
	t.plan = next
	return nil
}

func (t *reactThread) renderPlan() string {
	var b strings.Builder
	b.WriteString("PERSISTENT GOAL: " + t.goal + "\nPLAN (model progress; build/test is verified separately):\n")
	if len(t.plan) == 0 {
		b.WriteString("No plan set. Use tool=plan with ordered steps before declaring done.\n")
	}
	completed := 0
	for i, s := range t.plan {
		fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, s.Status, s.Step)
		if s.Status == "completed" {
			completed++
		}
	}
	fmt.Fprintf(&b, "Progress: %d/%d completed.", completed, len(t.plan))
	return b.String()
}

func (t *reactThread) planCompletionError() error {
	if t.goal != "" && len(t.plan) == 0 {
		return fmt.Errorf("task has no plan: set tool=plan before done")
	}
	for i, s := range t.plan {
		if s.Status != "completed" {
			return fmt.Errorf("plan incomplete: step %d (%s) is %s; continue work and update tool=plan before done", i+1, s.Step, s.Status)
		}
	}
	return nil
}

// Both execution and repair use the same plan state, outside the sliding window.
func (o *Options) executeThreadAction(repo, logDir string, a reactAction, snap map[string]string, t *reactThread) (reactObs, bool, []string) {
	if strings.TrimSpace(a.Tool) == "plan" {
		if err := t.updatePlan(a.Steps); err != nil {
			return reactObs{Obs: err.Error(), Err: err.Error()}, false, nil
		}
		return reactObs{Obs: "Plan updated."}, true, nil
	}
	obs, ok, touched := o.executeReactAction(repo, logDir, a, snap)
	if obs.Err != "" && obs.Obs == "" {
		obs.Obs = obs.Err
	}
	return obs, ok, touched
}

func planStepKey(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
