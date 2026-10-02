// 多动作检测的边界测试（缺口 G5，最后一条）。
//
// G5 的真实危害：「查一下库存，然后记一下结果，最后提交」只执行第一个命中的意图，
// 其余动作**既不执行也不提示**，用户以为三件事都做了。
//
// 判据刻意用**顺序连接词**（然后/接着/最后…）而不是"句子里出现两个动作词"——
// M7 真机长句同时含 TEST 与 QUERY 词却是一段口语独白，必须保持不 Ask
// （既有回归 pipeline.TestColloquialQuestionNoReferAsk）。
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestMultiActionAsksForOrder G5 主线：一句话多件事 → 回问先做哪个，不静默丢弃。
func TestMultiActionAsksForOrder(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{
		"查一下库存，然后记一下结果，最后提交",
		"跑一下测试然后提交这批改动",
		"把主栈改成中文然后部署到服务器",
		"把报价模板改成新的公司抬头，然后跑一下测试",
		"记一下明天开会然后查一下上次的报价",
	} {
		got := c.ClassifyTask(text)
		if got.Ask == "" {
			t.Errorf("G5: %q 未回问（intent=%s）—— 其余动作会被静默丢弃", text, got.Intent)
		}
		if got.Conflict != contract.ConflictMultiAction {
			t.Errorf("%q: conflict = %q，期望 %q", text, got.Conflict, contract.ConflictMultiAction)
		}
	}
}

// TestMultiActionLongStatementExempt M7 真机口语长句不得被判多动作。
func TestMultiActionLongStatementExempt(t *testing.T) {
	m7 := "我现在测试一下，看看效果怎么样，如果这个效果好，我们就继续推进，就是重点是把这个能力建立起来"
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask(m7)
	if got.Conflict == contract.ConflictMultiAction {
		t.Errorf("M7 口语长句被误判为多动作（conflict=%q）", got.Conflict)
	}
	if got.Ask != "" {
		t.Errorf("M7 长句不得回问（既有回归 TestColloquialQuestionNoReferAsk）：Ask=%q", got.Ask)
	}
	if actions := multiActionIntents(m7); len(actions) >= 2 {
		t.Errorf("M7 长句被判多动作 %v —— 判据应基于顺序连接词", actions)
	}
}

// TestSingleActionUnchanged 单动作句子不受影响。
func TestSingleActionUnchanged(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct{ text, wantIntent string }{
		{"跑一下测试", contract.IntentTest},
		{"查一下库存", contract.IntentQuery},
		{"把报价模块改成中文", contract.IntentEdit},
		{"提交这批改动", contract.IntentCommit},
		{"记一下这个想法", contract.IntentNote},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.wantIntent {
			t.Errorf("%q: 意图 = %q，期望 %q（多动作检测误伤）", tc.text, got.Intent, tc.wantIntent)
		}
		if got.Conflict == contract.ConflictMultiAction {
			t.Errorf("%q 被误判为多动作", tc.text)
		}
	}
}

// TestMultiActionIntentsUnit 函数级单测：按连接词切分后数不同动作。
func TestMultiActionIntentsUnit(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"查一下库存，然后记一下结果，最后提交", 3},
		{"跑一下测试然后提交这批改动", 2},
		{"跑一下测试", 1},
		{"查一下库存", 1},
		{"", 0},
	}
	for _, tc := range cases {
		if got := len(multiActionIntents(tc.text)); got != tc.want {
			t.Errorf("multiActionIntents(%q) 数 = %d，期望 %d（%v）",
				tc.text, got, tc.want, multiActionIntents(tc.text))
		}
	}
}

// TestMultiActionDoesNotShadowOrchestrate 编排任务本身是多动作，已由计划承载，不该再问。
func TestMultiActionDoesNotShadowOrchestrate(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	text := "把全部沟通记录和设计文档整理成《全景开发文档》并保存提交"
	got := c.ClassifyTask(text)
	if got.Conflict == contract.ConflictMultiAction {
		t.Errorf("编排任务被多动作检测抢走：intent=%s conflict=%q", got.Intent, got.Conflict)
	}
}
