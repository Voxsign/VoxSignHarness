// 缺口 G4 的回归测试：UNKNOWN 的澄清原因不得被指代回问覆写。
//
// 但也不能一刀切——既有回归 pipeline.TestCodexNineRegressions#1 要求
// 「我现在想认真开始测…把这个哈…推进起来…」这一句**必须**保留 refer 的指代回问。
// 区分标准是"是不是操作指代"：
//   - 「把 这个…」是操作对象，refer 的澄清有价值；
//   - 「嗯 那个 呃 记一下」里的"那个"只是语气词，不是操作对象。
package refer

import (
	"testing"

	"voicesign-harness/contract"
)

func TestOperationAnaphora(t *testing.T) {
	cases := []struct {
		text    string
		trigger string
		want    bool
	}{
		{"把这个改一下", "这个", true},    // 前一字是"把"
		{"那个文件改一下", "那个文件", true}, // 后一字是动词"改"
		{"嗯那个呃记一下", "那个", false},  // 前后都不是操作语境
		{"我那个前端的问题", "那个", false}, // 只是叙述
		{"打开它", "它", true},        // 后一字是动词"打"（打开）
	}
	for _, tc := range cases {
		if got := operationAnaphora(tc.text, tc.trigger); got != tc.want {
			t.Errorf("operationAnaphora(%q, %q) = %v，期望 %v", tc.text, tc.trigger, got, tc.want)
		}
	}
}

// TestAnyOperationAnaphoraScansAll 必须遍历全部候选：词表顺序与出现位置无关，
// 长句里可能先命中语气词"那个"，却漏掉更早的操作指代"把这个"。
func TestAnyOperationAnaphoraScansAll(t *testing.T) {
	long := "我现在想认真开始测，测完了之后能把这个哈你真的开始推进起来，我那个前端的问题又不过来"
	if !anyOperationAnaphora(long) {
		t.Error("长句里存在操作指代「把这个」，anyOperationAnaphora 应为 true")
	}
	if anyOperationAnaphora("嗯那个呃记一下") {
		t.Error("「嗯那个呃记一下」只有语气词，应为 false")
	}
}

// TestUnknownKeepsClassifierAsk G4 主线：UNKNOWN + 非操作指代 → 保留分类器的澄清原因。
func TestUnknownKeepsClassifierAsk(t *testing.T) {
	r := New(nil)
	classifierAsk := "你是想让我做什么？请再说清楚一点"
	it := &contract.Intent{
		Intent:        contract.IntentUnknown,
		Confidence:    0.2,
		CorrectedText: "嗯那个呃记一下",
		Ask:           classifierAsk,
	}
	got, _, err := r.ResolveOptions(it, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Ask != classifierAsk {
		t.Errorf("G4: UNKNOWN 的澄清原因被覆写为 %q，期望保留 %q", got.Ask, classifierAsk)
	}
}

// TestUnknownWithOperationAnaphoraStillAsks 有操作指代时仍应让 refer 提问（既有回归）。
func TestUnknownWithOperationAnaphoraStillAsks(t *testing.T) {
	r := New(nil)
	long := "我现在想认真开始测，测完了之后能把这个哈你真的开始推进起来，我那个前端的问题又不过来"
	it := &contract.Intent{
		Intent:        contract.IntentUnknown,
		Confidence:    0.2,
		CorrectedText: long,
		Ask:           "你是想让我做什么？",
	}
	got, _, err := r.ResolveOptions(it, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Ask == "你是想让我做什么？" {
		t.Errorf("含操作指代时 refer 应给出指代回问，实际仍为分类器原文 %q", got.Ask)
	}
}
