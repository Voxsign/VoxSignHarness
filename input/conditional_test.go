// 条件句仲裁的边界测试（缺口 G6）。
//
// G6 的真实危害：「如果测试通过就提交」被判 TEST 直接执行 —— **前提被完全忽略**。
// 系统替用户做了他还没同意的事。
//
// 边界的关键在"条件 + 紧跟其后的动作"这个窗口：
// M7 真机长句「我现在测试一下…如果这个效果好，我们就继续推进…」同时含"如果""就"和动作词，
// 却是一段口语陈述，必须保持 Ask 为空（既有回归 pipeline.TestColloquialQuestionNoReferAsk）。
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestConditionalAsksInsteadOfExecuting G6 主线：条件句不得无条件执行。
func TestConditionalAsksInsteadOfExecuting(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{
		"如果测试通过就提交",
		"测试过了就部署",
		"只要测试全绿就发布",
		"要是没报错就提交",
	} {
		got := c.ClassifyTask(text)
		if got.Ask == "" {
			t.Errorf("G6: %q 含前置条件却 Ask 为空（intent=%s）—— 会被无条件执行", text, got.Intent)
		}
		if got.Conflict != contract.ConflictConditional {
			t.Errorf("%q: conflict = %q，期望 %q", text, got.Conflict, contract.ConflictConditional)
		}
		if got.Intent != contract.IntentAsk {
			t.Errorf("%q: 意图 = %q，期望 ASK", text, got.Intent)
		}
	}
}

// TestConditionalLongStatementExempt M7 真机长句不得被判条件句。
func TestConditionalLongStatementExempt(t *testing.T) {
	m7 := "我现在测试一下，看看效果怎么样，如果这个效果好，我们就继续推进，就是重点是把这个能力建立起来"
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask(m7)
	if got.Conflict == contract.ConflictConditional {
		t.Errorf("M7 口语长句被误判为条件句（conflict=%q）", got.Conflict)
	}
	if _, ok := conditionalClause(m7); ok {
		t.Error("conditionalClause 对 M7 长句应为 false —— 条件必须是后果紧跟条件")
	}
}

// TestConditionalDoesNotBreakNormalCommands 正常指令不受影响。
func TestConditionalDoesNotBreakNormalCommands(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct{ text, wantIntent string }{
		{"跑一下测试", contract.IntentTest},
		{"提交这批改动", contract.IntentCommit},
		{"部署到服务器", contract.IntentDeploy},
		{"把报价模块改成中文", contract.IntentEdit},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.wantIntent {
			t.Errorf("%q: 意图 = %q，期望 %q（条件仲裁误伤）", tc.text, got.Intent, tc.wantIntent)
		}
		if got.Conflict == contract.ConflictConditional {
			t.Errorf("%q 被误判为条件句", tc.text)
		}
	}
}

// TestConditionalClauseUnit 函数级单测。
func TestConditionalClauseUnit(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"如果测试通过就提交", true},
		{"测试过了就部署", true},
		{"只要全绿就发布", true},
		{"把报价模块改成中文", false}, // "改成"里有"改"，但没有条件标记
		{"提交这批改动", false},
		{"", false},
	}
	for _, tc := range cases {
		if _, got := conditionalClause(tc.text); got != tc.want {
			t.Errorf("conditionalClause(%q) = %v，期望 %v", tc.text, got, tc.want)
		}
	}
}

// TestNegationBeatsConditional 否定优先于条件。
func TestNegationBeatsConditional(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("如果测试通过就不要提交")
	// 两句都该拦住；这里只断言"不得进入可执行路径"，并记录实际命中的是哪一条
	if got.Ask == "" {
		t.Fatalf("既含否定又含条件，Ask 却为空（intent=%s）", got.Intent)
	}
	t.Logf("同时含否定与条件时命中 = %q（否定或条件都可接受，绝不能是可执行路径）", got.Conflict)
}
