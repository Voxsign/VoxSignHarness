//   fixpos  boundary  (   G7). 
//
// G7      : useuser  timemodify --`  underA,  to, modifybecome B` --   pipe
// " to"ofbefore  segcurbecomeobjtgt  (object="  underA,  to"), i.e.**usebe   in    **. 
//
//   needpt: 
//  1. fixpostgt get** after  **outnow,  keeplinkcontinuemodify ; 
//  2. fixposafter   to (`pipetgt modifybecomein ,  to, modifybecome  `),  ** connect**fixposbefore to . 
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestSelfCorrectionDropsPriorClause fixposbefore in    in  . 
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

// TestSelfCorrectionCarriesObject fixposafter  to time,  connectfixposbefore to . 
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

// TestLastCorrectionTakesLatest linkcontinuemodify get after  . 
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

// TestNoCorrectionUnchanged nofixpostgt time as change. 
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

// TestLastCorrectionUnit  num   . 
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
