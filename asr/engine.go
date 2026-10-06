// engine.go -- ASR  ityize diff  fastroute now(P2). 
//
// split (numdata  to,   only    ,      ): 
//
//	raw
//	 ├─ detectFillers      sentboundary fill   (   , see filler.go)
//	 ├─ detectAuto          statewordtable  modifywrite( audiotriggersendword /  disconnectalsoorig, see lexicon.go)
//	 ├─ resolve             time   resolve(   heavy )
//	 ├─ guard              empty   :     !=     -> origkindreturnback,  on clarification
//	 └─ rebuild            byorig charnode timeheavy  base +    Correction
//
//	and  route( modify base, onlygive  ,   / out confirm): 
//	 └─ detectCandidates    risk name +  audio audio  (see pinyin.go)
//
// statusand  split : 
//   - Correct tocurbeforefast is**  num**: read-only, no  use,  andsend; 
//   -  ityizestatusonlyby Observe modifywrite, fast use atomic.Pointer  body  (writeerserial, readerno ); 
//   - Lexicon  out   fast . 
package asr

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// span is place**  use** modifywrite,  tgtis rune undertgt [start,end). 
//  endtoout    Correction  tgtischarnodeundertgt(by runeOffsets   ). 
type span struct {
	start, end int
	from       string
	to         string
	kind       string
	conf       float64
	evidence   string
}

// compiled is    after read-only     . wordtablechangeize = heavynew   +  body refer , 
//     Correct path modify    close . 
type compiled struct {
	trie    *trie
	py      *pinyinIndex
	entries []entry
}

// snapshot is    change  ityizestatus base. 
type snapshot struct {
	compiled *compiled
	hotwords []Hotword
	version  string
}

// Personalized is Engine  occurproduce now. 
//
// charsegsplit class,     : 
//   - state  : read-onlyfast , Correct pathunique     (no ); 
//   - its    : writeerstatus, only  mu inmodify, modifyfinish bodyheavy  becomenewfast . 
type Personalized struct {
	state atomic.Pointer[snapshot]

	mu       sync.Mutex
	catal    catalog
	learned  []entry // onlyby learn   writeback(LEARN-02+; curbefore asempty)
	seen     map[string]*Hotword
	evidence []Evidence // Observe  artifact: only  data,   writeback
	rev      int
}

// Evidence is  rev  origstart data. 
//
// as    store : ASR-MODEL-02   L2 rule "onlyhas `learn`    bywritebackkeep   ". 
// atis `Observe` **  asonly  data**--   "useusermodify  , confirmand ,   andtimetime", 
// but produceoccur    changechange;  pos   by `learn`    after     data(LEARN-02+). 
type Evidence struct {
	At        string `json:"at"`
	Raw       string `json:"raw"`
	Corrected string `json:"corrected"`
	Accepted  bool   `json:"accepted"`
	Source    string `json:"source"`
}

// evidenceCap isinstore data num onlimit( line #4:  noboundary  ). 
//  outafter      data; keep ize databy learn    to usage-events.jsonl(P2). 
const evidenceCap = 4096

// NewEngine   default  : in human   wordtable + empty   status. 
func NewEngine() *Personalized {
	p := &Personalized{catal: defaultCatalog(), seen: make(map[string]*Hotword)}
	p.rebuildLocked()
	return p
}

var _ Engine = (*Personalized)(nil)

// rebuildLocked pipe(in obj  +     )heavy  as  newfast andorig  in. 
// calluseer  keephas p.mu(  period out). 
func (p *Personalized) rebuildLocked() {
	entries := make([]entry, 0, len(p.catal.entries)+len(p.learned))
	entries = append(entries, p.catal.entries...)
	entries = append(entries, p.learned...)
	comp := compile(entries, p.catal)

	hw := make([]Hotword, 0, len(p.catal.hotwords)+len(p.seen))
	hw = append(hw, p.catal.hotwords...)
	for _, h := range p.seen {
		if h.Weight <= 0 {
			continue
		}
		hw = append(hw, *h)
	}
	sort.Slice(hw, func(i, j int) bool {
		if hw[i].Weight != hw[j].Weight {
			return hw[i].Weight > hw[j].Weight
		}
		return hw[i].Term < hw[j].Term
	})

	p.rev++
	p.state.Store(&snapshot{
		compiled: comp,
		hotwords: hw,
		version:  "vhs-asr/p2." + strconv.Itoa(p.rev),
	})
}

// Correct  pos   ASR origstart base. read-onlyfast , no  use,  andsend. 
func (p *Personalized) Correct(req CorrectRequest) CorrectResult {
	start := time.Now()
	raw := req.Raw
	res := CorrectResult{Text: raw}
	if raw == "" {
		res.Latency = time.Since(start)
		return res
	}

	comp := p.state.Load().compiled
	runes := []rune(raw)
	offs := runeOffsets(raw)

	// noin   (C3):  sentonlyhas fillword/coreferenceword + tgtptempty  ->  is"    ", 
	// butis"   /  in ".  base char  ,  ed Candidates sendout**clarificationsignal**
	// (asr.go:40: Candidates  empty =    /  out ).       . 
	if pureNoise(runes) {
		res.Candidates = []Candidate{{
			Text:       raw, //   providemodifywrite  : origkindisunique read 
			Confidence: 0,   //     =  has  modifywrite
			Reason:     askNoiseReasonPrefix + " —— 整句只有填充词/指代词，无可用内容，需上层回问澄清（引擎不猜测）",
		}}
		res.Punctuated = raw // noin    tgtpt
		res.Latency = time.Since(start)
		return res
	}

	applied := detectFillers(runes)
	applied = append(applied, comp.detectAuto(runes)...)
	applied = resolve(applied, len(runes))

	if len(applied) > 0 {
		text, corrs := rebuild(runes, offs, applied)
		// empty   : pipe sentall   is"   ", is"pipe    ". 
		//   origkindreturnback, pipe disconnect backon (C3:  voice   giveon clarification). 
		if text != raw && strings.TrimSpace(text) != "" {
			res.Text = text
			res.Corrections = corrs
		}
	}

	// tgtpt  (needrequire 4.3 / C1 v2): close onlywrite Punctuated + PunctuationCorrections, 
	// **  modify Text,     Corrections**--keep  C4  " modify Text   has  " changeform. 
	res.Punctuated, res.PunctuationCorrections = punctuate(res.Text)

	//    route: only  ,  modify base;  emptyi.e.tableshow"   /  out ". 
	res.Candidates = comp.detectCandidates(runes, req.Context)
	res.Latency = time.Since(start)
	return res
}

