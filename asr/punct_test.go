// punct_test.go -- tgtpt   close  data(SCOPE-PUNCT, needrequire 4.3 / C1 v2). 
//
//    build tag:  is**already now  ** close  data,  default forbid  . 
//  onlydisconnectlang         ,      "tgtptis  however"(   unverified, RC7): 
//  1. tgtpt  trigger pos ( tgtptafter Text/Punctuated  char etc; keep classalsoand Raw  etc); 
//  2. has  then  has  (PunctuationCorrections  empty, Kind/Confidence/Evidence   ); 
//  3. Corrections  semantic change(only to Text  change; Text  modifythen  asempty). 
package asr

import (
	"strings"
	"testing"
	"unicode"
)

// stripPunct   tgtptandempty , only pos char . 
func stripPunct(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsPunct(r) || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestSCOPEPUNCT01PunctuationNeverTouchesBody is C1 v2  safesafetyside. 
func TestSCOPEPUNCT01PunctuationNeverTouchesBody(t *testing.T) {
	eng := NewEngine()
	for _, c := range loadCorpusFile(t) {
		got := eng.Correct(CorrectRequest{Raw: c.Raw})
		if stripPunct(got.Punctuated) != stripPunct(got.Text) {
			t.Errorf("[%s] 标点改变了正文：Text=%q Punctuated=%q", c.ID, got.Text, got.Punctuated)
		}
		if c.Expect.Fidelity || tagged(c, "negation") || tagged(c, "safety") || tagged(c, "long") {
			if stripPunct(got.Punctuated) != stripPunct(c.Raw) {
				t.Errorf("[%s] 保真类正文被标点步骤改动：raw=%q Punctuated=%q", c.ID, c.Raw, got.Punctuated)
			}
			if got.Text != c.Raw {
				t.Errorf("[%s] 保真类 Text 被改动：%q → %q", c.ID, c.Raw, got.Text)
			}
		}
	}
}

// TestSCOPEPUNCT02PunctuationObservable is C1 v2     side. 
func TestSCOPEPUNCT02PunctuationObservable(t *testing.T) {
	eng := NewEngine()
	seen := 0
	for _, c := range loadCorpusFile(t) {
		got := eng.Correct(CorrectRequest{Raw: c.Raw})
		if got.Punctuated == got.Text {
			if len(got.PunctuationCorrections) != 0 {
				t.Errorf("[%s] 未恢复标点却有留痕：%+v", c.ID, got.PunctuationCorrections)
			}
			continue
		}
		seen++
		if len(got.PunctuationCorrections) == 0 {
			t.Errorf("[%s] 标点变了却没有 PunctuationCorrections（不可观测）", c.ID)
			continue
		}
		for _, cor := range got.PunctuationCorrections {
			if cor.Kind != "punctuation" {
				t.Errorf("[%s] Kind=%q，期望 punctuation", c.ID, cor.Kind)
			}
			if cor.From != "" || cor.Start != cor.End {
				t.Errorf("[%s] 标点留痕应为插入语义：%+v", c.ID, cor)
			}
			if cor.To == "" {
				t.Errorf("[%s] 插入内容为空：%+v", c.ID, cor)
			}
			if cor.Confidence <= 0 || cor.Confidence > 1 {
				t.Errorf("[%s] 置信度越界：%v", c.ID, cor.Confidence)
			}
			if cor.Evidence == "" {
				t.Errorf("[%s] 缺 Evidence", c.ID)
			}
			if cor.Start < 0 || cor.Start > len(got.Text) {
				t.Errorf("[%s] 插入点越界：%d (Text len=%d)", c.ID, cor.Start, len(got.Text))
			}
		}
	}
	if seen == 0 {
		t.Fatal("语料里没有一条发生标点恢复——判据形同虚设")
	}
	t.Logf("标点恢复覆盖 %d 条（共 %d 条）", seen, len(loadCorpusFile(t)))
}

// TestSCOPEPUNCT03ExistingPunctuationPreserved: alreadystore  tgtpt  keep ,    modify/ . 
func TestSCOPEPUNCT03ExistingPunctuationPreserved(t *testing.T) {
	eng := NewEngine()
	raw := "做的另外一个地方开发嘛，开发在哪里嘛？这个只是在哪里跑的问题嘛"
	got := eng.Correct(CorrectRequest{Raw: raw})
	// alreadyhas tgtpt segorigkind close  ( hasbemodifywrite)
	if !strings.Contains(got.Punctuated, "嘛，开发在哪里嘛？") {
		t.Errorf("已有标点被改写：%q → %q", raw, got.Punctuated)
	}
	// pos  change
	if stripPunct(got.Punctuated) != stripPunct(raw) {
		t.Errorf("正文被改动：%q → %q", raw, got.Punctuated)
	}
	// alreadyhastgtpt  all be (only allow  )
	if countPunct(got.Punctuated) < countPunct(raw) {
		t.Errorf("已有标点被删除：raw=%d → out=%d", countPunct(raw), countPunct(got.Punctuated))
	}
}

func countPunct(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsPunct(r) {
			n++
		}
	}
	return n
}

