package pipeline

import (
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/verify"
)

// TestBusinessStateNeverFalseDone locks the P0 rule (Codex supervision 2026-10-11):
// failure / confirmation-pending / undrained sequence must never be reported as
// business done, even though the HTTP turn itself has finished.
func TestBusinessStateNeverFalseDone(t *testing.T) {
	ok := contract.Receipt{Tool: "note", OK: true, Stdout: "已记录"}
	fail := contract.Receipt{Tool: "email", OK: false, Err: "取邮件失败：AIOPS_KEY 未配置"}
	gate := contract.Receipt{Tool: "install", OK: false, ConfirmAsk: true,
		Err: "准备安装 codex（npm 全局安装）。确认后我立刻执行——回复「确认」"}

	cases := []struct {
		name string
		it   contract.Intent
		rs   []contract.Receipt
		vr   verify.Result
		g    *Goal
		want string
	}{
		{
			name: "R-a external action FAILED must not be done",
			it:   contract.Intent{Intent: contract.IntentEmail},
			rs:   []contract.Receipt{fail},
			vr:   verify.Result{Status: verify.StatusUnverifiable},
			g:    &Goal{Status: GoalActive},
			want: BizFailed,
		},
		{
			name: "R-a one failing receipt among OK steps still fails the objective",
			it:   contract.Intent{Intent: contract.IntentEmail},
			rs:   []contract.Receipt{ok, fail},
			vr:   verify.Result{Status: verify.StatusUnverifiable},
			g:    &Goal{Status: GoalActive},
			want: BizFailed,
		},
		{
			name: "R-b install confirmation gate is need_confirm, not done and not failed",
			it:   contract.Intent{Intent: contract.IntentInstall},
			rs:   []contract.Receipt{gate},
			vr:   verify.Result{Status: verify.StatusUnverifiable},
			g:    &Goal{Status: GoalActive},
			want: BizNeedConfirm,
		},
		{
			name: "R-b confirmation gate outranks a co-occurring failure",
			it:   contract.Intent{Intent: contract.IntentInstall},
			rs:   []contract.Receipt{fail, gate},
			vr:   verify.Result{Status: verify.StatusUnverifiable},
			g:    &Goal{Status: GoalActive},
			want: BizNeedConfirm,
		},
		{
			name: "R-c active goal with queued steps is in_progress, never done",
			it:   contract.Intent{Intent: contract.IntentSequence},
			rs:   []contract.Receipt{ok},
			vr:   verify.Result{Status: verify.StatusPass},
			g:    &Goal{Status: GoalActive, Queue: []string{"第二步：记一条明天开会"}},
			want: BizInProgress,
		},
		{
			name: "R-c drained queue + all OK is genuinely done",
			it:   contract.Intent{Intent: contract.IntentNote},
			rs:   []contract.Receipt{ok},
			vr:   verify.Result{Status: verify.StatusPass},
			g:    &Goal{Status: GoalCompleted},
			want: BizDone,
		},
		{
			name: "R-a independent verify failure must not be done",
			it:   contract.Intent{Intent: contract.IntentEdit},
			rs:   []contract.Receipt{ok},
			vr:   verify.Result{Status: verify.StatusFail, Detail: "目标文本不存在"},
			g:    &Goal{Status: GoalActive},
			want: BizFailed,
		},
		{
			name: "repeated-failure blocked goal is blocked",
			it:   contract.Intent{Intent: contract.IntentEmail},
			rs:   []contract.Receipt{ok},
			vr:   verify.Result{Status: verify.StatusUnverifiable},
			g:    &Goal{Status: GoalBlocked, BlockedReason: "连续 3 次取邮件失败"},
			want: BizBlocked,
		},
		{
			name: "R-S2 explicit cancel is canceled (does not auto-undo completed work)",
			it:   contract.Intent{Intent: contract.IntentCancel},
			rs:   []contract.Receipt{ok},
			vr:   verify.Result{Status: verify.StatusUnverifiable},
			g:    &Goal{Status: GoalCanceled},
			want: BizCanceled,
		},
		{
			name: "failure outranks undrained queue (no masking by in_progress)",
			it:   contract.Intent{Intent: contract.IntentSequence},
			rs:   []contract.Receipt{fail},
			vr:   verify.Result{Status: verify.StatusUnverifiable},
			g:    &Goal{Status: GoalActive, Queue: []string{"第二步"}},
			want: BizFailed,
		},
		{
			name: "confirmation outranks undrained queue",
			it:   contract.Intent{Intent: contract.IntentSequence},
			rs:   []contract.Receipt{gate},
			vr:   verify.Result{Status: verify.StatusUnverifiable},
			g:    &Goal{Status: GoalActive, Queue: []string{"第二步"}},
			want: BizNeedConfirm,
		},
		{
			name: "plain conversational success with no goal is done",
			it:   contract.Intent{Intent: contract.IntentNote},
			rs:   []contract.Receipt{ok},
			vr:   verify.Result{Status: verify.StatusUnverifiable},
			g:    nil,
			want: BizDone,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := BusinessState(c.it, c.rs, c.vr, c.g)
			if got != c.want {
				t.Fatalf("BusinessState = %q (reason=%q), want %q", got, reason, c.want)
			}
		})
	}
}
