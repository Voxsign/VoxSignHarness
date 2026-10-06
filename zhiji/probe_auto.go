// probe_auto.go --    · A4 detail survival       (M1 v1.3 P0 rule ). 
//
//   : OnTaskEnd  trace  after, frombasetask base     2–3  "   close  node"
//   become SurvivalProbe.       = baseseg base onlyoutnow 1    body token
// (numchar/ name), but rev  call   word--  use     after nodeis also (   ). 
//
//   stdlib, no type, noout dependency; and compress.go   markProbeHits( backside)becometo: 
//   responsible"  ",     in KeyDetail timecall ProbeRecall tgt back. 
package zhiji

import (
	"regexp"
	"strings"
)

// probeTokenRe       ,   pipe "230SAR"    "SAR" curbecome   write nameheavy  . 
//   - \d+[A-Za-z%./]*  numchar/num word(230SAR / 5G / 1204 / 429 / 30min / 3.5%)
//   - [A-Z][A-Za-z]+    write   name(STC / Zain / Leap; >=2  char )
var probeTokenRe = regexp.MustCompile(`[0-9]+[A-Za-z%./]*|[A-Z][A-Za-z]+`)

// probeStopwords  use freqstopuseword(  ize writeafter  ). P0 onlyrecv  see ; 
//  name(STC/    )andnumchar(230SAR)dayhowever  table . 
var probeStopwords = map[string]bool{
	//   
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"this": true, "that": true, "and": true, "or": true, "to": true, "of": true,
	"in": true, "on": true, "for": true, "with": true, "task": true,
	"success": true, "failed": true, "retried": true, "judge": true,
	"cheap": true, "premium": true, "mid": true,
	// in  useword(  name; bigram   )
	"任务": true, "我们": true, "这个": true, "那个": true, "进行": true,
	"需要": true, "成功": true, "失败": true, "一个": true, "没有": true,
	"就是": true, "还是": true, "不是": true, "可以": true, "什么": true,
}

// extractLowSalienceDetails fromtask base   n  "   close  node"(P0 rule ). 
//
//   rule( end  v1.3.1): 
//  1.    token  class: ① numchar/num word(raisestartasnumchar,   char / idafter : 230SAR/5G/1204/429)
//     ②  write   name(firstchar  writeand >=2 char : STC/Zain/Leap)
//     ③ in    n-gram(2-gram + 3-gram:   /  /   ,  connect NER/splitword ). 
//  2. in  n-gram firstedstopusechared :     cnStopRunes( //is/ / / / / /…) connect . 
//  3.  baseseg basein      outnow num; onlykeep   outnow 1   (   ,  rev  call). 
//  4.  stopuseword(   use freqword);   by write   . 
//  5. by orig first outnowfirstafter  returnbackbefore n  ; same (   ) heavy. 
//
// empty base/no  timereturnback nil,   panic. 
func extractLowSalienceDetails(text string, n int) []string {
	if n <= 0 || strings.TrimSpace(text) == "" {
		return nil
	}
	type tally struct {
		freq    int
		surface string // firstnowwrite ( showuse)
	}
	var order []string //    firstnow  
	t := map[string]*tally{}
	add := func(surface string) {
		norm := strings.ToLower(surface)
		if _, ok := t[norm]; !ok {
			order = append(order, norm)
			t[norm] = &tally{surface: surface}
		}
		t[norm].freq++
	}

	// ① numcharword + ②  write   name(  posthen    ,    230SAR    SAR be   to)
	for _, m := range probeTokenRe.FindAllString(text, -1) {
		add(m)
	}
	// ③ in    2-gram + 3-gram;  stopusechar  n-gram  connect . 
	var cjk []rune
	flushCJK := func() {
		for size := 2; size <= 3; size++ {
			for i := 0; i+size <= len(cjk); i++ {
				g := string(cjk[i : i+size])
				if hasStopRune(g) {
					continue
				}
				add(g)
			}
		}
		cjk = nil
	}
	for _, r := range text {
		if isCJK(r) {
			cjk = append(cjk, r)
		} else {
			flushCJK()
		}
	}
	flushCJK()

	// byfirstnow   : freq ==1 and stopuseword. 
	out := make([]string, 0, n)
	for _, norm := range order {
		if t[norm].freq != 1 || probeStopwords[norm] {
			continue
		}
		out = append(out, t[norm].surface)
		if len(out) >= n {
			break
		}
	}
	return out
}

// hasStopRune n-gram is    in stopusechar( use edges_rules.go   cnStopRunes). 
func hasStopRune(g string) bool {
	for _, r := range g {
		if cnStopRunes[r] {
			return true
		}
	}
	return false
}

// taskEndCorpus pipe   CallLog  becomeprovide   get lang (TaskProfile as , DetailProbe as ). 
func taskEndCorpus(log CallLog) string {
	return strings.Join([]string{log.TaskProfile, log.DetailProbe, log.Model, log.Outcome}, " ")
}
