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

// validatePlanSteps checks the internal consistency of an ordered plan: bounded
// size, unique single-line titles, and a valid status order (completed steps come
// first, at most one in_progress).
func validatePlanSteps(steps []reactPlanStep) error {
	if len(steps) == 0 || len(steps) > 8 {
		return fmt.Errorf("plan requires 1–8 ordered steps")
	}
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
	}
	return nil
}

// updatePlan validates a bounded replacement before committing it. The goal is
// owned by the caller and cannot be overwritten by model actions.
//
// Robustness (R21): strong models rewrite the full step list from a long context
// and occasionally omit an existing step. Instead of rejecting the whole update
// ("cannot delete"), omitted steps are retained with their prior status — old
// order is preserved, model updates are applied by key, and genuinely new steps
// are appended. A completed step can never regress.
func (t *reactThread) updatePlan(steps []reactPlanStep) error {
	for i := range steps {
		steps[i].Step = strings.TrimSpace(steps[i].Step) // store trimmed titles (m7)
	}
	seen := map[string]bool{}
	for _, s := range steps {
		k := planStepKey(s.Step)
		if seen[k] {
			return fmt.Errorf("duplicate step in plan update (normalize whitespace): %s", s.Step)
		}
		seen[k] = true
	}
	if err := validatePlanSteps(steps); err != nil {
		return err
	}
	nextByKey := map[string]reactPlanStep{}
	for _, s := range steps {
		nextByKey[planStepKey(s.Step)] = s
	}
	merged := make([]reactPlanStep, 0, 8)
	used := map[string]bool{}
	retainedOmitted := map[string]bool{}
	for _, old := range t.plan {
		k := planStepKey(old.Step)
		if u, ok := nextByKey[k]; ok {
			if old.Status == "completed" && u.Status != "completed" {
				return fmt.Errorf("completed step cannot regress: %s", old.Step)
			}
			merged = append(merged, u)
		} else {
			kept := old
			if kept.Status == "in_progress" {
				// The model moved its focus to another step; demote the omitted
				// active step so the merge never has two in_progress (M4).
				kept.Status = "pending"
			}
			merged = append(merged, kept) // retain an omitted step; no deletion
			retainedOmitted[k] = true
		}
		used[k] = true
	}
	for _, s := range steps { // append genuinely new steps
		if k := planStepKey(s.Step); !used[k] {
			merged = append(merged, s)
			used[k] = true
		}
	}
	merged = reconcilePlanBound(merged, retainedOmitted)
	if err := validatePlanSteps(merged); err != nil {
		return fmt.Errorf("plan update invalid after merging retained steps: %w", err)
	}
	t.plan = merged
	return nil
}

// reconcilePlanBound compacts completed history only. Unfinished work must never
// disappear to make an overflowing update fit; validation rejects that update.
func reconcilePlanBound(steps []reactPlanStep, _ map[string]bool) []reactPlanStep {
	if len(steps) <= 8 {
		return steps
	}
	completed := 0
	var rest []reactPlanStep
	for _, s := range steps {
		if s.Status == "completed" {
			completed++
		} else {
			rest = append(rest, s)
		}
	}
	if completed == 0 {
		return steps
	}
	return append([]reactPlanStep{{Step: fmt.Sprintf("Earlier completed steps (%d)", completed), Status: "completed"}}, rest...)
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

// canAutoClosePlan reports whether the engine may finish the plan bookkeeping
// after the objective gate has passed. Auto-close is allowed only when a plan
// exists, every step before the last is completed, and at most the final step is
// in_progress (work actually underway). An empty plan or a never-started (pending)
// final step is not auto-closed, so one trivial edit can never pass an unstarted
// plan (B1).
func canAutoClosePlan(plan []reactPlanStep) bool {
	if len(plan) == 0 {
		return false
	}
	for i, s := range plan {
		isLast := i == len(plan)-1
		switch s.Status {
		case "completed":
		case "in_progress":
			if !isLast {
				return false
			}
		default: // pending or unknown: that work was never started/done
			return false
		}
	}
	return true
}

// markFinalStepComplete closes the final in_progress plan step. It is called only
// after canAutoClosePlan passed and the objective gate verified, so only the
// bookkeeping of the active final step is finished — never unstarted steps.
func (t *reactThread) markFinalStepComplete() {
	if len(t.plan) == 0 {
		return
	}
	t.plan[len(t.plan)-1].Status = "completed"
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
