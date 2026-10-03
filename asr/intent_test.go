// intent_test.go —— 本地意图分类的默认门禁（无网络、确定性）。
package asr

import "testing"

func TestLocalIntentNineTypes(t *testing.T) {
	cases := map[string]string{
		"把报价单改成中文":     "EDIT",
		"查一下这个报错的原因":   "DEBUG",
		"库存还有多少":       "QUERY",
		"跑一下测试":        "TEST",
		"提交这次改动":       "COMMIT",
		"部署到服务器":       "DEPLOY",
		"记一下这个想法":      "NOTE",
		"这个怎么弄":        "ASK",
		"先查库存再改报价最后提交": "ORCHESTRATE",
	}
	for text, want := range cases {
		if got := ClassifyIntent(text); got.Type != want {
			t.Errorf("ClassifyIntent(%q) = %q，期望 %q", text, got.Type, want)
		}
	}
}

func TestControlSemanticsAreInteractionOnly(t *testing.T) {
	for text, want := range map[string]string{"暂停": "pause", "撤销": "undo", "打断": "interrupt"} {
		got := ClassifyIntent(text)
		if got.Control != want {
			t.Errorf("ClassifyIntent(%q).Control = %q，期望 %q", text, got.Control, want)
		}
		if got.Type != "ASK" {
			t.Errorf("控制语义不得翻译成业务 type: %q → %q", text, got.Type)
		}
	}
}

func TestLowConfidenceAsksBack(t *testing.T) {
	got := ClassifyIntent("嗯这个东西吧")
	if !got.NeedDisambiguate {
		t.Errorf("低置信应 need_disambiguate: %+v", got)
	}
	if len(got.DomainSuggestion) != 0 {
		t.Errorf("默认拒绝：不得给域建议: %v", got.DomainSuggestion)
	}
}
