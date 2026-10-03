// intent_test.go —— 本地意图分类的默认门禁（无网络、确定性）。
package asr

import (
	"context"
	"errors"
	"testing"
	"time"
)

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

// fakeIntentModel 记录调用次数，用于证明"高置信不调模型"。
type fakeIntentModel struct {
	calls int
	typ   string
	conf  float64
	err   error
}

func (f *fakeIntentModel) ClassifyIntent(ctx context.Context, text string) (string, float64, error) {
	f.calls++
	return f.typ, f.conf, f.err
}

func TestHighConfidenceNeverCallsModel(t *testing.T) {
	f := &fakeIntentModel{typ: "DEPLOY", conf: 0.99}
	got := ClassifyIntentWith(context.Background(), "提交这次改动", f, time.Second)
	if f.calls != 0 {
		t.Fatalf("高置信路径调了模型 %d 次（违反红线 #6：核心路径必须本地）", f.calls)
	}
	if got.Type != "COMMIT" || got.Degraded {
		t.Errorf("高置信结果不符: %+v", got)
	}
}

func TestLowConfidenceUsesModelFallback(t *testing.T) {
	f := &fakeIntentModel{typ: "QUERY", conf: 0.88}
	got := ClassifyIntentWith(context.Background(), "嗯这个东西吧", f, time.Second)
	if f.calls != 1 {
		t.Fatalf("低置信未调用兜底模型: calls=%d", f.calls)
	}
	if got.Type != "QUERY" || got.Degraded || got.NeedDisambiguate {
		t.Errorf("兜底结果不符: %+v", got)
	}
}

func TestModelFailureDegradesToLocal(t *testing.T) {
	f := &fakeIntentModel{err: errors.New("boom")}
	got := ClassifyIntentWith(context.Background(), "嗯这个东西吧", f, time.Second)
	if !got.Degraded || got.DegradedReason == "" {
		t.Errorf("模型失败未降级留痕: %+v", got)
	}
	if got.Type == "" {
		t.Errorf("降级后必须返回本地结果: %+v", got)
	}
}

func TestModelInvalidIntentIgnored(t *testing.T) {
	f := &fakeIntentModel{typ: "DELETE_EVERYTHING", conf: 0.99}
	got := ClassifyIntentWith(context.Background(), "嗯这个东西吧", f, time.Second)
	if !got.Degraded {
		t.Errorf("非法意图类别应被忽略并降级: %+v", got)
	}
}
