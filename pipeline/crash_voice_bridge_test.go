package pipeline

import (
	"context"
	"strings"
	"testing"

	"voicesign-harness/contract"
)

// seedExternalCrash writes the on-disk state of "an EMAIL step's side effect was
// in flight when the prior process died": an external lease is present and the
// email step is still at the queue head, with a NOTE tail step behind it.
func seedExternalCrash(t *testing.T, conv, logDir string) {
	t.Helper()
	saveGoal(logDir, conv, &Goal{
		Objective: "外部崩溃序列",
		Status:    GoalActive,
		Queue:     []string{emailStep, tailNote},
		StepSeq:   1,
		StepLease: &StepLease{Seq: 1, Text: emailStep, Intent: contract.IntentEmail,
			StartedAt: "2026-10-11T00:00:00Z"},
	})
}

func sawEmailAttempt(recvs []contract.Receipt) bool {
	for _, r := range recvs {
		if r.Tool == "email" {
			return true
		}
	}
	return false
}

// TestCrashedLeaseVoiceBridge: the operator resolves a held, externally
// uncertain crashed step through natural-language turns. Accept never re-sends;
// redo re-attempts exactly once; cancel stops the goal; an unrelated utterance
// re-surfaces the confirm gate without launching any new action.
func TestCrashedLeaseVoiceBridge(t *testing.T) {
	conv := "default"
	ctx := context.Background()

	// --- accept: "已发送，继续吧" must dequeue the email step WITHOUT re-sending,
	// then run the tail note once.
	t.Run("accept_does_not_resend_and_continues", func(t *testing.T) {
		logDir := t.TempDir()
		gEx, gSize, gMtime := globalNotesStat(t)
		o := queueTestOptions(t, conv, logDir)
		defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)
		seedExternalCrash(t, conv, logDir)

		recvs, handled := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
			contract.Intent{Intent: contract.IntentUnknown, CorrectedText: "已发送，继续吧"}, "已发送，继续吧")
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
		if len(g.Queue) != 0 {
			t.Fatalf("accept should finish the chain; queue=%v", g.Queue)
		}
	})

	// --- redo: "重发一遍" re-runs the head email step exactly once (offline it
	// fails fast for lack of AIOPS_KEY, but an email attempt receipt must exist).
	t.Run("redo_reattempts_once", func(t *testing.T) {
		logDir := t.TempDir()
		gEx, gSize, gMtime := globalNotesStat(t)
		o := queueTestOptions(t, conv, logDir)
		defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)
		seedExternalCrash(t, conv, logDir)

		recvs, handled := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
			contract.Intent{Intent: contract.IntentContinue, CorrectedText: "重发一遍"}, "重发一遍")
		if !handled {
			t.Fatal("held external lease must be handled by the voice bridge")
		}
		if !sawEmailAttempt(recvs) {
			t.Fatalf("redo must re-attempt the email step once; recvs=%+v", recvs)
		}
		if n := noteLines(t, logDir); n != 0 {
			t.Fatalf("tail note must not run while the email re-attempt fails; notes=%d", n)
		}
		g := loadGoal(logDir, conv)
		if len(g.Queue) != 2 || !strings.Contains(g.Queue[0], "邮件") {
			t.Fatalf("failed redo must keep the email step parked at head with tail behind; queue=%v", g.Queue)
		}
	})

	// --- cancel: an explicit cancel stops the goal and fires nothing.
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
		if g.Status != GoalCanceled {
			t.Fatalf("goal must be canceled; status=%s", g.Status)
		}
		if g.StepLease != nil {
			t.Fatalf("cancel must clear the lease; lease=%+v", g.StepLease)
		}
	})

	// --- hold: an unrelated utterance re-surfaces the gate, starts nothing new,
	// and preserves the lease so a later turn can still decide.
	t.Run("unrelated_utterance_reholds", func(t *testing.T) {
		logDir := t.TempDir()
		gEx, gSize, gMtime := globalNotesStat(t)
		o := queueTestOptions(t, conv, logDir)
		defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)
		seedExternalCrash(t, conv, logDir)

		recvs, handled := o.resolveCrashedLeaseTurn(ctx, logDir, conv,
			contract.Intent{Intent: contract.IntentNote, CorrectedText: "记一下 今天天气不错"}, "记一下 今天天气不错")
		if !handled {
			t.Fatal("held external lease must intercept the turn")
		}
		if !hasConfirmAsk(recvs) {
			t.Fatalf("unrelated turn must re-surface need_confirm; recvs=%+v", recvs)
		}
		if sawEmailAttempt(recvs) {
			t.Fatalf("unrelated turn must not re-send; recvs=%+v", recvs)
		}
		if n := noteLines(t, logDir); n != 0 {
			t.Fatalf("unrelated turn must not run the new NOTE either; notes=%d", n)
		}
		g := loadGoal(logDir, conv)
		if g.StepLease == nil {
			t.Fatal("lease must be preserved while awaiting a decision")
		}
		if len(g.Queue) != 2 {
			t.Fatalf("queue must be untouched while held; queue=%v", g.Queue)
		}
	})
}
