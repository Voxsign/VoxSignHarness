package pipeline

import (
	"context"
	"strings"
	"testing"

	"voicesign-harness/contract"
)

// seedExternalCrash writes the on-disk state of "an EMAIL step's side effect was
// in flight when the prior process died": an external lease with stable
// (GoalID,StepID,Attempt) is present and the email step is still at the queue
// head, with a NOTE tail step behind it.
func seedExternalCrash(t *testing.T, conv, logDir string) {
	t.Helper()
	saveGoal(logDir, conv, &Goal{
		Objective: "外部崩溃序列", Status: GoalActive, GoalID: "ge", StepCounter: 2,
		Queue:     []string{emailStep, tailNote},
		QueueIDs:  []string{"ge.s1", "ge.s2"},
		HeadAttempt: 1, StepSeq: 1,
		StepLease: &StepLease{GoalID: "ge", StepID: "ge.s1", Attempt: 1, Seq: 1,
			Text: emailStep, Intent: contract.IntentEmail, StartedAt: "2026-10-11T00:00:00Z"},
	})
}

func sawEmailAttempt(recvs []contract.Receipt) bool {
	return countEmailAttempts(recvs) > 0
}

func countEmailAttempts(recvs []contract.Receipt) int {
	n := 0
	for _, r := range recvs {
		if r.Tool == "email" {
			n++
		}
	}
	return n
}

