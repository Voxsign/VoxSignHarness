package pipeline

// Goal is the cross-turn durable objective (R9-D1, distilled from DeepSeek
// packages/goal + goal-round-driver).
//
// TaskSlot records "what this turn is doing"; Goal records "what the whole
// objective is, how far it got, and what comes next" — the persistence layer
// for persistent execution. One goal per conversation, stored as a single
// overwrite JSON file under <logDir>/context_slots/goal-<convID>.json so it
// survives restarts and resumes across iOS turns.
//
// Status machine: active -> paused | blocked | completed.
//   - active:    work continues; each turn bumps Round.
//   - paused:    user explicitly paused; a stale pause is surfaced proactively.
//   - blocked:   same failure streak >= 3 with a reason and options (DeepSeek:
//                difficulty != blocker, consecutive-blocked rule).
//   - completed: only when evidence shows the whole objective is achieved
//                (premature "done" is the #1 agent failure mode).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"voicesign-harness/contract"
)

// GoalStatus values.
const (
	GoalActive    = "active"
	GoalPaused    = "paused"
	GoalBlocked   = "blocked"
	GoalCompleted = "completed"
)

// MaxGoalBlockStreak is how many consecutive failing turns before the goal is
// reported blocked (DeepSeek: only a repeated blocker is a real blocker).
const MaxGoalBlockStreak = 3

// GoalStaleMinutes: an active goal untouched for this long is surfaced as
// "上次做到哪，是否继续" (proactive nudge, R9-D5).
const GoalStaleMinutes = 30

