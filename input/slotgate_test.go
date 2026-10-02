// 槽位闸（缺口 G2）的边界测试。
//
// G2 的真实危害：「改一下」「修」「查」这类**零信息**句子被判 0.85 高置信、
// Ask 为空，直接进入执行通道 —— 系统会在完全不知道对象的情况下动手。
//
// 根因是单类触发恒 0.85，恒高于默认阈值 0.6，于是"低置信回问"分支在默认配置下
// 几乎不可达。修法是把"缺必需槽位"与"低置信"拆成两条独立路径。
//
// 本文件把边界钉死：**有信息就不能回问**，否则会把正常指令变成无谓的决策点。
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestSlotGateAsksOnZeroInformation G2 主线：零信息句子必须回问。
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

// TestSlotGateDoesNotFireOnInformativeText 有信息就不该回问 —— 否则正常指令被拖成决策点。
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

// TestSlotGateScopeIsLimited 槽位闸只应对 EDIT/DEBUG/QUERY 生效。
// TEST/COMMIT/DEPLOY/NOTE/ASK 有默认值或本就不需要对象。
func TestSlotGateScopeIsLimited(t *testing.T) {
	for _, kind := range []string{
		contract.IntentTest, contract.IntentCommit, contract.IntentDeploy,
		contract.IntentNote, contract.IntentAsk, contract.IntentRegisterTool,
	} {
		if slotGateTrips(kind, "跑一下") {
			t.Errorf("%s 不在槽位闸范围内，不应触发", kind)
		}
	}
	// 三类在范围内
	for _, kind := range []string{contract.IntentEdit, contract.IntentDebug, contract.IntentQuery} {
		if !slotGateTrips(kind, "改一下") && !slotGateTrips(kind, "修") && !slotGateTrips(kind, "查") {
			t.Errorf("%s 应在槽位闸范围内", kind)
		}
	}
}

// TestSlotGateTripsUnit 是函数级单测：剥除触发词与虚词后的残留长度判定。
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
