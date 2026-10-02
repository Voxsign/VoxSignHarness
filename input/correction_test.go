// 自我修正的边界测试（缺口 G7）。
//
// G7 的真实危害：用户口述时改口——`记一下A，不对，改成记B` —— 系统把
// "不对"之前的整段当成目标槽位（object="记一下A，不对"），即**用被作废的内容去执行**。
//
// 设计要点：
//  1. 修正标记取**最后一次**出现，支持连续改口；
//  2. 修正后常省略对象（`把标题改成中文，不对，改成英文`），须**承接**修正前的对象。
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestSelfCorrectionDropsPriorClause 修正前的内容不得进入槽位。
func TestSelfCorrectionDropsPriorClause(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("记一下A，不对，改成记B")
	if got.Intent == contract.IntentEdit {
		obj := got.Params["object"]
		if obj != "" && (contains(obj, "不对") || contains(obj, "记一下A")) {
			t.Errorf("G7: object=%q 把修正前的内容吞进了目标槽位", obj)
		}
	}
}

// TestSelfCorrectionCarriesObject 修正后省略对象时，承接修正前的对象。
func TestSelfCorrectionCarriesObject(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("把标题改成中文，不对，改成英文")
	if got.Intent != contract.IntentEdit {
		t.Fatalf("意图 = %q，期望 EDIT", got.Intent)
	}
	if got.Params["object"] != "标题" {
		t.Errorf("object = %q，期望「标题」（修正前对象应被承接）", got.Params["object"])
	}
	if got.Params["value"] != "英文" {
		t.Errorf("value = %q，期望「英文」（应取最后一次改口的值）", got.Params["value"])
	}
}

// TestLastCorrectionTakesLatest 连续改口取最后一次。
func TestLastCorrectionTakesLatest(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("把标题改成中文，不对，改成英文，说错了，改成阿拉伯语")
	if got.Intent != contract.IntentEdit {
		t.Fatalf("意图 = %q，期望 EDIT", got.Intent)
	}
	if got.Params["value"] != "阿拉伯语" {
		t.Errorf("value = %q，期望「阿拉伯语」（最后一次改口）", got.Params["value"])
	}
	if got.Params["object"] != "标题" {
		t.Errorf("object = %q，期望「标题」", got.Params["object"])
	}
}

// TestNoCorrectionUnchanged 无修正标记时行为不变。
func TestNoCorrectionUnchanged(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("把报价模块改成中文")
	if got.Intent != contract.IntentEdit {
		t.Fatalf("意图 = %q，期望 EDIT", got.Intent)
	}
	if got.Params["object"] != "报价模块" || got.Params["value"] != "中文" {
		t.Errorf("槽位被改动: object=%q value=%q", got.Params["object"], got.Params["value"])
	}
}

// TestLastCorrectionUnit 函数级单测。
func TestLastCorrectionUnit(t *testing.T) {
	cases := []struct {
		text      string
		wantFound bool
	}{
		{"记一下A，不对，改成记B", true},
		{"把标题改成中文，说错了，改成英文", true},
		{"把报价模块改成中文", false},
		{"", false},
	}
	for _, tc := range cases {
		if cut, end := lastCorrection(tc.text); (cut >= 0) != tc.wantFound {
			t.Errorf("lastCorrection(%q) = (%d,%d)，期望 found=%v", tc.text, cut, end, tc.wantFound)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
