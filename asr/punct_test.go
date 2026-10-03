// punct_test.go —— 标点恢复的结构判据（SCOPE-PUNCT，需求 4.3 / C1 v2）。
//
// 不带 build tag：这是**已实现能力**的结构判据，随默认门禁常跑。
// 它只断言三件可机械判定的事，不假装评价"标点是否自然"（质量 unverified，RC7）：
//  1. 标点绝不触碰正文（去标点后 Text/Punctuated 逐字相等；保真类还与 Raw 相等）；
//  2. 有恢复就必须有留痕（PunctuationCorrections 非空、Kind/Confidence/Evidence 齐备）；
//  3. Corrections 的语义不变（只记对 Text 的改动；Text 未改则必须为空）。
package asr

import (
	"strings"
	"testing"
	"unicode"
)

// stripPunct 去掉标点与空白，只留正文字符。
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

// TestSCOPEPUNCT01PunctuationNeverTouchesBody 是 C1 v2 的安全侧。
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

// TestSCOPEPUNCT02PunctuationObservable 是 C1 v2 的可观测侧。
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

// TestSCOPEPUNCT03ExistingPunctuationPreserved：已存在的标点一律保留，不擅自改/删。
func TestSCOPEPUNCT03ExistingPunctuationPreserved(t *testing.T) {
	eng := NewEngine()
	raw := "做的另外一个地方开发嘛，开发在哪里嘛？这个只是在哪里跑的问题嘛"
	got := eng.Correct(CorrectRequest{Raw: raw})
	// 已有的标点片段原样在结果里（没有被改写）
	if !strings.Contains(got.Punctuated, "嘛，开发在哪里嘛？") {
		t.Errorf("已有标点被改写：%q → %q", raw, got.Punctuated)
	}
	// 正文不变
	if stripPunct(got.Punctuated) != stripPunct(raw) {
		t.Errorf("正文被改动：%q → %q", raw, got.Punctuated)
	}
	// 已有标点一个都没被删（只允许追加）
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

// TestSCOPEPUNCT04Deterministic：标点恢复可回放（纯函数）。
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

// TestSCOPEPUNCT05CorrectionsSemanticsUnchanged：标点不得混进 Corrections（C4 不变式）。
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
