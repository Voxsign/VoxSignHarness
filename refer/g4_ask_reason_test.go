//    G4  back   : UNKNOWN    origbecause  becoreferenceclarification write. 
//
// butalso     -- hasback  pipeline.TestCodexNineRegressions#1 needrequire
// " now    openstart …pipe   …  raise …"  sent**  **keep  refer  coreferenceclarification. 
//  splittgtapproveis"is is  coreference": 
//   - "pipe   …"is  to , refer    has value; 
//   - "         under"  "  "onlyislang word,  is  to . 
package refer

import (
	"strings"
	"testing"

	"voicesign-harness/contract"
)

func TestOperationAnaphora(t *testing.T) {
	cases := []struct {
		text    string
		trigger string
		want    bool
	}{
		{"把这个改一下", "这个", true},    // before charis"pipe"
		{"那个文件改一下", "那个文件", true}, // after charis word"modify"
		{"嗯那个呃记一下", "那个", false},  // beforeafterall is  lang 
		{"我那个前端的问题", "那个", false}, // onlyis  
		{"打开它", "它", true},        // after charis word" "( open)
	}
	for _, tc := range cases {
		if got := operationAnaphora(tc.text, tc.trigger); got != tc.want {
			t.Errorf("operationAnaphora(%q, %q) = %v，期望 %v", tc.text, tc.trigger, got, tc.want)
		}
	}
}

// TestAnyOperationAnaphoraScansAll     safety   : wordtable  andoutnow  noclose, 
//  sent   first inlang word"  ", but  change    coreference"pipe  ". 
func TestAnyOperationAnaphoraScansAll(t *testing.T) {
	long := "我现在想认真开始测，测完了之后能把这个哈你真的开始推进起来，我那个前端的问题又不过来"
	if !anyOperationAnaphora(long) {
		t.Error("长句里存在操作指代「把这个」，anyOperationAnaphora 应为 true")
	}
	if anyOperationAnaphora("嗯那个呃记一下") {
		t.Error("「嗯那个呃记一下」只有语气词，应为 false")
	}
}

// TestUnknownKeepsClassifierAsk G4  line: UNKNOWN +    coreference -> keep classify    origbecause. 
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

// TestUnknownWithOperationAnaphoraStillAsks has  coreferencetime    refer   ( hasback ). 
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

// TestNotePayloadIsJustPronoun    G9   boundary: NOTE  in coreference vs safety in . 
func TestNotePayloadIsJustPronoun(t *testing.T) {
	cases := []struct {
		text    string
		trigger string
		want    bool
	}{
		{"记一下 这个", "这个", true},              // coreferencethenissafety in  ->     
		{"记一下：这次要修的是报价页那个错别字", "那个", false}, // isin  ->    
		{"记一下，季总那个厂房下周一出报价", "那个", false},   // isin  ->    
	}
	for _, tc := range cases {
		if got := notePayloadIsJustPronoun(tc.text, tc.trigger); got != tc.want {
			t.Errorf("notePayloadIsJustPronoun(%q, %q) = %v，期望 %v", tc.text, tc.trigger, got, tc.want)
		}
	}
}

// TestNoteContentAnaphoraDoesNotAsk G9  line(refer  ). 
func TestNoteContentAnaphoraDoesNotAsk(t *testing.T) {
	r := New(nil)
	it := &contract.Intent{
		Intent:        contract.IntentNote,
		Confidence:    0.85,
		CorrectedText: "记一下：这次要修的是报价页那个错别字",
	}
	got, _, err := r.ResolveOptions(it, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Ask, "指的是哪个") {
		t.Errorf("G9: NOTE 的内容指代仍被追问：Ask=%q", got.Ask)
	}
	//  boundary: coreferencethenissafety in time    
	it2 := &contract.Intent{Intent: contract.IntentNote, Confidence: 0.85, CorrectedText: "记一下 这个"}
	got2, _, _ := r.ResolveOptions(it2, "")
	if !strings.Contains(got2.Ask, "Which did you mean") {
		t.Errorf("既有回归：'记一下 这个' 必须追问，实际 Ask=%q", got2.Ask)
	}
}
