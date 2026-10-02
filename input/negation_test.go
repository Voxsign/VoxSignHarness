// 否定仲裁的回归测试（缺口 G1）。
//
// G1 的真实危害：`不要删除那个文件` 被判成 EDIT(action=delete) —— 目标一旦可解析就会真的删。
// 修法是把"否定词直接支配动作"转为 ASK 确认（Ask != ” → 绝不执行）。
//
// 但否定词识别极易误伤，本文件把三类边界钉死：
//  1. 「能不能」不是否定 —— 它是否定词"不能"的常见载体，收了会把可行性问句判错；
//  2. 「特别/别的/告别」不是否定 —— 裸"别"会命中词的一部分；
//  3. 无动作的寒暄不该变成决策点 —— 「不用担心」不该弹确认。
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestNegationBecomesAskNotExecute G1 主线：否定 + 动作 → ASK 确认，绝不落成可执行删除。
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

// TestFeasibilityQuestionIsNotNegation 「能不能」含"不能"，绝不能被当成否定。
// 这条是新增否定表时最容易踩的坑。
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

// TestSingleBieNeedsFollowingVerb 裸"别"必须看后随动词，不能命中"特别/别的"。
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

// TestNegationWithoutActionIsNotDecisionPoint 无动作的寒暄不该变成决策点。
func TestNegationWithoutActionIsNotDecisionPoint(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{"不用担心", "不用了，谢谢"} {
		got := c.ClassifyTask(text)
		if got.Conflict == contract.ConflictNegation {
			t.Errorf("%q 无动作词却被判否定（会凭空弹确认）", text)
		}
	}
}

// TestNonNegatedCommandsUnchanged 防回归：不带否定的正常指令不被打扰。
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

// TestHasNegationUnit 直接锁 hasNegation 的行为。
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