type Goal struct {
	Objective     string   `json:"objective"`               // original user instruction, verbatim
	IntentKind    string   `json:"intent_kind,omitempty"`   // executable intent type for resume
	Status        string   `json:"status"`                  // active|paused|blocked|completed
	Round         int      `json:"round"`                   // continuation rounds executed
	Steps         []string `json:"steps,omitempty"`         // executed steps so far
	LastProgress  string   `json:"last_progress,omitempty"` // what the last turn achieved
	NextStep      string   `json:"next_step,omitempty"`     // single next action
	BlockedReason string   `json:"blocked_reason,omitempty"`
	Options       []string `json:"options,omitempty"` // alternatives when blocked
	FailStreak    int      `json:"fail_streak"`
	FirstRetryOK  bool     `json:"first_retry_ok"` // R9-D4: first-retry-success indicator
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

func goalPath(logDir, convID string) string {
	convID = strings.TrimSpace(convID)
	if convID == "" {
		convID = "default"
	}
	return filepath.Join(logDir, "context_slots", "goal-"+convID+".json")
}

func loadGoal(logDir, convID string) *Goal {
	b, err := os.ReadFile(goalPath(logDir, convID))
	if err != nil {
		return nil
	}
	var g Goal
	if json.Unmarshal(b, &g) != nil {
		return nil
	}
	return &g
}

func saveGoal(logDir, convID string, g *Goal) {
	if g == nil {
		return
	}
	g.UpdatedAt = time.Now().Format(time.RFC3339)
	if g.CreatedAt == "" {
		g.CreatedAt = g.UpdatedAt
	}
	dir := filepath.Join(logDir, "context_slots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	b, _ := json.MarshalIndent(g, "", "  ")
	_ = os.WriteFile(goalPath(logDir, convID), b, 0o644)
}

// maybeCreateGoal establishes a durable goal for real-execution intents so a
// later "继续/接着干" turn resumes the whole objective, not just the slot.
func (o *Options) maybeCreateGoal(it contract.Intent) *Goal {
	convID := strings.TrimSpace(o.ConvID)
	if convID == "" {
		convID = "default"
	}
	g := loadGoal(o.logDir(), convID)
	obj := strings.TrimSpace(it.CorrectedText)
	if obj == "" {
		obj = strings.TrimSpace(it.RawText)
	}
	if obj == "" {
		return g
	}
	if g != nil && g.Status == GoalActive && g.Objective == obj {
		return g // same objective, keep going
	}
	if g != nil && g.Status == GoalCompleted && g.Objective == obj {
		return g
	}
	// New objective (or the old one was paused/blocked and the user re-asks):
	// fresh active goal.
	ng := &Goal{
		Objective:  obj,
		IntentKind: it.Intent,
		Status:     GoalActive,
		Round:      0,
	}
	saveGoal(o.logDir(), convID, ng)
	return ng
}

// syncGoalFromTurn updates the goal after one executed turn:
//   - success (receipt present, no FAILED marker): round++, progress, streak=0
//   - failure: streak++, and blocked once the streak reaches the limit, with a
//     reason and options (never silently dropping the objective)
//   - completed slot: mark the goal completed (evidence-based done)
func (o *Options) syncGoalFromTurn(slotStatus string, receipt string, failed bool, reason string) *Goal {
	convID := strings.TrimSpace(o.ConvID)
	if convID == "" {
		convID = "default"
	}
	g := loadGoal(o.logDir(), convID)
	if g == nil || g.Status != GoalActive {
		return g
	}
	if slotStatus == "done" && !failed {
		g.Status = GoalCompleted
		g.LastProgress = truncateStr(receipt, 200)
		g.NextStep = ""
		g.FailStreak = 0
		saveGoal(o.logDir(), convID, g)
		return g
	}
	if failed {
		g.FailStreak++
		if g.FailStreak >= MaxGoalBlockStreak {
			g.Status = GoalBlocked
			g.BlockedReason = reason
			g.Options = []string{"换一种方式重试", "说明更精确的目标", "暂停这个目标"}
		}
		g.LastProgress = truncateStr(receipt, 200)
		g.NextStep = ""
		saveGoal(o.logDir(), convID, g)
		return g
	}
	g.Round++
	g.FailStreak = 0
	g.LastProgress = truncateStr(receipt, 200)
	saveGoal(o.logDir(), convID, g)
	return g
}

// isExecutableIntent reports whether the intent performs real work worth a
// durable goal (conversational intents ASK/QUERY/NOTE/UNKNOWN do not).
func isExecutableIntent(it contract.Intent) bool {
	switch it.Intent {
	case contract.IntentBuildTest, contract.IntentInstall, contract.IntentEmail,
		contract.IntentEdit, contract.IntentDebug, contract.IntentTest,
		contract.IntentCommit, contract.IntentDeploy, contract.IntentShell,
		contract.IntentFileWrite, contract.IntentRegisterTool, contract.IntentOrchestrate:
		return true
	}
	return false
}

// goalRoundText renders the continuation prompt for one goal round (DeepSeek
// goal-round-driver wording, Chinese): inspect current state, make concrete
// progress, verify, gather evidence before claiming completion.
func goalRoundText(g *Goal) string {
	if g == nil || g.Status != GoalActive {
		return ""
	}
	return "<goal_round>\n" +
		"目标：" + g.Objective + "\n" +
		"轮次：" + strconv.Itoa(g.Round+1) + "\n\n" +
		"继续朝着目标推进（同一会话内）。以当前实际状态（工具结果、任务槽、文件）为准，" +
		"不要假设之前的叙述仍然成立。做出实质进展并验证结果。宣称完成之前，" +
		"先收集证据证明整个目标已达成；若还有工作，保持目标激活进入下一轮。" +
		"\n</goal_round>"
}

// goalStale returns a proactive nudge when an active goal has been untouched
// for longer than GoalStaleMinutes (R9-D5).
func goalStale(g *Goal) string {
	if g == nil || g.Status != GoalActive || g.UpdatedAt == "" {
		return ""
	}
	u, err := time.Parse(time.RFC3339, g.UpdatedAt)
	if err != nil {
		return ""
	}
	if time.Since(u) > GoalStaleMinutes*time.Minute {
		return "上次任务停在：" + g.Objective + "（进展：" + g.LastProgress + "）。要继续吗？"
	}
	return ""
}
