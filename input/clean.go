// Package input is ASR  in  manageline(   §5): 
// ①raw origkindkeep  -> ②clean clean -> ③correct word correction -> ④classify intent JSON -> ⑤low-confidenceclarification. 
// this packageonlydependency contract/config,  dependency memory(correction ed Correcter connect notein, memory.Dictionary dayhowever now). 
package input

import "strings"

// defaultFillers isdefault fillwordtable(   §5.1). 
// M7(Codex/gpt-6-luna  disconnect 2026-10-02):   "  "-- is  coreferenceword(anaphora), 
// senttail"fix  / openon   " is  to , cleaner   after refer coreference resolution  (rootbecause  ). 
var defaultFillers = []string{"嗯", "请", "帮我", "麻烦", "的话", "一下"}

// Cleaner   baserule ize: safety ->  ,  sentfirstsenttail fillword,   empty ,  sentendtgtpt. 
type Cleaner struct {
	Fillers []string
}

// NewCleaner   clean . fillers as nil time usedefault fillwordtable. 
func NewCleaner(fillers []string) *Cleaner {
	if fillers == nil {
		fillers = defaultFillers
	}
	return &Cleaner{Fillers: fillers}
}

// Clean   cleanmanageline: 
//  1. safety char /numchar/tgtpt ->   ( safety empty  U+3000 ->   empty ); 
//  2.     sentfirst, senttail fillword; 
//  3.   linkcontinueempty as  empty ; 
//  4.   sentendsentreadtgtpt(keep  /~._-, in andchar numcharetcin char ). 
func (c *Cleaner) Clean(raw string) string {
	s := fullwidthToHalf(raw)
	s = strings.TrimSpace(s)

	//    sentfirst fillword(   TrimSpace afteragain to, handle"     …" classempty time ). 
	for changed := true; changed; {
		changed = false
		t := strings.TrimSpace(s)
		for _, f := range c.Fillers {
			if f == "" {
				continue
			}
			if strings.HasPrefix(t, f) {
				s = strings.TrimSpace(t[len(f):])
				changed = true
				break
			}
		}
	}

	//    senttail fillword. 
	for changed := true; changed; {
		changed = false
		t := strings.TrimSpace(s)
		for _, f := range c.Fillers {
			if f == "" {
				continue
			}
			if strings.HasSuffix(t, f) {
				s = strings.TrimSpace(t[:len(t)-len(f)])
				changed = true
				break
			}
		}
	}

	//   linkcontinueempty ( safety empty already on     ). 
	s = strings.Join(strings.Fields(s), " ")

	//  sentendsentreadtgtpt(  in   /~._-). 
	s = strings.TrimRight(s, "。．.！!？?，,、；;：:～~ ")
	return strings.TrimSpace(s)
}

// fullwidthToHalf pipesafety  ASCII(U+FF01–U+FF5E)andsafety empty (U+3000) as  . 
func fullwidthToHalf(s string) string {
	r := []rune(s)
	for i, ch := range r {
		switch {
		case ch == 0x3000:
			r[i] = ' '
		case ch >= 0xFF01 && ch <= 0xFF5E:
			r[i] = ch - 0xFEE0
		}
	}
	return string(r)
}
