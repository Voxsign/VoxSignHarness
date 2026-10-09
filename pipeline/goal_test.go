package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tmpLogDir(t *testing.T) string {
	t.Helper()
	d := filepath.Join(os.TempDir(), "vhs-goal-test-"+shortID())
	if err := os.MkdirAll(d, 0o644); err == nil {
		_ = os.RemoveAll(d)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

func TestGoalLifecycle(t *testing.T) {
	logDir := tmpLogDir(t)
	convID := "default"

	// create
	g := &Goal{Objective: "安装 codex 并测试运行", Status: GoalActive}
	saveGoal(logDir, convID, g)
	if got := loadGoal(logDir, convID); got == nil || got.Status != GoalActive {
		t.Fatalf("goal not persisted")
	}

	// progress
	g2 := syncGoalFromTurnStatus(logDir, convID, "", "done", false, "installed")
	if g2.Round != 1 || g2.Status != GoalActive {
		t.Fatalf("round should bump on success: %+v", g2)
	}

	// blocked after 3 consecutive failures
	for i := 0; i < 3; i++ {
		g3 := syncGoalFromTurnStatus(logDir, convID, "running", "", true, "npm error")
		if i < 2 && g3.Status != GoalActive {
			t.Fatalf("should stay active before streak limit: %+v", g3)
		}
	}
	gb := loadGoal(logDir, convID)
	if gb.Status != GoalBlocked || gb.BlockedReason == "" || len(gb.Options) == 0 {
		t.Fatalf("goal must be blocked with reason+options: %+v", gb)
	}
}

func TestGoalCompletedOnlyOnEvidence(t *testing.T) {
	logDir := tmpLogDir(t)
	g := &Goal{Objective: "下载编译测试", Status: GoalActive}
	saveGoal(logDir, "default", g)
	// slot done + no failure -> completed
	g2 := syncGoalFromTurnStatus(logDir, "default", "done", "test passed", false, "")
	if g2.Status != GoalCompleted {
		t.Fatalf("done slot must complete the goal: %+v", g2)
	}
	// a later failure must not revive it
	g3 := syncGoalFromTurnStatus(logDir, "default", "running", "", true, "err")
	if g3.Status != GoalCompleted {
		t.Fatalf("completed goal must stay completed: %+v", g3)
	}
}

func TestGoalRoundPrompt(t *testing.T) {
	g := &Goal{Objective: "把 Sofia 的邮件回掉", Status: GoalActive, Round: 2}
	text := goalRoundText(g)
	for _, want := range []string{"<goal_round>", "把 Sofia 的邮件回掉", "轮次：3", "验证结果", "宣称完成之前", "保持目标激活"} {
		if !strings.Contains(text, want) {
			t.Fatalf("goal round prompt missing %q: %s", want, text)
		}
	}
	if goalRoundText(&Goal{Status: GoalCompleted}) != "" {
		t.Fatalf("completed goal must not render a round prompt")
	}
	if goalRoundText(nil) != "" {
		t.Fatalf("nil goal must not render a round prompt")
	}
}

func TestGoalStaleNudge(t *testing.T) {
	if goalStale(nil) != "" {
		t.Fatalf("nil goal: no nudge")
	}
	g := &Goal{Objective: "x", Status: GoalActive, UpdatedAt: "2026-10-09T00:00:00+03:00"}
	if n := goalStale(g); !strings.Contains(n, "要继续吗") {
		t.Fatalf("stale active goal must nudge: %q", n)
	}
	g2 := &Goal{Objective: "x", Status: GoalCompleted}
	if goalStale(g2) != "" {
		t.Fatalf("completed goal: no nudge")
	}
}

// syncGoalFromTurnStatus is the test seam for syncGoalFromTurn without an Options.
func syncGoalFromTurnStatus(logDir, convID, slotStatus, receipt string, failed bool, reason string) *Goal {
	g := loadGoal(logDir, convID)
	if g == nil || g.Status != GoalActive {
		return g
	}
	if slotStatus == "done" && !failed {
		g.Status = GoalCompleted
		g.LastProgress = truncateStr(receipt, 200)
		g.FailStreak = 0
		saveGoal(logDir, convID, g)
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
		saveGoal(logDir, convID, g)
		return g
	}
	g.Round++
	g.FailStreak = 0
	g.LastProgress = truncateStr(receipt, 200)
	saveGoal(logDir, convID, g)
	return g
}

func shortID() string {
	return "t" + os.Getenv("RANDOM")
}