// Observe back   rev . by ASR-MODEL-02   L2, ** only  data,  writeback  **: 
//   -      Evidence( ,  time, modify  , is confirm,   ); 
//   -  word heavyonly freq   , provide Lexicon   ; 
//   - ** again**  /    word  -- writebackkeep    unique  is `learn`(LEARN-02+). 
//
//  kind"  change "onlyhas     ,  close  in ,  linerev    become       . 
func (p *Personalized) Observe(fb Feedback) error {
	if strings.TrimSpace(fb.Raw) == "" && strings.TrimSpace(fb.Corrected) == "" {
		return errEmptyFeedback
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.evidence = append(p.evidence, Evidence{
		At:        time.Now().UTC().Format(time.RFC3339Nano),
		Raw:       fb.Raw,
		Corrected: fb.Corrected,
		Accepted:  fb.Accepted,
		Source:    fb.Source,
	})
	if len(p.evidence) > evidenceCap {
		p.evidence = p.evidence[len(p.evidence)-evidenceCap:]
	}

	if term := strings.TrimSpace(fb.Corrected); term != "" {
		h := p.seen[term]
		if h == nil {
			h = &Hotword{Term: term, Kind: "term"}
			p.seen[term] = h
		}
		if fb.Accepted {
			h.Weight++
			h.SeenCnt++
		} else if h.Weight > 0 {
			h.Weight--
		}
	}

	p.rebuildLocked()
	return nil
}

// Evidence  outrev  datafast (   ).  is `learn`     in,  is  base . 
func (p *Personalized) Evidence() []Evidence {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Evidence, len(p.evidence))
	copy(out, p.evidence)
	return out
}

// Lexicon  outcurbefore ityizestatus    fast (   , out change   in status). 
func (p *Personalized) Lexicon(domain string) Lexicon {
	s := p.state.Load()
	hw := make([]Hotword, len(s.hotwords))
	copy(hw, s.hotwords)
	return Lexicon{Domain: domain, Hotwords: hw, Version: s.version}
}

// ---------------------------------------------------------------------------
// in   
// ---------------------------------------------------------------------------

// runeOffsets returnback   rune  raisestartcharnodeundertgt, endtailpatch len(s), 
// atis rune  time [a,b) to charnode time [offs[a], offs[b]). 
func runeOffsets(s string) []int {
	offs := make([]int, 0, len(s)+1)
	for i := range s {
		offs = append(offs, i)
	}
	offs = append(offs, len(s))
	return offs
}

// resolve ed   /empty   span, by    and  heavy (keep firstoutnowandchange er). 
func resolve(spans []span, n int) []span {
	out := make([]span, 0, len(spans))
	for _, s := range spans {
		if s.start < 0 || s.end > n || s.start >= s.end || s.from == s.to {
			continue
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].start != out[j].start {
			return out[i].start < out[j].start
		}
		return out[i].end > out[j].end
	})
	var res []span
	for _, s := range out {
		if n := len(res); n > 0 && s.start < res[n-1].end {
			continue
		}
		res = append(res, s)
	}
	return res
}

// rebuild by span heavy  base, andproduceoutcharnode time back   Correction. 
func rebuild(runes []rune, offs []int, spans []span) (string, []Correction) {
	var b strings.Builder
	var corrs []Correction
	prev := 0
	for _, s := range spans {
		if s.start < prev || s.end > len(runes) {
			continue
		}
		b.WriteString(string(runes[prev:s.start]))
		b.WriteString(s.to)
		corrs = append(corrs, Correction{
			Start:      offs[s.start],
			End:        offs[s.end],
			From:       string(runes[s.start:s.end]),
			To:         s.to,
			Kind:       s.kind,
			Confidence: s.conf,
			Evidence:   s.evidence,
		})
		prev = s.end
	}
	b.WriteString(string(runes[prev:]))
	return b.String(), corrs
}

// byunder   helper provide **learn    writebackpath** use(LEARN-02+, etc typeand    ). 
// by L2 onlyhas learn  calluse  ; `Observe` already againcalluse( only  data), 
// because objbefore hascallusept-- is   ,  is  code. 

// upsertLearned   / ize    word (same from->to only   and  ). 
func upsertLearned(list []entry, e entry) []entry {
	for i := range list {
		if list[i].from == e.from && list[i].to == e.to {
			list[i].conf = minFloat(0.99, list[i].conf+0.05)
			return list
		}
	}
	return append(list, e)
}

func removeLearned(list []entry, from, to string) []entry {
	out := list[:0]
	for _, e := range list {
		if e.from == from && e.to == to {
			continue
		}
		out = append(out, e)
	}
	return out
}

func sourceConf(src string) float64 {
	switch src {
	case "user_edit":
		return 0.90
	case "reviewer":
		return 0.95
	case "implicit":
		return 0.60
	default:
		return 0.70
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
