// edges_rules.go --    · semantic / causal rule     (P2 task). 
//
//   code disconnect,  call  out  type/  ;      Graph.Add(   heavy  , same from,to,rel
// get  heavy). temporal / entity  class already diffplace ly, basefileonlynewadd semantic and causal: 
//
//	semantic:  seg base"close word∪ body"   >=2 -> noto (Weight=0.6)
//	causal  : same  base inbecause linkconnectwordand  outorigbecause/close  side -> hasto (Weight=0.7)
//
// P0 safetyis rule(nosplitword /no NER), P2   type  sent attributionand body get. 
package zhiji

import "strings"

// causalWords because linkconnectwordtable(P0 frozen; and systemone.go Route  because wordis  ). 
var causalWords = []string{"因为", "所以", "导致", "使得", "因此", "于是", "从而", "鉴于", "造成"}

// effectLeadWords close side  word:  ini.e.pipe  sentfrom place become [origbecauseside | close side]. 
var effectLeadWords = []string{"所以", "因此", "于是", "从而", "导致", "使得", "造成"}

// causeLeadWords origbecauseside  word:  inthenitsaftercontinue sent asclose side. 
var causeLeadWords = []string{"因为", "鉴于"}

// clauseSplitterChars in /  tgtpt: by  pipe seg base become sent. 
const clauseSplitterChars = "，。！？；、,."

// cnStopRunes in stopusechar:  wordtime  outnowi.e. as boundary(andbase   inword/in body). 
// note : is" split boundary" is"deleteafter connect"-- then" day   " charafter  out voice bigram"day ". 
var cnStopRunes = map[rune]bool{
	'我': true, '你': true, '他': true, '她': true, '它': true, '们': true,
	'的': true, '了': true, '是': true, '在': true, '和': true, '与': true,
	'也': true, '就': true, '都': true, '还': true, '把': true, '被': true,
	'让': true, '向': true, '从': true, '到': true, '要': true, '会': true,
	'能': true, '个': true, '这': true, '那': true, '有': true, '不': true,
	'没': true, '很': true, '又': true, '或': true, '及': true, '之': true,
	'于': true, '而': true, '但': true, '其': true, '此': true, '该': true,
	'上': true, '下': true, '着': true, '过': true, '吗': true, '呢': true,
	'吧': true, '啊': true, '说': true, '去': true, '来': true, '做': true,
}

// enStopWords   stopuseword( write form  ; numchar and write   name accept table  ). 
var enStopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"was": true, "were": true, "and": true, "or": true, "of": true,
	"to": true, "in": true, "on": true, "for": true, "with": true,
	"at": true, "by": true, "from": true, "as": true, "it": true,
	"its": true, "this": true, "that": true, "be": true, "been": true,
}

// isCJK is  CJK   table  char(P0   ,      ). 
func isCJK(r rune) bool { return r >= 0x4e00 && r <= 0x9fff }

// extractKeywords from seg base "close word  "( heavy,     ): 
//
//	  : linkcontinuechar numchar  writeize,   >=2 and    stopusewordtable(numchar  230sar/5g   keep ); 
//	in : bytgtpt/  /numchar/in stopusechar sent, segintolinkcontinue char    (bigram). 
//	       stopusechar  bigram dayhowever produceoccur(stopusechari.e. splitpt),  stopusecharseg connect  . 
func extractKeywords(text string) []string {
	seen := map[string]struct{}{}
	add := func(w string) {
		if w != "" {
			seen[w] = struct{}{}
		}
	}
	var ascii strings.Builder
	flushASCII := func() {
		if ascii.Len() == 0 {
			return
		}
		w := strings.ToLower(ascii.String())
		ascii.Reset()
		if len(w) >= 2 && !enStopWords[w] {
			add(w)
		}
	}
	var cjk strings.Builder
	flushCJK := func() {
		if cjk.Len() == 0 {
			return
		}
		rs := []rune(cjk.String())
		cjk.Reset()
		for i := 0; i+1 < len(rs); i++ {
			add(string(rs[i : i+2])) //    alreadynostopusechar, bigram    
		}
	}
	for _, r := range text {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			flushCJK()
			ascii.WriteRune(r)
		case isCJK(r):
			if cnStopRunes[r] { // stopusechar= split boundary,   inword
				flushCJK()
				continue
			}
			flushASCII()
			cjk.WriteRune(r)
		default: // tgtpt/empty / id:  kind  all   
			flushASCII()
			flushCJK()
		}
	}
	flushASCII()
	flushCJK()
	out := make([]string, 0, len(seen))
	for w := range seen {
		out = append(out, w)
	}
	return out
}

// extractEntities from seg base " body  "(P0   ,  heavy): 
//
//	 write   name: linkcontinue >=2   writechar , e.g. STC / CPE / DNA; 
//	in  name seg: by  char/stopusechar seg, seg >=2  linkcontinue char ( seg   body). 
//
// P0   startsendform; P2   NER  type. 
func extractEntities(text string) []string {
	seen := map[string]struct{}{}
	var upper strings.Builder
	flushUpper := func() {
		if upper.Len() == 0 {
			return
		}
		w := upper.String()
		upper.Reset()
		if len(w) >= 2 {
			seen[w] = struct{}{}
		}
	}
	for _, r := range text {
		if r >= 'A' && r <= 'Z' {
			upper.WriteRune(r)
			continue
		}
		flushUpper()
	}
	flushUpper()
	for _, seg := range strings.FieldsFunc(text, func(r rune) bool { return !isCJK(r) }) {
		var buf []rune
		emit := func() {
			if len(buf) >= 2 {
				seen[string(buf)] = struct{}{}
			}
			buf = nil
		}
		for _, r := range []rune(seg) {
			if cnStopRunes[r] { // stopusecharplacedisconnectopen,   connect
				emit()
				continue
			}
			buf = append(buf, r)
		}
		emit()
	}
	out := make([]string, 0, len(seen))
	for w := range seen {
		out = append(out, w)
	}
	return out
}

