// lexicon.go -- wordtableobj (human  )+    +  line   status  . 
//
// word split class, **safesafetyetc  same**: 
//
//	auto = true   **  modifywrite**. only allowhuman  , has  lang  data, and  and name 
//	               risk obj( chartriggersendword audio,  disconnectalsoorig). curbeforeonlyhas  . 
//	auto = false  **onlyout  **(Candidate).  risk obj     route: 
//	               name/  /domainnamebe ASR   (deept->DeepSeek,    ->harness…), 
//	              byand audio audio  giveout   . origbecause: modify   modify (task  §5.2). 
//
// as   nameonlygive  but    : 
//   - lang  same  name  sendlanginhas kind  (obs-03 deept / deep sick), 
//         unique    data  ; 
//   - C1 keep needrequire   **observed**  sent  char   . 
//
//     samekindkeep : Observe  to   only  as  word ,     modifywrite. 
package asr

import (
	"errors"
	"sort"
	"strings"
)

var errEmptyFeedback = errors.New("asr: feedback 既无 raw 也无 corrected，无法学习")

// maxCandidates is   Correct   giveout   num,   pipe audiosafety giveon . 
const maxCandidates = 3

// entry is  wordtable obj. 
type entry struct {
	from, to string
	kind     string
	conf     float64
	evidence string
	auto     bool // true =   modifywrite; false = onlyout  

	// byunderby compile   from   , useat  word boundary  . 
	latinStart, latinEnd bool
}

// catalog isin obj : word  +  audio word + audionodetable +    word. 
type catalog struct {
	entries  []entry
	terms    []pinyinTerm
	table    map[rune]string
	hotwords []Hotword
}

// defaultCatalog returnbackin obj (human  ,      ). 
func defaultCatalog() catalog {
	return catalog{
		entries:  defaultEntries(),
		terms:    defaultPinyinTerms(),
		table:    buildPinyinTable(),
		hotwords: defaultHotwords(),
	}
}

// defaultEntries ishuman   wordtable.   allneed answer"lang    sent   ". 
func defaultEntries() []entry {
	return []entry{
		// ----   modifywrite( risk, haslang  data)----------------------------------
		{
			from: "题交", to: "提交", kind: "homophone", auto: true, conf: 0.92,
			evidence: "近音词表命中：题交(tí jiāo) → 提交(tí jiāo)；多字触发词，还原后整条链路才通（real-02）",
		},
		{
			from: "别字", to: "错别字", kind: "truncation", auto: true, conf: 0.80,
			evidence: "截断还原：别字 → 错别字（ASR 漏掉前字「错」）（real-03）；守卫见 truncationGuard",
		},

		// ---- onlyout  ( risk name/  ,     modifywrite)-----------------------
		{
			from: "deept", to: "DeepSeek", kind: "hotword", conf: 0.62,
			evidence: "专名候选：deept ≈ DeepSeek（obs-03；同一专名的另一种错法见 deep sick）",
		},
		{
			from: "deep sick", to: "DeepSeek", kind: "hotword", conf: 0.70,
			evidence: "专名候选：deep sick ≈ DeepSeek（obs-03/obs-18，同一错法重复出现=个性化词表可用信号）",
		},
		{
			from: "哈尼斯", to: "harness", kind: "hotword", conf: 0.60,
			evidence: "专名候选：哈尼斯 ≈ harness（obs-10，同一句内两种错法之一）",
		},
		{
			from: "汉尼斯", to: "harness", kind: "hotword", conf: 0.60,
			evidence: "专名候选：汉尼斯 ≈ harness（obs-10，同一句内两种错法之二）",
		},
		{
			from: "issanghannes", to: "harness", kind: "hotword", conf: 0.45,
			evidence: "专名候选：issanghannes ≈ harness（obs-19，撕碎到几乎不可复原，必须人工确认）",
		},
		{
			from: "voice sound harnessnes", to: "voice-sign harness", kind: "hotword", conf: 0.55,
			evidence: "专名候选：voice sound harnessnes ≈ voice-sign harness（obs-01）",
		},
		{
		},
		{
			from: "AR", to: "ASR", kind: "hotword", conf: 0.40,
			evidence: "专名候选：AR ≈ ASR（obs-10/obs-18）；仅 2 字母，易与普通缩写混淆，务必人工确认",
		},
	}
}

// defaultPinyinTerms is and audio audio   pos write . 
//    obj**onlyproduceout  **-- audio    back, but  by connectmodifywrite. 
func defaultPinyinTerms() []pinyinTerm {
	return []pinyinTerm{
		{canonical: "提交", syllables: []string{"ti", "jiao"}, conf: 0.55, note: "多字触发词"},
		{canonical: "通过", syllables: []string{"tong", "guo"}, conf: 0.50, note: "缺字候选（obs-14 登记类别）"},
		{canonical: "报价单", syllables: []string{"bao", "jia", "dan"}, conf: 0.55, note: "槽位专名"},
		{canonical: "库存", syllables: []string{"ku", "cun"}, conf: 0.50, note: "槽位专名"},
		{canonical: "测试", syllables: []string{"ce", "shi"}, conf: 0.45, note: "常用词"},
	}
}

