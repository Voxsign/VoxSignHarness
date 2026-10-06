//    (   G2)  boundary  . 
//
// G2      : "modify under""fix"" " class**   **sent be  0.85    , 
// Ask asempty,  connect in     --     finishsafety   to  case under  . 
//
// rootbecauseis classtriggersend  0.85,   atdefault value 0.6, atis"low-confidenceclarification"branch default  under
//      . fix ispipe"  need  "and"low-confidence" become    path. 
//
// basefilepipe boundary  : **has  then  clarification**,  then pipepos refer changebecomeno  decision point. 
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestSlotGateAsksOnZeroInformation G2  line:    sent   clarification. 
func TestSlotGateAsksOnZeroInformation(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct{ text, wantIntent string }{
		{"改一下", contract.IntentEdit},
		{"修", contract.IntentDebug},
		{"查", contract.IntentQuery},
		{"改", contract.IntentEdit},
		{"看看", contract.IntentQuery},
		{"把它改一下", contract.IntentEdit},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.wantIntent {
			t.Errorf("%q: 意图 = %q，期望 %q", tc.text, got.Intent, tc.wantIntent)
		}
		if got.Ask == "" {
			t.Errorf("G2: %q 是零信息句子却 Ask 为空（会静默进执行通道）", tc.text)
		}
		if got.Ask == taskAskTemplate {
			t.Errorf("%q: 回问应针对意图定制（指出缺什么），不该用通用模板", tc.text)
		}
	}
}

// TestSlotGateDoesNotFireOnInformativeText has  then  clarification --  thenpos refer be becomedecision point. 
func TestSlotGateDoesNotFireOnInformativeText(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct{ text, wantIntent string }{
		{"修一下这个 bug", contract.IntentDebug},
		{"查一下 quotes 目录里有多少个文件", contract.IntentQuery},
		{"把报价模块改成中文", contract.IntentEdit},
		{"查一下 voxbuybot 昨天那个订单状态", contract.IntentQuery},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.wantIntent {
			t.Errorf("%q: 意图 = %q，期望 %q", tc.text, got.Intent, tc.wantIntent)
		}
		if got.Ask != "" {
			t.Errorf("%q: 句子有信息却被回问（Ask=%q）—— 槽位闸误伤", tc.text, got.Ask)
		}
	}
}

// TestSlotGateScopeIsLimited    only to EDIT/DEBUG/QUERY occur . 
// TEST/COMMIT/DEPLOY/NOTE/ASK hasdefaultvalueorbasethen needneedto . 
func TestSlotGateScopeIsLimited(t *testing.T) {
	for _, kind := range []string{
		contract.IntentTest, contract.IntentCommit, contract.IntentDeploy,
		contract.IntentNote, contract.IntentAsk, contract.IntentRegisterTool,
	} {
		if slotGateTrips(kind, "跑一下") {
			t.Errorf("%s 不在槽位闸范围内，不应触发", kind)
		}
	}
	//  class   in
	for _, kind := range []string{contract.IntentEdit, contract.IntentDebug, contract.IntentQuery} {
		if !slotGateTrips(kind, "改一下") && !slotGateTrips(kind, "修") && !slotGateTrips(kind, "查") {
			t.Errorf("%s 应在槽位闸范围内", kind)
		}
	}
}

// TestSlotGateTripsUnit is num   :   triggersendwordand wordafter       . 
func TestSlotGateTripsUnit(t *testing.T) {
	cases := []struct {
		kind string
		text string
		want bool
	}{
		{contract.IntentEdit, "改一下", true},
		{contract.IntentEdit, "把它改一下", true},
		{contract.IntentEdit, "把报价模块改成中文", false},
		{contract.IntentDebug, "修", true},
		{contract.IntentDebug, "修一下这个 bug", false},
		{contract.IntentQuery, "查", true},
		{contract.IntentQuery, "查一下 quotes 目录里有多少个文件", false},
	}
	for _, tc := range cases {
		if got := slotGateTrips(tc.kind, tc.text); got != tc.want {
			t.Errorf("slotGateTrips(%s, %q) = %v，期望 %v", tc.kind, tc.text, got, tc.want)
		}
	}
}
