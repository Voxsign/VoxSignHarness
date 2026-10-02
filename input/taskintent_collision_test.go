package input

// taskintent_collision_test.go —— M5-1 触发词碰撞仲裁：NOTE 语境词 vs TEST 触发词。
//
// 修复背景：M4 冒烟暴露「在笔记里记下 M4 测试」被 TEST 触发词「测试」抢先误判，
// 压制了 NOTE。根因 = noteTriggers 缺「记下/记个」，句尾「测试」成了首个命中。
// 修复 = noteTriggers 补「记下/记个」，且 2b 单类开关 NOTE 本就在 TEST 之前。
//
// 断言口径（正反例 + QUERY 不回归）：
//   - 正例（含 NOTE 语境词，句尾「测试」是被记录对象）→ NOTE
//   - 反例（仅独立 TEST 触发，不含 NOTE 词）→ TEST
//   - QUERY 样本不得被这次仲裁改动影响
//
// 验证器：TestTriggerCollisionNoteVsTest（SPEC v2 §1 已钉）。

import (
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/memory"
)

func TestTriggerCollisionNoteVsTest(t *testing.T) {
	cleaner := NewCleaner(nil)
	dict, err := memory.LoadDictionary("")
	if err != nil {
		t.Fatalf("加载内置词典失败: %v", err)
	}
	// 与 20 样例同一空间集（vaultnotes 别名含「想法/想法库」），保证上下文一致。
	clf := NewTaskClassifier(0.6, twentyTestSpaces())

	cases := []struct {
		name string
		text string
		want string
	}{
		// 正例：NOTE 语境词应压过句尾「测试」。
		{"note_记下_带测试", "在笔记里记下 M4 测试", contract.IntentNote},
		{"note_记一下_带测试", "记一下 昨天那个测试结果", contract.IntentNote},
		{"note_记个想法_测试下", "记个想法测试下", contract.IntentNote},
		// 反例：独立 TEST 触发，不含任何 NOTE 词，必须保持 TEST 不回归。
		{"test_跑一下", "跑一下测试", contract.IntentTest},
		{"test_执行", "执行测试", contract.IntentTest},
		{"test_这个函数", "测试这个函数", contract.IntentTest},
		// QUERY 不回归。
		{"query_不回归", "查一下有什么问题", contract.IntentQuery},
	}

	for _, tc := range cases {
		cleaned := cleaner.Clean(tc.text)
		corrected, _ := dict.Correct(cleaned)
		got := clf.ClassifyTask(corrected)
		if got.Intent != tc.want {
			t.Errorf("%s %q: 实跑意图=%q, 期望=%q", tc.name, tc.text, got.Intent, tc.want)
		}
	}
}
