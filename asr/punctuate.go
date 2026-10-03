// punctuate.go —— 保守的规则式标点恢复（需求 4.3；判据 C1 v2）。
//
// 安全契约（本文件只允许做这三件事）：
//  1. **只插入标点，绝不改/删正文任何字符** —— 保证
//     stripPunct(Punctuated) == stripPunct(Text) == stripPunct(Raw)（保真类）；
//  2. **不动 Text、不动 Corrections** —— 结果只写
//     CorrectResult.Punctuated 与 CorrectResult.PunctuationCorrections，
//     从而 C4 的"没改 Text 不得有记录"不变式原样成立；
//  3. **已存在的标点一律保留** —— 不擅自改用户已有的东西（宁漏不错）。
//
// 诚实边界（RC7）：本实现只保证"结构可观测 + 正文安全"。
// 标点**质量**（分段是否自然、逗号位置是否符合语感）**无标注语料、未验证**，
// 不得把"有留痕"说成"标点自然"。
//
// 规则（刻意最小）：
//   - 句末无标点 → 补「。」；末字为疑问尾（吗/呢/嘛）→ 补「？」；
//   - 连接词（所以/但是/…）前且前一小句 ≥4 字、后接正文 → 补「，」。
package asr

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// commaMarkers 是"通常另起一小句"的连接词。刻意**不含**「就是」「然后」这类
// 在真句里常带实义/口语化的词（任务书 §5.2 告诫：填充/连接词表不要贪）。
var commaMarkers = []string{"所以", "但是", "不过", "而且", "另外", "因此", "可是", "否则"}

// isPunctRune 报告一个字符是否为标点（不含空白——空白交给上层，不在此处恢复）。
func isPunctRune(r rune) bool {
	switch r {
	case '。', '！', '？', '，', '、', '；', '：', '…', '—', '·', '～',
		'.', '!', '?', ',', ';', ':', '~',
		'“', '”', '‘', '’', '「', '」', '『', '』', '（', '）', '【', '】', '《', '》':
		return true
	}
	return false
}

// punctuate 返回标点恢复后的文本 + 逐条插入留痕。
// 无任何恢复时返回 (text, nil)，即 Punctuated == Text。
func punctuate(text string) (string, []Correction) {
	if text == "" {
		return "", nil
	}
	type insert struct {
		at   int // 插入点：Text 中的字节下标
		s    string
		conf float64
		why  string
	}
	var inserts []insert

	// 规则 1：连接词前补逗号。
	for _, m := range commaMarkers {
		for from := 0; ; {
			i := strings.Index(text[from:], m)
			if i < 0 {
				break
			}
			b := from + i
			from = b + len(m)
			if b == 0 {
				continue // 句首不补逗号
			}
			if len([]rune(text[:b])) < 4 {
				continue // 前一小句太短，切得太碎
			}
			prev, _ := utf8.DecodeLastRuneInString(text[:b])
			if isPunctRune(prev) || prev == ' ' || prev == '\t' {
				continue // 前面已有标点/空白
			}
			if r, _ := utf8.DecodeRuneInString(text[b+len(m):]); isPunctRune(r) {
				continue // 连接词后面直接是标点，不切
			}
			inserts = append(inserts, insert{
				at: b, s: "，", conf: 0.55,
				why: "标点恢复：连接词「" + m + "」前分句（只插标点，不改正文）",
			})
		}
	}

	// 规则 2：句末补句号/问号。
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
			continue // 防重复/越界
		}
		b.WriteString(text[prev:in.at])
		b.WriteString(in.s)
		corrs = append(corrs, Correction{
			Start: in.at, End: in.at, // 插入语义：Start == End
			From: "", To: in.s,
			Kind: "punctuation", Confidence: in.conf, Evidence: in.why,
		})
		prev = in.at
	}
	b.WriteString(text[prev:])
	return b.String(), corrs
}
