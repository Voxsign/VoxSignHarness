// punctuate.go -- keep  ruleformtgtpt  (needrequire 4.3;  data C1 v2). 
//
// safesafety  (basefileonly allow     ): 
//  1. **only intgtpt,   modify/ pos   char ** -- keep 
//     stripPunct(Punctuated) == stripPunct(Text) == stripPunct(Raw)(keep class); 
//  2. **   Text,    Corrections** -- close onlywrite
//     CorrectResult.Punctuated and CorrectResult.PunctuationCorrections, 
//     frombut C4  " modify Text   has  " changeformorigkindbecome ; 
//  3. **alreadystore  tgtpt  keep ** --    modifyuseuseralreadyhas   (    ). 
//
//    boundary(RC7): base nowonlykeep "close     + pos safesafety". 
// tgtpt**  **(splitsegis  however,  id  is   lang )**notgtnotelang ,    **, 
//   pipe"has  " become"tgtpt however". 
//
// rule(    ): 
//   - sentendnotgtpt -> patch". "; endcharas  tail( / / )-> patch" "; 
//   - linkconnectword( by/butis/…)beforeandbefore  sent >=4 char, afterconnectpos  -> patch", ". 
package asr

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// commaMarkers is"   raise  sent" linkconnectword.   **  **"thenis""howeverafter" class
//   sent     / langize word(task  §5.2   :  fill/linkconnectwordtable need ). 
var commaMarkers = []string{"所以", "但是", "不过", "而且", "另外", "因此", "可是", "否则"}

// isPunctRune     char is astgtpt(  empty --empty  giveon ,    place  ). 
func isPunctRune(r rune) bool {
	switch r {
	case '。', '！', '？', '，', '、', '；', '：', '…', '—', '·', '～',
		'.', '!', '?', ',', ';', ':', '~',
		'“', '”', '‘', '’', '「', '」', '『', '』', '（', '）', '【', '】', '《', '》':
		return true
	}
	return false
}

// punctuate returnbacktgtpt  after  base +    in  . 
// no    timereturnback (text, nil), i.e. Punctuated == Text. 
func punctuate(text string) (string, []Correction) {
	if text == "" {
		return "", nil
	}
	type insert struct {
		at   int //  inpt: Text in charnodeundertgt
		s    string
		conf float64
		why  string
	}
	var inserts []insert

	// rule 1: linkconnectwordbeforepatch id. 
	for _, m := range commaMarkers {
		for from := 0; ; {
			i := strings.Index(text[from:], m)
			if i < 0 {
				break
			}
			b := from + i
			from = b + len(m)
			if b == 0 {
				continue // sentfirst patch id
			}
			if len([]rune(text[:b])) < 4 {
				continue // before  sent  ,     
			}
			prev, _ := utf8.DecodeLastRuneInString(text[:b])
			if isPunctRune(prev) || prev == ' ' || prev == '\t' {
				continue // beforefacealreadyhastgtpt/empty 
			}
			if r, _ := utf8.DecodeRuneInString(text[b+len(m):]); isPunctRune(r) {
				continue // linkconnectwordafterface connectistgtpt,   
			}
			inserts = append(inserts, insert{
				at: b, s: "，", conf: 0.55,
				why: "标点恢复：连接词「" + m + "」前分句（只插标点，不改正文）",
			})
		}
	}

	// rule 2: sentendpatchsentid/ id. 
	last, _ := utf8.DecodeLastRuneInString(text)
	if !isPunctRune(last) {
		ins := insert{at: len(text), s: "。", conf: 0.65, why: "标点恢复：句末补句号（只插标点，不改正文）"}
		switch last {
		case '吗', '呢', '嘛':
			ins.s = "？"
			ins.conf = 0.70
			ins.why = "标点恢复：疑问尾「" + string(last) + "」→ 句末问号（只插标点，不改正文）"
		}
		inserts = append(inserts, ins)
	}

	if len(inserts) == 0 {
		return text, nil
	}
	sort.SliceStable(inserts, func(i, j int) bool { return inserts[i].at < inserts[j].at })

	var b strings.Builder
	var corrs []Correction
	prev := 0
	for _, in := range inserts {
		if in.at < prev || in.at > len(text) {
			continue // preventheavy /out-of-scope
		}
		b.WriteString(text[prev:in.at])
		b.WriteString(in.s)
		corrs = append(corrs, Correction{
			Start: in.at, End: in.at, //  insemantic: Start == End
			From: "", To: in.s,
			Kind: "punctuation", Confidence: in.conf, Evidence: in.why,
		})
		prev = in.at
	}
	b.WriteString(text[prev:])
	return b.String(), corrs
}