// TestCrashedLeaseVoiceBridge (round 4): natural-language resolution of a held,
// externally uncertain crashed step. A plain "继续/执行/立刻" never re-sends;
// re-send requires an explicit request AND a second explicit risk confirmation;
// accept requires an explicit human attestation (recorded) and never re-sends;
// vague words like "跳过/搞定/完成了" do not count as accept.
func TestCrashedLeaseVoiceBridge(t *testing.T) {
	conv := "default"
	ctx := context.Background()

	// accept: an explicit human attestation (even with "继续吧" appended) dequeues
	// WITHOUT re-sending, records the attestation, and runs the tail note once.
	t.Run("accept_attests_and_never_resends", func(t *testing.T) {
		logDir := t.TempDir()
		gEx, gSize, gMtime := globalNotesStat(t)
		o := queueTestOptions(t, conv, logDir)
		defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)
		seedExternalCrash(t, conv, logDir)

		recvs, handled := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
			contract.Intent{Intent: contract.IntentUnknown, CorrectedText: "我确认已经手动发送了，继续吧"},
			"我确认已经手动发送了，继续吧")
		if !handled {
			t.Fatal("held external lease must be handled by the voice bridge")
		}
		if sawEmailAttempt(recvs) {
			t.Fatalf("accept must NOT re-send the email; recvs=%+v", recvs)
		}
		if n := noteLines(t, logDir); n != 1 {
			t.Fatalf("accept should run the tail note exactly once; notes=%d", n)
		}
		g := loadGoal(logDir, conv)
		if g.StepLease != nil {
			t.Fatalf("accept must clear the lease; lease=%+v", g.StepLease)
		}
		attested := false
		for _, s := range g.Steps {
			if strings.Contains(s, "human-verified") {
				attested = true
			}
		}
		if !attested {
			t.Fatalf("accept must leave a human-verified attestation; steps=%v", g.Steps)
		}
		if len(g.Queue) != 0 {
			t.Fatalf("accept should finish the chain; queue=%v", g.Queue)
		}
	})

	// redo is a TWO-turn gate: the first request only opens the risk gate (zero
	// sends); the explicit confirmation then re-runs the head exactly once.
	t.Run("redo_requires_explicit_risk_confirmation", func(t *testing.T) {
		logDir := t.TempDir()
		gEx, gSize, gMtime := globalNotesStat(t)
		o := queueTestOptions(t, conv, logDir)
		defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)
		seedExternalCrash(t, conv, logDir)

		// Turn 1: "重发一遍" -> risk gate, nothing sent.
		r1, h1 := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
			contract.Intent{Intent: contract.IntentContinue, CorrectedText: "重发一遍"}, "重发一遍")
		if !h1 || !hasConfirmAsk(r1) {
			t.Fatalf("redo request must open a confirmation gate; recvs=%+v", r1)
		}
		if sawEmailAttempt(r1) {
			t.Fatalf("redo request must NOT send before confirmation; recvs=%+v", r1)
		}
		g1 := loadGoal(logDir, conv)
		if g1.LeaseDecision != "redo_requested" || g1.StepLease == nil {
			t.Fatalf("after request, decision must be redo_requested with lease held; g=%+v", g1)
		}

		// A plain "继续" while the risk gate is pending must STILL not send.
		rMid, _ := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
			contract.Intent{Intent: contract.IntentContinue, CorrectedText: "继续"}, "继续")
		if sawEmailAttempt(rMid) {
			t.Fatalf("plain 继续 must not confirm a re-send; recvs=%+v", rMid)
		}
		if !hasConfirmAsk(rMid) {
			t.Fatalf("plain 继续 must re-show the risk gate; recvs=%+v", rMid)
		}

		// Turn 2: explicit "确认重发" -> one re-attempt (offline it fails fast).
		r2, h2 := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
			contract.Intent{Intent: contract.IntentContinue, CorrectedText: "确认重发"}, "确认重发")
		if !h2 {
			t.Fatal("confirmation must be handled by the voice bridge")
		}
		if n := countEmailAttempts(r2); n != 1 {
			t.Fatalf("confirmed redo must attempt the email exactly once, got %d; recvs=%+v", n, r2)
		}
		if n := noteLines(t, logDir); n != 0 {
			t.Fatalf("tail note must not run while the email re-attempt fails; notes=%d", n)
		}
		g2 := loadGoal(logDir, conv)
		if len(g2.Queue) != 2 || !strings.Contains(g2.Queue[0], "邮件") {
			t.Fatalf("failed confirmed redo must keep the email step parked at head; queue=%v", g2.Queue)
		}
	})

	// Plain go-ahead words with no redo request must never re-send.
	t.Run("plain_go_ahead_never_resends", func(t *testing.T) {
		for _, say := range []string{"继续", "继续吧", "立刻执行", "马上", "continue", "go ahead"} {
			logDir := t.TempDir()
			o := queueTestOptions(t, conv, logDir)
			seedExternalCrash(t, conv, logDir)
			recvs, handled := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
				contract.Intent{Intent: contract.IntentContinue, CorrectedText: say}, say)
			if !handled {
				t.Fatalf("%q must be intercepted by the bridge", say)
			}
			if sawEmailAttempt(recvs) {
				t.Fatalf("%q must never re-send; recvs=%+v", say, recvs)
			}
			if !hasConfirmAsk(recvs) {
				t.Fatalf("%q must re-surface the unknown gate; recvs=%+v", say, recvs)
			}
			g := loadGoal(logDir, conv)
			if g.StepLease == nil || g.LeaseDecision != "" {
				t.Fatalf("%q must keep the lease held with no decision; g=%+v", say, g)
			}
		}
	})

	// Vague completion words (and negated claims) do NOT prove the email was sent.
	t.Run("vague_words_do_not_accept", func(t *testing.T) {
		for _, say := range []string{"跳过", "搞定了", "完成了", "收到了", "还没手动发送", "skip", "done"} {
			logDir := t.TempDir()
			o := queueTestOptions(t, conv, logDir)
			seedExternalCrash(t, conv, logDir)
			recvs, handled := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
				contract.Intent{Intent: contract.IntentUnknown, CorrectedText: say}, say)
			if !handled {
				t.Fatalf("%q must be intercepted", say)
			}
			if sawEmailAttempt(recvs) {
				t.Fatalf("%q must not re-send; recvs=%+v", say, recvs)
			}
			g := loadGoal(logDir, conv)
			if g.StepLease == nil {
				t.Fatalf("%q must not be treated as accept: lease cleared", say)
			}
		}
	})

	// cancel stops the goal and fires nothing.
	t.Run("cancel_stops_goal", func(t *testing.T) {
		logDir := t.TempDir()
		gEx, gSize, gMtime := globalNotesStat(t)
		o := queueTestOptions(t, conv, logDir)
		defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)
		seedExternalCrash(t, conv, logDir)

		recvs, handled := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
			contract.Intent{Intent: contract.IntentCancel, CorrectedText: "算了，别发了"}, "算了，别发了")
		if !handled {
			t.Fatal("cancel must be handled by the voice bridge")
		}
		if sawEmailAttempt(recvs) {
			t.Fatalf("cancel must not attempt the email; recvs=%+v", recvs)
		}
		if n := noteLines(t, logDir); n != 0 {
			t.Fatalf("cancel must run no tail note; notes=%d", n)
		}
		g := loadGoal(logDir, conv)
		if g.Status != GoalCanceled || g.StepLease != nil {
			t.Fatalf("goal must be canceled with lease cleared; status=%s lease=%+v", g.Status, g.StepLease)
		}
	})

	// An unrelated utterance re-surfaces the gate, starts nothing new, and keeps
	// the lease so a later turn can still decide.
	t.Run("unrelated_utterance_reholds", func(t *testing.T) {
		logDir := t.TempDir()
		gEx, gSize, gMtime := globalNotesStat(t)
		o := queueTestOptions(t, conv, logDir)
		defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)
		seedExternalCrash(t, conv, logDir)

		recvs, handled := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
			contract.Intent{Intent: contract.IntentNote, CorrectedText: "记一下 今天天气不错"}, "记一下 今天天气不错")
		if !handled || !hasConfirmAsk(recvs) {
			t.Fatalf("unrelated turn must re-surface need_confirm; recvs=%+v", recvs)
		}
		if sawEmailAttempt(recvs) {
			t.Fatalf("unrelated turn must not re-send; recvs=%+v", recvs)
		}
		if n := noteLines(t, logDir); n != 0 {
			t.Fatalf("unrelated turn must not run the new NOTE; notes=%d", n)
		}
		g := loadGoal(logDir, conv)
		if g.StepLease == nil || len(g.Queue) != 2 {
			t.Fatalf("lease/queue must be preserved while held; g=%+v", g)
		}
	})
}
