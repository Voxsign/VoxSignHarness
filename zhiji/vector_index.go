// vector_index.go --    ·    useto     (M1 P1: TF-IDF/word  bot,   stdlib,  under   out  type). 
//
//   : model.go alreadyhas Embedding []float32 charsegbut empty; basefilefirstuse word  TF-IDF
//  out  back "to  pt", provide store.go   BudgetSearch andword kind   (hybridSeeds). 
// in no jieba dependency: by rune   unigram+bigram;   by write word . 
// DF instore  ; IDF    = log((N+1)/(df+1))+1; to  L2   izeafter   (   heavy,   ∈[0,1]). 
//   safety    i.e. (M1   ),    Save/Load JSON keep ize. 
package zhiji

import (
	"math"
	"os"
	"sort"
	"strings"
	"sync"
)

// ScoredNode to  back   close (nodeID +   splitnum, Search alreadybysplitnum  ). 
type ScoredNode struct {
	NodeID string  `json:"node_id"`
	Score  float64 `json:"score"`
}

// VectorIndex TF-IDF instoreto   (line safesafety;   origstart num + DF,   timenow   izeto ,    Upsert after IDF   ). 
type VectorIndex struct {
	mu   sync.RWMutex
	docs map[string]map[string]int // nodeID -> token -> origstartwordfreq num
	df   map[string]int            // token ->   freqrate(   token    num)
	n    int                        //   num(distinct nodeID; = len(docs))
}

// NewVectorIndex empty    (NewStore      Vec; calluse byneed New after to Store.Vec). 
func NewVectorIndex() *VectorIndex {
	return &VectorIndex{docs: map[string]map[string]int{}, df: map[string]int{}}
}

// Upsert write/changenew  nodept base; same nodeID heavywritefirstrollback  token   DF, again  new . 
// empty base asdelete nodept(  panic). 
func (v *VectorIndex) Upsert(nodeID, text string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if old, ok := v.docs[nodeID]; ok {
		for t := range old {
			v.df[t]--
			if v.df[t] <= 0 {
				delete(v.df, t)
			}
		}
	}
	toks := tokenizeText(text)
	if len(toks) == 0 {
		delete(v.docs, nodeID)
		v.n = len(v.docs)
		return
	}
	counts := make(map[string]int, len(toks))
	for _, t := range toks {
		counts[t]++
	}
	v.docs[nodeID] = counts
	for t := range counts {
		v.df[t]++
	}
	v.n = len(v.docs)
}

// Search    top-k(  splitnum  ); empty  /empty query safesafetyreturnback nil,   panic. 
func (v *VectorIndex) Search(query string, k int) []ScoredNode {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.n == 0 {
		return nil
	}
	qc := make(map[string]int)
	for _, t := range tokenizeText(query) {
		qc[t]++
	}
	if len(qc) == 0 {
		return nil
	}
	qv := v.vecOf(qc)
	type scored struct {
		id  string
		sco float64
	}
	var all []scored
	for id, dc := range v.docs {
		dv := v.vecOf(dc)
		var dot float64
		for t, w := range qv {
			dot += w * dv[t] // dv[t]   i.e. 0(Go map  value)
		}
		if dot > 0 {
			all = append(all, scored{id, dot})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].sco > all[j].sco })
	if k <= 0 || k > len(all) {
		k = len(all)
	}
	out := make([]ScoredNode, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, ScoredNode{NodeID: all[i].id, Score: all[i].sco})
	}
	return out
}

// Size already    num(provide  /  ). 
func (v *VectorIndex) Size() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.n
}

// ---- in : splitword / IDF /   izeto  ----

// idf      freqrate: log((N+1)/(df+1))+1(  >0,     and  heavy). 
func (v *VectorIndex) idf(t string) float64 {
	df := v.df[t]
	return math.Log(float64(v.n+1)/float64(df+1)) + 1
}

// vecOf byorigstartwordfreq num   L2   ize TF-IDF   to . 
func (v *VectorIndex) vecOf(counts map[string]int) map[string]float64 {
	w := make(map[string]float64, len(counts))
	var sum float64
	for t, c := range counts {
		x := float64(c) * v.idf(t)
		w[t] = x
		sum += x * x
	}
	if sum > 0 {
		inv := 1 / math.Sqrt(sum)
		for t := range w {
			w[t] *= inv
		}
	}
	return w
}

// tokenizeText   splitword: ASCII char numcharlinkbecome   writeword; its  split  rune byin char  
//   unigram + bigram; empty /tgtpt  boundary. noout dependency. 
func tokenizeText(text string) []string {
	var out []string
	var eng []rune
	var cjk []rune
	flushEng := func() {
		if len(eng) > 0 {
			out = append(out, strings.ToLower(string(eng)))
			eng = nil
		}
	}
	flushCJK := func() {
		for _, r := range cjk {
			out = append(out, string(r)) // unigram
		}
		for i := 0; i+1 < len(cjk); i++ {
			out = append(out, string(cjk[i:i+2])) // bigram
		}
		cjk = nil
	}
	isSep := func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '，', '。', '、', '？', '！', '；', '：',
			',', '.', '?', '!', ';', ':', '(', ')', '（', '）':
			return true
		}
		return false
	}
	for _, r := range text {
		if isASCIIAlnum(r) {
			flushCJK()
			eng = append(eng, r)
		} else if isSep(r) {
			flushEng()
			flushCJK()
		} else {
			flushEng()
			cjk = append(cjk, r)
		}
	}
	flushEng()
	flushCJK()
	return out
}

func isASCIIAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// ---- keep ize(  store.go writeJSON: tmp+rename orig write)----

type vectorIndexSnapshot struct {
	N    int                        `json:"n"`
	DF   map[string]int             `json:"df"`
	Docs map[string]map[string]int `json:"docs"`
}

// Save   (   vector.json;   same graph.go.Save). 
func (v *VectorIndex) Save(path string) error {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return writeJSON(path, vectorIndexSnapshot{N: v.n, DF: v.df, Docs: v.docs})
}

// Load from path   ; file store  asempty  (and graph.go.Load   ). 
func (v *VectorIndex) Load(path string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	snap := vectorIndexSnapshot{}
	if err := readJSON(path, &snap); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	v.n = snap.N
	v.df = snap.DF
	v.docs = snap.Docs
	if v.df == nil {
		v.df = map[string]int{}
	}
	if v.docs == nil {
		v.docs = map[string]map[string]int{}
	}
	return nil
}