// TestSCOPEPUNCT04Deterministic: tgtpt   back (  num). 
func TestSCOPEPUNCT04Deterministic(t *testing.T) {
	eng := NewEngine()
	for _, c := range loadCorpusFile(t) {
		a := eng.Correct(CorrectRequest{Raw: c.Raw})
		b := eng.Correct(CorrectRequest{Raw: c.Raw})
		if a.Punctuated != b.Punctuated || len(a.PunctuationCorrections) != len(b.PunctuationCorrections) {
			t.Errorf("[%s] 不幂等/不可回放：%q vs %q", c.ID, a.Punctuated, b.Punctuated)
		}
	}
}

// TestSCOPEPUNCT05CorrectionsSemanticsUnchanged: tgtpt     Corrections(C4  changeform). 
func TestSCOPEPUNCT05CorrectionsSemanticsUnchanged(t *testing.T) {
	eng := NewEngine()
	for _, c := range loadCorpusFile(t) {
		got := eng.Correct(CorrectRequest{Raw: c.Raw})
		for _, cor := range got.Corrections {
			if cor.Kind == "punctuation" {
				t.Errorf("[%s] 标点留痕混进了 Corrections（应为 PunctuationCorrections）：%+v", c.ID, cor)
			}
		}
		if got.Text == c.Raw && len(got.Corrections) > 0 {
			t.Errorf("[%s] Text 未改却有 Correction（C4 被破）", c.ID)
		}
	}
}

// TestSCOPEPUNCT06ClauseSplitInsidePunctuated: fromsentdisconnectsent**onlysendoccur  punctuated in**, 
// corrected  allow (§5.1   6  : tgtptdisconnectsent).    data  "disconnectsentalready now": 
// linkconnectwordbeforepatch id + sentendpatchsentid, pos  char change. 
func TestSCOPEPUNCT06ClauseSplitInsidePunctuated(t *testing.T) {
	eng := NewEngine()
	cases := []struct{ raw, wantPunct string }{
		{"先改这个文件所以再提交", "，所以"},
		{"我想查一下库存但是先等一下", "，但是"},
		{"这个比较复杂而且很紧急", "，而且"},
	}
	seen := 0
	for _, c := range cases {
		got := eng.Correct(CorrectRequest{Raw: c.raw})
		if got.Text != c.raw {
			t.Errorf("[PUNCT-06] 断句改动了 corrected：%q → %q", c.raw, got.Text)
		}
		if strings.Contains(got.Punctuated, c.wantPunct) {
			seen++
		} else {
			t.Errorf("[PUNCT-06] punctuated 未做从句断句：raw=%q punct=%q want=%q", c.raw, got.Punctuated, c.wantPunct)
		}
		if stripPunct(got.Punctuated) != stripPunct(c.raw) {
			t.Errorf("[PUNCT-06] 断句改了正文：%q → %q", c.raw, got.Punctuated)
		}
	}
	if seen == 0 {
		t.Fatal("三条断句样例一条都没生效——判据形同虚设")
	}
}
