// filler.go —— 填充词清理的**守卫式**实现。
//
// 为什么单独成层：这是全层最容易闯祸的地方（任务书 §5.2：DSH 起草时当场踩过
// 「就是这里时候要不要用deept的」被删掉「就是」的过度纠正）。
//
// 本层采用三条互锁的守卫，宁可漏纠，不可纠错：
//
//	G1 只认**非词汇性感叹词**（嗯/呃/哦/哎/唉/诶/唔）为"可删信号"。
//	   「就是」「然后」**不在词表内**——它们在真句里常带实义。
//	   「这个/那个」是指示代词，默认**不删**。
//
//	G2 删除必须发生在**句界**（句首，或紧跟标点/空白之后）。
//	   句中「报价页那个别字」的「那个」不满足 G2，因此天然保住。
//
//	G3 指示代词（这个/那个）只有在**被填充词夹住**（前后都是填充词）时才随串删除。
//	   「嗯这个功能」里「这个」是实义定语 → 只删「嗯」。
//
// 另有一条全局守卫在 engine.go：删光后为空 → 整句作废（噪声交给上层回问）。
package asr

// interjections 是**非词汇性**感叹词：删掉不承担语义。已刻意排除「啊」——
// 它在句尾常是语气助词（"好啊"），风险高于收益。
var interjections = []string{"嗯", "呃", "哦", "哎", "唉", "诶", "唔"}

// demonstrativeFillers 是指示代词：只有被填充词夹住才删（G3）。
var demonstrativeFillers = []string{"那个", "这个"}

// detectFillers 扫描全文，返回可删的填充串。
func detectFillers(runes []rune) []span {
	var out []span
	i := 0
	for i < len(runes) {
		// G2：只有句界位置才允许起一个填充串。
		if i != 0 && !isBoundary(runes[i-1]) {
			i++
			continue
		}
		spans, runEnd, hasInterj := scanFillerRun(runes, i)
		if !hasInterj || len(spans) == 0 {
			// G1：整串没有感叹词 → 不是填充串（例如「那个那个那个」）。
			if runEnd > i {
				i = runEnd
			} else {
				i++
			}
			continue
		}
		if i == 0 {
			spans = absorbLeadingClausePunct(runes, spans, runEnd)
		}
		out = append(out, mergeAdjacent(spans)...)
		if runEnd > i {
			i = runEnd
		} else {
			i++
		}
	}
	return out
}

// scanFillerRun 从 i 起尽量长地吃掉连续填充词，返回可删 span 与串尾位置。
func scanFillerRun(runes []rune, i int) (spans []span, runEnd int, hasInterj bool) {
	type tok struct {
		start, end int
		interj     bool
	}
	var toks []tok
	pos := i
	for pos < len(runes) {
		n, interj := matchFillerWord(runes[pos:])
		if n == 0 {
			break
		}
		toks = append(toks, tok{start: pos, end: pos + n, interj: interj})
		if interj {
			hasInterj = true
		}
		pos += n
	}
	runEnd = pos
	for k, t := range toks {
		remove := t.interj
		if !t.interj {
			// G3：指示代词仅在被前后填充词夹住时删除。
			remove = k > 0 && k < len(toks)-1
		}
		if !remove {
			continue
		}
		from := string(runes[t.start:t.end])
		spans = append(spans, span{
			start: t.start, end: t.end, from: from, to: "",
			kind: "filler", conf: 0.95,
			evidence: "填充词「" + from + "」位于句界填充串中（G1/G2/G3 守卫通过）",
		})
	}
	return spans, runEnd, hasInterj
}

// matchFillerWord 在 runes 开头匹配一个填充词，返回其 rune 数与是否为感叹词。
func matchFillerWord(runes []rune) (n int, interj bool) {
	for _, w := range interjections {
		if wr := []rune(w); len(runes) >= len(wr) && string(runes[:len(wr)]) == w {
			return len(wr), true
		}
	}
	for _, w := range demonstrativeFillers {
		if wr := []rune(w); len(runes) >= len(wr) && string(runes[:len(wr)]) == w {
			return len(wr), false
		}
	}
	return 0, false
}

// absorbLeadingClausePunct 处理句首填充串后紧跟的从句标点：
// 「嗯，有个…」→ 删「嗯，」而不是留下孤零零的「，」。
// 仅当 0..runEnd 被整段删除时才吞标点，且只吞从句标点（不吞句末 。！？）。
func absorbLeadingClausePunct(runes []rune, spans []span, runEnd int) []span {
	if len(spans) == 0 || spans[0].start != 0 {
		return spans
	}
	covered := 0
	for _, s := range spans {
		if s.start == covered {
			covered = s.end
		} else {
			break
		}
	}
	if covered != runEnd {
		return spans
	}
	j := runEnd
	for j < len(runes) && isClausePunct(runes[j]) {
		j++
	}
	if j > runEnd {
		spans[0].end = j
		spans[0].from = string(runes[spans[0].start:j])
	}
	return spans
}

// mergeAdjacent 合并首尾相接的 span。
func mergeAdjacent(spans []span) []span {
	if len(spans) <= 1 {
		return spans
	}
	out := spans[:1]
	for _, s := range spans[1:] {
		last := &out[len(out)-1]
		if s.start == last.end {
			last.end = s.end
			last.from = last.from + s.from
			continue
		}
		out = append(out, s)
	}
	return out
}

// isBoundary 报告一个 rune 是否构成"句界"（句读标点或空白）。
// G2 用它把句中指示代词挡在门外。
func isBoundary(r rune) bool {
	switch r {
	case '。', '！', '？', '，', '、', '；', '：',
		'.', '!', '?', ',', ';', ':', '…', '—',
		'“', '”', '‘', '’', '（', '）', '【', '】', '「', '」', '『', '』',
		' ', '\t', '\n', '\r':
		return true
	}
	return false
}

// isClausePunct 报告一个 rune 是否为**从句**标点（可随句首填充词一并吞掉）。
func isClausePunct(r rune) bool {
	switch r {
	case '，', '、', '；', '：', ',', ';', ':', ' ', '\t':
		return true
	}
	return false
}

// askNoiseReasonPrefix 是"无内容、需回问"信号的**稳定机器可判前缀**。
// 上层据此回问；判据可据此精确断言（见 engine.go Correct 的无内容守卫）。
const askNoiseReasonPrefix = "ask:noise"

// pureNoise 报告整句是否**只由填充词/指示代词 + 标点空白构成**（即无可用内容）。
//
// 用途：C3 的"该回问"信号。它只认"一个字都没有实义"的极窄情形——
// 任何实义词、数字、拉丁字母出现即返回 false，因此不会把正常短句判成噪声
// （C3 的反向反例：real-15 这类长句必须 ask_empty）。
func pureNoise(runes []rune) bool {
	fillers := 0
	for i := 0; i < len(runes); {
		if isBoundary(runes[i]) {
			i++
			continue
		}
		n, _ := matchFillerWord(runes[i:])
		if n == 0 {
			return false
		}
		fillers++
		i += n
	}
	return fillers > 0
}
