package pipeline

import (
	"strconv"

	"voicesign-harness/contract"
	"voicesign-harness/verify"
)

// Business (objective-level) terminal states, distinct from the per-turn HTTP
// lifecycle. A single HTTP turn can be handled (request finished) while the
// business objective is still waiting on confirmation, has a failed external
// action, or has queued steps left. Those cases must NEVER be reported as
// business "done" — the #1 structural false-success mode.
//
// This file is the single authority mapping final receipts / independent verify
// result / durable goal to the honest objective state. The server's HTTP status
// is derived from it instead of "the turn returned without a question".
const (
	// BizDone: evidence shows the whole objective is achieved this turn.
	BizDone = "done"
	// BizNeedConfirm: blocked on an explicit user confirmation gate (install,
	// email reply/forward draft). An honest wait, not a failure and not done.
	BizNeedConfirm = "need_confirm"
	// BizFailed: an external action/tool failed or independent verify failed.
	BizFailed = "failed"
	// BizBlocked: the durable goal hit the repeated-failure streak limit.
	BizBlocked = "blocked"
	// BizInProgress: the turn ended but the objective still has queued/remaining
	// work (multi-step chain not yet drained).
	BizInProgress = "in_progress"
	// BizCanceled: the user explicitly canceled the objective.
	BizCanceled = "canceled"
)

// BusinessState derives the OBJECTIVE-level state from the final receipts, the
// independent verify result and the durable goal. It is a pure function (no
// network/mail/fs), so the "never claim done prematurely" rule is unit-testable.
//
// Precedence, highest first:
//
//	cancel > confirmation gate > failure / verify-fail > blocked goal >
//	queued work remaining > done.
//
// TDD: implemented to satisfy the red suite; precedence is enforced top-down.
func BusinessState(it contract.Intent, rs []contract.Receipt, vr verify.Result, g *Goal) (state, reason string) {
	if it.Intent == contract.IntentCancel {
		return BizCanceled, ""
	}
	// A confirmation gate is an honest "waiting for the user" — not a failure and
	// never done, even though its receipt carries OK=false. It outranks a
	// co-occurring failure so the user is asked to confirm rather than seeing FAILED.
	if hasConfirmAsk(rs) {
		return BizNeedConfirm, firstErr(rs)
	}
	if hasFailure(rs) {
		return BizFailed, firstErr(rs)
	}
	if vr.Status == verify.StatusFail {
		return BizFailed, "独立校验未通过：" + vr.Detail
	}
	if g != nil {
		switch g.Status {
		case GoalBlocked:
			return BizBlocked, g.BlockedReason
		case GoalCanceled:
			return BizCanceled, ""
		case GoalActive, GoalPaused:
			// A finished turn is not a finished objective if steps remain queued.
			if len(g.Queue) > 0 {
				return BizInProgress, "还有 " + strconv.Itoa(len(g.Queue)) + " 个排队步骤未执行"
			}
		}
	}
	return BizDone, ""
}
