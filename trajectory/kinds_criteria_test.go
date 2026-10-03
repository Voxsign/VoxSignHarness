// kinds_criteria_test.go —— 判据⑪：kind 单一枚举 + 未登记必须报错（防"假绿空过"）。
package trajectory

import "testing"

// ⑪a 单一枚举：**每个 kind 常量都必须在 Kinds 里**（否则判据会引用不存在的符号）
func TestKindSingleSourceOfTruth(t *testing.T) {
	for _, k := range []string{
		KindInputRaw, KindInputClean, KindInputCorrec, KindIntentSource, KindIntent,
		KindStart, KindModel, KindActions, KindReceipts, KindFinal, KindError,
	} {
		if !KnownKind(k) {
			t.Errorf("[⑪] kind %q 未在 Kinds 中登记（判据若引用它会空过）", k)
		}
	}
	if KindIntentSource != "intent_source" {
		t.Errorf("[⑪] KindIntentSource 值应为 intent_source，实际 %q", KindIntentSource)
	}
	if len(Kinds) < 11 {
		t.Errorf("[⑪] Kinds 条目过少（%d）—— 疑似漏登记", len(Kinds))
	}
}

// ⑪b 未登记 kind ⇒ Write **必须报错**（不许静默写入）
func TestUnknownKindIsRejectedOnWrite(t *testing.T) {
	tr, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()
	if err := tr.Write(Entry{RequestID: "r1", Kind: "no_such_kind_xyz"}); err == nil {
		t.Error("[⑪] 未登记 kind 竟然写入成功（判据会静默空过）")
	}
	// 反例：已登记 kind ⇒ 必须成功
	if err := tr.Write(Entry{RequestID: "r1", Kind: KindIntentSource, Content: IntentSourceASR}); err != nil {
		t.Errorf("[⑪ 反例] 已登记 kind 被拒: %v", err)
	}
}

// ⑪c intent_source 值域必须是枚举（不许自由文本）
func TestIntentSourceValueDomain(t *testing.T) {
	if err := Validate(Entry{Kind: KindIntentSource, Content: "随便写的"}); err == nil {
		t.Error("[⑪] intent_source 接受了值域外的自由文本")
	}
	for _, v := range []string{IntentSourceASR, IntentSourceTextFallback} {
		if err := Validate(Entry{Kind: KindIntentSource, Content: v}); err != nil {
			t.Errorf("[⑪] 合法值 %q 被拒: %v", v, err)
		}
	}
}

// ⑪d 空 kind ⇒ 报错（不许"没有 kind 就当默认"）
func TestEmptyKindIsRejected(t *testing.T) {
	if err := Validate(Entry{}); err == nil {
		t.Error("[⑪] 空 kind 未报错")
	}
}