// defaultHotwords isin  word(human  ,  is    ; SeenCnt no    ,    1). 
func defaultHotwords() []Hotword {
	return []Hotword{
		{Term: "voice-sign harness", Kind: "project", Weight: 1.0, SeenCnt: 1},
		{Term: "DeepSeek", Kind: "project", Weight: 1.0, SeenCnt: 1},
		{Term: "ASR", Kind: "term", Weight: 1.0, SeenCnt: 1},
		{Term: "提交", Kind: "command", Weight: 1.0, SeenCnt: 1},
		{Term: "错别字", Kind: "term", Weight: 1.0, SeenCnt: 1},
	}
}

// compile pipeword   becomeread-only     . 
func compile(entries []entry, c catalog) *compiled {
	es := make([]entry, len(entries))
	for i, e := range entries {
		if e.from != "" {
			rs := []rune(e.from)
			e.latinStart = isASCIIAlnum(rs[0])
			e.latinEnd = isASCIIAlnum(rs[len(rs)-1])
		}
		es[i] = e
	}
	return &compiled{
		trie:    newTrie(es),
		py:      newPinyinIndex(c.terms, c.table),
		entries: es,
	}
}

// detectAuto  out **  modifywrite**  time.     get  and ed    auto  obj. 
func (c *compiled) detectAuto(runes []rune) []span {
	var out []span
	for i := 0; i < len(runes); i++ {
		best, bestLen := -1, -1
		for _, k := range c.trie.matchAt(runes, i) {
			e := c.entries[k]
			if !e.auto {
				continue
			}
			n := len([]rune(e.from))
			end := i + n
			if e.latinStart && i > 0 && isASCIIAlnum(runes[i-1]) {
				continue
			}
			if e.latinEnd && end < len(runes) && isASCIIAlnum(runes[end]) {
				continue
			}
			if e.kind == "truncation" && !truncationGuard(runes, i, e) {
				continue
			}
			if n > bestLen {
				bestLen, best = n, k
			}
		}
		if best < 0 {
			continue
		}
		e := c.entries[best]
		out = append(out, span{
			start: i, end: i + bestLen, from: string(runes[i : i+bestLen]), to: e.to,
			kind: e.kind, conf: e.conf, evidence: e.evidence,
		})
		i += bestLen - 1
	}
	return out
}

// truncationGuard is disconnectalsoorig   :  pipealready isfinish pos word  segagain   
// ( then" diffchar"->"  diffchar"). 
func truncationGuard(runes []rune, i int, e entry) bool {
	if i > 0 && runes[i-1] == '错' {
		return false
	}
	target := []rune(e.to)
	if i+len(target) <= len(runes) && string(runes[i:i+len(target)]) == e.to {
		return false
	}
	return true
}

// detectCandidates produceout** modify base**   :  risk name +  audio audio  . 
//
//    empty = this layer  tableshow"   /  out ",  is C4        . 
func (c *compiled) detectCandidates(runes []rune, ctx []string) []Candidate {
	best := make(map[string]Candidate)
	add := func(text, reason string, conf float64) {
		if text == "" {
			return
		}
		if cur, ok := best[text]; ok && cur.Confidence >= conf {
			return
		}
		best[text] = Candidate{Text: text, Confidence: conf, Reason: reason}
	}

	// 1)  state  wordtable( name/  /domainname). 
	for i := 0; i < len(runes); i++ {
		for _, k := range c.trie.matchAt(runes, i) {
			e := c.entries[k]
			if e.auto {
				continue
			}
			n := len([]rune(e.from))
			end := i + n
			if e.latinStart && i > 0 && isASCIIAlnum(runes[i-1]) {
				continue
			}
			if e.latinEnd && end < len(runes) && isASCIIAlnum(runes[end]) {
				continue
			}
			add(e.to, e.evidence, e.conf)
		}
	}

	// 2)  audio audio  :     ,   safetychar  andaudionode inonlyproduceout  . 
	if c.py != nil {
		for i := 0; i < len(runes); i++ {
			for _, l := range c.py.lens {
				if i+l > len(runes) {
					break
				}
				key, ok := c.py.keyOf(runes[i : i+l])
				if !ok {
					continue
				}
				w := string(runes[i : i+l])
				for _, t := range c.py.byKey[key] {
					if w == t.canonical {
						continue // already ispos write ,   out  
					}
					reason := "拼音近音候选：" + w + "（" + strings.Join(t.syllables, " ") + "）≈ " + t.canonical
					if t.note != "" {
						reason += "；" + t.note
					}
					add(t.canonical, reason, t.conf)
				}
			}
		}
	}

	out := make([]Candidate, 0, len(best))
	for _, cand := range best {
		if contextHas(ctx, cand.Text) {
			cand.Confidence = minFloat(0.95, cand.Confidence+0.10)
			cand.Reason += "；近期上下文出现过，置信度上调"
		}
		out = append(out, cand)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].Text < out[j].Text
	})
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out
}

// contextHas    periodonunder  is outnowed word(useat    ,  modifychange base). 
func contextHas(ctx []string, term string) bool {
	for _, s := range ctx {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}
