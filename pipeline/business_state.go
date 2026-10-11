package pipeline

import (
	"strconv"

	"voicesign-harness/contract"
	"voicesign-harness/verify"
)

// Business objective states reported to clients. These describe whether the
// OBJECTIVE is achieved, not whether the single HTTP/voice turn merely returned.
const (
	BizDone        = "done"         // objective achieved with positive evidence
	BizNeedConfirm = "need_confirm" // blocked on an explicit human confirmation gate
	BizNeedAsk     = "need_ask"     // blocked on a clarification question
	BizFailed      = "failed"       // an action or independent verification failed
	BizBlocked     = "blocked"      // repeated failures parked the durable goal
	BizInProgress  = "in_progress"  // turn ended but the objective is unproven / work remains
	BizCanceled    = "canceled"     // the user explicitly canceled
)

// nonEvidenceTools names receipts that only describe orchestration or a pending
// handoff to the model tool loop; they prove no completed side effect and must
// never count as success evidence for settling an executable objective.
var nonEvidenceTools = map[string]bool{
	"sequence": true, // "按顺序执行…" planning / auto-continue note
	"pipeline": true, // "M2 passed… action pending model tool loop"
}

// confirmQuestion returns the human-facing question carried by a confirmation
// gate receipt (hasConfirmAsk/firstErr/hasFailure live in pipeline.go).
func confirmQuestion(rs []contract.Receipt) string {
	for _, r := range rs {
		if r.ConfirmAsk && r.Err != "" {
			return r.Err
		}
	}
	return "该操作需要你确认后才会执行，回复「确认」继续。"
}

func hasCancel(rs []contract.Receipt) bool {
	for _, r := range rs {
		if r.OK && r.Tool == "cancel" {
			return true
		}
	}
	return false
}

// hasRealActionSuccess reports whether at least one receipt proves a real,
// completed side effect: an OK tool receipt that is not an orchestration/pending
// note and was not safety-blocked. Independent verify.Pass is handled separately
// in BusinessState. This is what separates "the tool said it did it" from the
// planning notes that otherwise masquerade as success.
func hasRealActionSuccess(rs []contract.Receipt) bool {
	for _, r := range rs {
		if r.OK && r.Blocked == "" && !nonEvidenceTools[r.Tool] {
			return true
		}
	}
	return false
}

// BusinessState derives the OBJECTIVE-level business state for a turn from the
// final intent, action receipts, independent verification result and the
// durable goal. It is a PURE function (no I/O) so the rule is unit-testable.
//
// Precedence (highest first):
//
//	cancel > confirmation-gate > action failure / verify-fail > blocked goal
//	       > queued work remaining / no success evidence (in_progress) > done
//
// Critical rule (P0, 2026-10-11): the HTTP turn finishing without a clarification
// question is NOT the same as the objective being achieved. An executable or
// sequence turn may only settle as done when there is positive evidence —
// independent verification passed, or a real side-effect action reported OK.
// Empty receipts, a pure orchestration/pending note, or an unverifiable result
// with no action mean the objective is still in_progress. Conversational turns
// (NOTE/QUERY/ASK/…) complete on their own answer and are never gated by an
// unrelated long-lived goal that happens to be active.
func BusinessState(it contract.Intent, rs []contract.Receipt, vr verify.Result, g *Goal) (state, reason string) {
	switch {
	case hasCancel(rs):
		return BizCanceled, "用户已取消，停止后续动作（已完成的副作用不自动回滚）"
	case hasConfirmAsk(rs):
		return BizNeedConfirm, confirmQuestion(rs)
	case hasFailure(rs):
		return BizFailed, "动作未成功：" + firstErr(rs)
	case vr.Status == verify.StatusFail:
		detail := vr.Detail
		if detail == "" {
			detail = "独立验证未通过"
		}
		return BizFailed, "验证失败：" + detail
	}

	if g != nil {
		switch g.Status {
		case GoalBlocked:
			reason := g.BlockedReason
			if reason == "" {
				reason = "目标连续失败，已挂起"
			}
			return BizBlocked, reason
		case GoalCanceled:
			return BizCanceled, "目标已取消"
		}
	}

	// Only a turn that actually advances an executable/sequence objective can
	// settle that objective. Conversational turns finish on their own answer.
	advancing := isExecutableIntent(it) || it.Intent == contract.IntentSequence
	if !advancing {
		return BizDone, ""
	}

	// Work still queued on an active/paused goal -> the objective is in progress.
	if g != nil && (g.Status == GoalActive || g.Status == GoalPaused) && len(g.Queue) > 0 {
		return BizInProgress, "目标还有 " + strconv.Itoa(len(g.Queue)) + " 个排队步骤未完成"
	}

	// No positive success evidence -> the objective is unproven, never done.
	if vr.Status != verify.StatusPass && !hasRealActionSuccess(rs) {
		return BizInProgress, "未取得可证实的动作成功证据，目标尚未完成"
	}

	return BizDone, ""
}
