package pipeline

import (
	"context"
	"testing"

	"voicesign-harness/contract"
)

// P0 round 5 — crash-recovery classification must be a PROVEN read-only
// whitelist, not an intent-label guess. Unknown and anything that can build,
// test, debug, launch a process, or change state is held after a crash
// (external_uncertain) and never auto-replayed; only provably read-only /
// conversational intents re-run, and local NOTE/REMINDER reconcile from disk.

func TestLeaseRecoveryClass_Whitelist(t *testing.T) {
	readOnly := []string{
		contract.IntentQuery, contract.IntentAsk, contract.IntentInfo,
		contract.IntentFileRead, contract.IntentFileList, contract.IntentTime,
	}
	for _, in := range readOnly {
		if got := leaseRecoveryClass(in); got != recoveryIdempotent {
			t.Errorf("read-only intent %s classified %s, want %s", in, got, recoveryIdempotent)
		}
	}
	for _, in := range []string{contract.IntentNote, contract.IntentReminder} {
		if got := leaseRecoveryClass(in); got != recoveryLocalNote {
			t.Errorf("local intent %s classified %s, want %s", in, got, recoveryLocalNote)
		}
	}
	// Everything that is unknown or can have side effects must be held.
	held := []string{
		contract.IntentUnknown, contract.IntentTest, contract.IntentBuildTest,
		contract.IntentDebug, contract.IntentAppLaunch,
		contract.IntentEmail, contract.IntentInstall, contract.IntentEdit,
		contract.IntentDeploy, contract.IntentCommit, contract.IntentSequence,
		contract.IntentRegisterTool, contract.IntentOrchestrate, contract.IntentSelfImprove,
	}
	for _, in := range held {
		if got := leaseRecoveryClass(in); got != recoveryExternal {
			t.Errorf("side-effecting/unknown intent %s classified %s, want %s (must hold, never auto-replay)",
				in, got, recoveryExternal)
		}
	}
}

// seedLeaseWithIntent writes a crashed in-flight queue whose head carries the
// given intent, with a NOTE tail behind it.
func seedLeaseWithIntent(t *testing.T, conv, logDir, goalID, headText, intent string) {
	t.Helper()
	saveGoal(logDir, conv, &Goal{
		Objective: "崩溃分类 " + intent, Status: GoalActive, GoalID: goalID, StepCounter: 2,
		Queue:      []string{headText, tailNote},
		QueueIDs:   []string{goalID + ".s1", goalID + ".s2"},
		HeadAttempt: 1, StepSeq: 1,
		StepLease: &StepLease{GoalID: goalID, StepID: goalID + ".s1", Attempt: 1, Seq: 1,
			Text: headText, Intent: intent, StartedAt: "2026-10-11T00:00:00Z"},
	})
}

// A crash on an UNKNOWN or side-effecting head must hold: ask for confirmation,
// keep the lease and queue head untouched, bump no attempt, run no side effect.
func TestCrashedSideEffectingIntents_Held(t *testing.T) {
	ctx := context.Background()
	conv := "default"
	cases := map[string]string{
		contract.IntentUnknown:   "随便帮我处理点事情",
		contract.IntentTest:      "跑一下测试",
		contract.IntentBuildTest: "从github下载代码编译测试",
		contract.IntentDebug:     "调试一下这个程序",
		contract.IntentAppLaunch: "打开计算器",
	}
	for intent, text := range cases {
		intent, text := intent, text
		t.Run(intent, func(t *testing.T) {
			logDir := t.TempDir()
			gEx, gSize, gMtime := globalNotesStat(t)
			o := queueTestOptions(t, conv, logDir)
			defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)
			seedLeaseWithIntent(t, conv, logDir, "g-"+intent, text, intent)

			recvs := o.advanceGoalQueue(ctx, logDir, nil)
			if !hasConfirmAsk(recvs) {
				t.Fatalf("%s crash must be held with a confirmation ask, got %+v", intent, recvs)
			}
			g := loadGoal(logDir, conv)
			if g.StepLease == nil {
				t.Fatalf("%s crash lease must be retained while held", intent)
			}
			if g.HeadAttempt != 1 {
				t.Fatalf("%s held lease must not bump attempt; HeadAttempt=%d", intent, g.HeadAttempt)
			}
			if len(g.Queue) != 2 || g.Queue[0] != text {
				t.Fatalf("%s head must stay parked at the queue front; queue=%v", intent, g.Queue)
			}
			if noteLines(t, logDir) != 0 {
				t.Fatalf("%s must not run any side effect (incl. the tail note) while held", intent)
			}
		})
	}
}

// A crash on a PROVEN read-only head is treated as idempotent: it is NOT held by
// the crash confirmation gate and the attempt is bumped so the head re-runs.
// (Whether the offline placeholder head succeeds is out of scope; the assertion
// is that it is never blocked as an uncertain/irreversible action.)
func TestCrashedReadOnlyIntents_NotHeld(t *testing.T) {
	ctx := context.Background()
	conv := "default"
	cases := map[string]string{
		contract.IntentQuery:    "解释一下什么是闭包",
		contract.IntentAsk:      "问一下这个概念是什么意思",
		contract.IntentInfo:     "介绍一下这个东西",
		contract.IntentFileRead: "读一下 README 文件",
		contract.IntentFileList: "列一下当前目录",
		contract.IntentTime:     "现在几点了",
	}
	for intent, text := range cases {
		intent, text := intent, text
		t.Run(intent, func(t *testing.T) {
			logDir := t.TempDir()
			gEx, gSize, gMtime := globalNotesStat(t)
			o := queueTestOptions(t, conv, logDir)
			defer assertGlobalNotesUnchanged(t, gEx, gSize, gMtime)
			seedLeaseWithIntent(t, conv, logDir, "gr-"+intent, text, intent)

			recvs := o.advanceGoalQueue(ctx, logDir, nil)
			if hasConfirmAsk(recvs) {
				t.Fatalf("%s is read-only and must NOT be held by the crash gate; %+v", intent, recvs)
			}
			g := loadGoal(logDir, conv)
			// The original crashed lease (attempt 1) must have been cleared for a
			// re-run, never left held as-is. After that the read-only head either
			// completed and popped (lease nil) or re-ran/parked under a NEW attempt
			// (>=2); retaining the attempt-1 lease is the held signature.
			if g.StepLease != nil && g.StepLease.Attempt == 1 {
				t.Fatalf("%s original crash lease was held instead of re-run; lease=%+v queue=%v",
					intent, g.StepLease, g.Queue)
			}
		})
	}
}