// sharedUnionSize    seg base"close word∪ body" writeizeafter  heavy     num. 
//    name(STC) close wordside writeizeas stc,  bodyside writeizealsoas stc, and    and heavy  num. 
func sharedUnionSize(textA, textB string) int {
	setA := map[string]struct{}{}
	for _, w := range extractKeywords(textA) {
		setA[strings.ToLower(w)] = struct{}{}
	}
	for _, w := range extractEntities(textA) {
		setA[strings.ToLower(w)] = struct{}{}
	}
	n := 0
	touch := func(w string) {
		k := strings.ToLower(w)
		if _, ok := setA[k]; ok {
			n++
			delete(setA, k) //  heavy: same   only   
		}
	}
	for _, w := range extractKeywords(textB) {
		touch(w)
	}
	for _, w := range extractEntities(textB) {
		touch(w)
	}
	return n
}

// MaybeAddSemantic if A, B  seg base"close word+ body"and    >=2    , 
//    noto semantic  (Weight=0.6);  then   . empty base/samenodeptsafesafety   . 
func MaybeAddSemantic(g *Graph, idA, idB string, textA, textB string) {
	if g == nil || idA == "" || idB == "" || idA == idB {
		return
	}
	if sharedUnionSize(textA, textB) >= 2 {
		g.Add(Edge{From: idA, To: idB, Rel: EdgeRelSemantic, Directed: false, Weight: 0.6})
	}
}

// MaybeAddCausal if base inbecause linkconnectwordand  outorigbecause/close  side, 
// i.e. as fromNode(origbecauseside)   toNode(close side),    hasto causal  (Weight=0.7). 
//  tobycalluse by sentfirstaftergiveout;  andlist/nolinkconnectword/onlyhaslinkconnectwordno sidein  ->    . 
func MaybeAddCausal(g *Graph, fromNode, toNode string, text string) {
	if g == nil || fromNode == "" || toNode == "" || fromNode == toNode {
		return
	}
	if !hasAny(text, causalWords) { //  use systemone.go    write      disconnect
		return
	}
	if _, _, ok := splitCausalClauses(text); !ok { // onlyhaslinkconnectword, no sidein  ->   
		return
	}
	g.Add(Edge{From: fromNode, To: toNode, Rel: EdgeRelCausal, Directed: true, Weight: 0.7})
}

// splitClauses byin  tgtptpipe seg become sent( empty ,  emptyseg). 
func splitClauses(text string) []string {
	f := func(r rune) bool { return strings.ContainsRune(clauseSplitterChars, r) }
	var out []string
	for _, seg := range strings.FieldsFunc(text, f) {
		if seg = strings.TrimSpace(seg); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// stripConns    sent    linkconnectwordchar(P0   after  change  ). 
func stripConns(s string) string {
	for _, w := range causalWords {
		s = strings.ReplaceAll(s, w, "")
	}
	return strings.TrimSpace(s)
}

// splitCausalClauses firstbytgtpt  sent, again  linkconnectword  to senttime   [origbecauseside | close side]: 
//
//	close side  word( by/because /atis/frombut/  /  / become): 
//	    linkconnectword side(same sent first, emptythengeton  sent)= origbecauseside, linkconnectword side = close side. 
//	    example: " daystop ,     stop  day"-> cause= daystop  / effect=  stop  day. 
//	origbecauseside  word(becauseas/ at): linkconnectword side=origbecauseside, under  sent=close side. 
//	  all emptyonly ok=true; nolinkconnectword / onlyhaslinkconnectwordno side -> ok=false. 
//
// note : P0 isbytgtpt+linkconnectword     ,    sent storesplit ; P2   type   because attribution. 
func splitCausalClauses(text string) (cause, effect string, ok bool) {
	clauses := splitClauses(text)
	for i, c := range clauses {
		// close side  word: get     in
		p, pHit := -1, ""
		for _, conn := range effectLeadWords {
			if j := strings.Index(c, conn); j >= 0 && (p < 0 || j < p) {
				p, pHit = j, conn
			}
		}
		if p >= 0 {
			effect = stripConns(c[p+len(pHit):])
			cause = stripConns(c[:p])
			if cause == "" && i > 0 { // same sent sideasempty ->  on  sentcurorigbecauseside
				cause = stripConns(clauses[i-1])
			}
			return cause, effect, cause != "" && effect != ""
		}
		// origbecauseside  word: get     in
		q, qHit := -1, ""
		for _, conn := range causeLeadWords {
			if j := strings.Index(c, conn); j >= 0 && (q < 0 || j < q) {
				q, qHit = j, conn
			}
		}
		if q >= 0 {
			cause = stripConns(c[q+len(qHit):])
			if cause == "" && i > 0 {
				cause = stripConns(clauses[i-1])
			}
			effect = ""
			if i+1 < len(clauses) {
				effect = stripConns(clauses[i+1])
			}
			return cause, effect, cause != "" && effect != ""
		}
	}
	return "", "", false
}
