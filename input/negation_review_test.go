//      **  patchfill**  (   G1 ·        ). 
//
//    P1/P2 referout  class  , basefile    : 
//
//	P1  recv recv:  /  /  /noneed/ again/ , byand "diff"after   /manage/ / 
//	   --    , "  delete"  be become   delete, G1 then   . 
//	P2   (safesafetyside): need need/ need    " need" is  
//	   --     pos refer  empty outconfirm, pipe"  delete"changebecome"confirm    ". 
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestNegationCoversReviewP1Markers    P1: patch after   word  alloccur . 
// R11 (2026-10-09): "别管那个删除操作" is an explicit stop → CANCEL (real cancel,
// no confirm loop); the remaining P1 markers stay negation-with-ask.
func TestNegationCoversReviewP1Markers(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []string{
		"请勿删除那个文件",
		"勿删",
		"无需提交",
		"不再部署到服务器",
		"切勿修改配置",
		"提交就免了",
		"别去删除那个文件",
	}
	for _, text := range cases {
		got := c.ClassifyTask(text)
		if got.Conflict != contract.ConflictNegation {
			t.Errorf("P1 未闭合: %q 未被识别为否定（conflict=%q intent=%s）",
				text, got.Conflict, got.Intent)
		}
		if got.Ask == "" {
			t.Errorf("%q: 否定句必须回问（Ask != '' 才保证绝不执行）", text)
		}
		if got.Intent == contract.IntentEdit && got.Params["action"] == "delete" {
			t.Errorf("%q 仍被判为可执行删除 —— G1 未闭合", text)
		}
	}
	// R11: explicit stop words cancel the current task instead of asking again.
	if got := c.ClassifyTask("别管那个删除操作"); got.Intent != contract.IntentCancel {
		t.Errorf("%q 应判 CANCEL（停止当前操作），实际 intent=%s conflict=%q",
			"别管那个删除操作", got.Intent, got.Conflict)
	}
}

// TestNegationReviewP2FalsePositives    P2:     ,     . 
func TestNegationReviewP2FalsePositives(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct{ text, why string }{
		{"要不要删除那个文件", "「不要」只是「要不要」的一部分"},
		{"不要紧，帮我删除它", "「不要紧」= 没关系，整句是请我删除"},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Conflict == contract.ConflictNegation {
			t.Errorf("P2 误伤: %q 被误判否定（%s）→ intent=%s",
				tc.text, tc.why, got.Intent)
		}
	}
	// " need ,   delete "   bydeletehandle( isuseuser    require)
	got := c.ClassifyTask("不要紧，帮我删除它")
	if got.Intent != contract.IntentEdit {
		t.Errorf("「不要紧，帮我删除它」意图 = %q，期望 EDIT（用户确实要求删除）", got.Intent)
	}

	//    hasNegation  face
	for _, text := range []string{"要不要删除", "不要紧"} {
		if _, _, hit := hasNegation(text); hit {
			t.Errorf("hasNegation(%q) 应为 false（形似否定）", text)
		}
	}
	for _, text := range []string{"请勿删除", "无需提交", "不再部署", "免了", "别去删除"} {
		if _, _, hit := hasNegation(text); !hit {
			t.Errorf("hasNegation(%q) 应为 true（P1 补词）", text)
		}
	}
}

// TestNegationMarkerPrecedence  word first:  needagain /  needneed   be  need  firstback . 
func TestNegationMarkerPrecedence(t *testing.T) {
	if mk, _, _ := hasNegation("不要再部署"); mk != "不要再" {
		t.Errorf("回显 = %q，期望「不要再」（长词优先，评审 P6）", mk)
	}
	if mk, _, _ := hasNegation("不需要提交"); mk != "不需要" {
		t.Errorf("回显 = %q，期望「不需要」", mk)
	}
}
