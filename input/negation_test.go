//      back   (   G1). 
//
// G1      : ` needdelete  file` be become EDIT(action=delete) -- objtgt   resolve then    . 
// fix ispipe"  word connect    " as ASK confirm(Ask != ” ->     ). 
//
// but  word diff    , basefilepipe class boundary  : 
//  1. "   " is   --  is  word"  "  see body, recv pipe  ity sent  ; 
//  2. " diff/diff / diff" is   --  "diff"  inword   split; 
//  3. no       changebecomedecision point -- " use  "   confirm. 
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestNegationBecomesAskNotExecute G1  line:    +    -> ASK confirm,    become   delete. 
func TestNegationBecomesAskNotExecute(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{
		"不要删除那个文件",
		"别删掉这条记录",
		"不用提交这批改动",
		"不需要部署到服务器",
		"先别发布",
	} {
		got := c.ClassifyTask(text)
		if got.Intent == contract.IntentEdit && got.Params["action"] == "delete" {
			t.Errorf("%q 仍被判为可执行删除 —— G1 未修复", text)
		}
		if got.Ask == "" {
			t.Errorf("%q: 否定句必须回问（Ask != '' 才保证绝不执行），实际 Ask 为空，intent=%s",
				text, got.Intent)
		}
		if got.Conflict != contract.ConflictNegation {
			t.Errorf("%q: conflict = %q，期望 %q", text, got.Conflict, contract.ConflictNegation)
		}
	}
}

// TestFeasibilityQuestionIsNotNegation "   " "  ",    becurbecome  . 
//   isnewadd  tabletime      . 
func TestFeasibilityQuestionIsNotNegation(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{
		"查一下能不能跑测试",
		"能不能帮我看一下库存",
		"可不可以改成中文",
	} {
		got := c.ClassifyTask(text)
		if got.Conflict == contract.ConflictNegation {
			t.Errorf("%q 被误判为否定（conflict=%s）—— 「能不能」里的\"不能\"不是否定",
				text, got.Conflict)
		}
		if got.Intent != contract.IntentAsk {
			t.Errorf("%q: 可行性问句应判 ASK，实际 %q", text, got.Intent)
		}
	}
}

// TestSingleBieNeedsFollowingVerb  "diff"   after  word,    in" diff/diff ". 
func TestSingleBieNeedsFollowingVerb(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	positives := []string{"别删了", "别发了", "别提交了", "别改了"}
	for _, text := range positives {
		if got := c.ClassifyTask(text); got.Conflict != contract.ConflictNegation {
			t.Errorf("%q 应判否定，实际 conflict=%q intent=%s", text, got.Conflict, got.Intent)
		}
	}
	negatives := []string{"特别关注一下那个报错", "改别的文件", "告别旧版本"}
	for _, text := range negatives {
		if got := c.ClassifyTask(text); got.Conflict == contract.ConflictNegation {
			t.Errorf("%q 被误判为否定 —— 裸\"别\"不该命中词的一部分", text)
		}
	}
}

// TestNegationWithoutActionIsNotDecisionPoint no       changebecomedecision point. 
func TestNegationWithoutActionIsNotDecisionPoint(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{"不用担心", "不用了，谢谢"} {
		got := c.ClassifyTask(text)
		if got.Conflict == contract.ConflictNegation {
			t.Errorf("%q 无动作词却被判否定（会凭空弹确认）", text)
		}
	}
}

// TestNonNegatedCommandsUnchanged preventback :      pos refer  be  . 
func TestNonNegatedCommandsUnchanged(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct{ text, wantIntent string }{
		{"删除那个文件", contract.IntentEdit},
		{"把报价模块改成中文", contract.IntentEdit},
		{"记一下这个想法", contract.IntentNote},
		{"跑一下测试", contract.IntentTest},
		{"提交这批改动", contract.IntentCommit},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.wantIntent {
			t.Errorf("%q: 意图 = %q，期望 %q（否定仲裁误伤正常指令）", tc.text, got.Intent, tc.wantIntent)
		}
		if got.Conflict == contract.ConflictNegation {
			t.Errorf("%q: 被误判为否定", tc.text)
		}
	}
}

// TestHasNegationUnit  connect  hasNegation   as. 
func TestHasNegationUnit(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"不要删除", true},
		{"别删", true},
		{"特别关注", false},
		{"能不能", false},
		{"别的", false},
		{"", false},
	}
	for _, tc := range cases {
		if _, _, got := hasNegation(tc.text); got != tc.want {
			t.Errorf("hasNegation(%q) = %v，期望 %v", tc.text, got, tc.want)
		}
	}
}
