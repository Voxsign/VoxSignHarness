package input

// taskintent_collision_test.go -- M5-1 triggersendword    : NOTE lang word vs TEST triggersendword. 
//
// fix  scenario: M4     "     under M4   "be TEST triggersendword"  " first  , 
//  restrict NOTE. rootbecause = noteTriggers  " under/  ", senttail"  "becomefirst  in. 
// fix  = noteTriggers patch" under/  ", and 2b  classopenclose NOTE basethen  TEST ofbefore. 
//
// disconnectlang path(posrevexample + QUERY  back ): 
//   - posexample(  NOTE lang word, senttail"  "isbe  to )-> NOTE
//   - revexample(only   TEST triggersend,    NOTE word)-> TEST
//   - QUERY kindbase  be    change  
//
//    : TestTriggerCollisionNoteVsTest(SPEC v2 §1 already ). 

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
	// and 20 kindexamplesame emptytime (vaultnotes diffname "  /   "), keep onunder   . 
	clf := NewTaskClassifier(0.6, twentyTestSpaces())

	cases := []struct {
		name string
		text string
		want string
	}{
		// posexample: NOTE lang word  edsenttail"  ". 
		{"note_记下_带测试", "在笔记里记下 M4 测试", contract.IntentNote},
		{"note_记一下_带测试", "记一下 昨天那个测试结果", contract.IntentNote},
		{"note_记个想法_测试下", "记个想法测试下", contract.IntentNote},
		// revexample:    TEST triggersend,      NOTE word,   keepkeep TEST  back . 
		{"test_跑一下", "跑一下测试", contract.IntentTest},
		{"test_执行", "执行测试", contract.IntentTest},
		{"test_这个函数", "测试这个函数", contract.IntentTest},
		// QUERY  back . 
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
