// filler.go --  fillword   **  form** now. 
//
// as    become :  issafety       ly (task  §5.2: DSH raise timecur  ed
// "thenis  time need needusedeept "be  "thenis" ed  pos). 
//
// this layer use       ,     ,   correction: 
//
//	G1 only ** word ity  word**( / / / / / / )as"  signal". 
//	   "thenis""howeverafter"**  wordtablein**--    sent     . 
//	   "  /  "isrefershow word, default**  **. 
//
//	G2 delete  sendoccur **sentboundary**(sentfirst, or  tgtpt/empty ofafter). 
//	   sentin"     diffchar" "  " full  G2, because dayhoweverkeep . 
//
//	G3 refershow word(  /  )onlyhas **be fillword  **(beforeafterallis fillword)timeonly  delete. 
//	   "     " "  "is   lang -> only " ". 
//
//  has  global    engine.go:   afterasempty ->  sent  ( voice giveon clarification). 
package asr

// interjections is** word ity**  word:      semantic. already    " "--
//   senttail islang  word("  "), risk atrecv . 
var interjections = []string{"嗯", "呃", "哦", "哎", "唉", "诶", "唔"}

// demonstrativeFillers isrefershow word: onlyhasbe fillword  only (G3). 
var demonstrativeFillers = []string{"那个", "这个"}

// detectFillers   safety , returnback    fill . 
func detectFillers(runes []rune) []span {
	var out []span
	i := 0
	for i < len(runes) {
		// G2: onlyhassentboundary  only allowraise   fill . 
		if i != 0 && !isBoundary(runes[i-1]) {
			i++
			continue
		}
		spans, runEnd, hasInterj := scanFillerRun(runes, i)
		if !hasInterj || len(spans) == 0 {
			// G1:    has  word ->  is fill (examplee.g."      "). 
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

// scanFillerRun from i raise   ly  linkcontinue fillword, returnback   span and tail  . 
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
			// G3: refershow wordonly bebeforeafter fillword  timedelete. 
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

// matchFillerWord   runes openhead     fillword, returnbackits rune numandis as  word. 
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

// absorbLeadingClausePunct handlesentfirst fill after   fromsenttgtpt: 
// " , has …"->  " , "but is under    ", ". 
// onlycur 0..runEnd be segdeletetimeonly tgtpt, andonly fromsenttgtpt(  sentend .   ). 
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

// mergeAdjacent  andfirsttail connect  span. 
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

// isBoundary      rune is  become"sentboundary"(sentreadtgtptorempty ). 
// G2 use pipesentinrefershow word   out. 
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

// isClausePunct      rune is as**fromsent**tgtpt(  sentfirst fillword and  ). 
func isClausePunct(r rune) bool {
	switch r {
	case '，', '、', '；', '：', ',', ';', ':', ' ', '\t':
		return true
	}
	return false
}

// askNoiseReasonPrefix is"noin , needclarification"signal **      before **. 
// on data clarification;  data data   disconnectlang(see engine.go Correct  noin   ). 
const askNoiseReasonPrefix = "ask:noise"

// pureNoise    sentis **onlyby fillword/refershow word + tgtptempty  become**(i.e.no usein ). 
//
// useway: C3  " clarification"signal.  only "  charall has  "   case --
//     word, numchar,   char outnowi.e.returnback false, because   pipepos  sent become voice
// (C3  revtorevexample: real-15  class sent   ask_empty). 
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
