// vector_index.go —— 知己 · 最小可用向量索引骨架（M1 P1：TF-IDF/词袋兜底，纯 stdlib，不下载任何外部模型）。
//
// 定位：model.go 已有 Embedding []float32 字段但留空；本文件先用纯词袋 TF-IDF
// 做出可召回的「向量锚点」，供 store.go 的 BudgetSearch 与词法种子混合（hybridSeeds）。
// 中文无 jieba 依赖：按 rune 切 unigram+bigram；英文按小写单词切。
// DF 内存累计；IDF 平滑 = log((N+1)/(df+1))+1；向量 L2 归一化后算余弦（非负权重，余弦∈[0,1]）。
// 小库全量算余弦即可（M1 量级），预留 Save/Load JSON 持久化。
package zhiji

import (
	"math"
	"os"
	"sort"
	"strings"
	"sync"
)

// ScoredNode 向量召回的一条结果（nodeID + 余弦分数，Search 已按分数降序）。
type ScoredNode struct {
	NodeID string  `json:"node_id"`
	Score  float64 `json:"score"`
}

// VectorIndex TF-IDF 内存向量索引（线程安全；文档原始计数 + DF，检索时现算归一化向量，避免 Upsert 后 IDF 漂移）。
type VectorIndex struct {
	mu   sync.RWMutex
	docs map[string]map[string]int // nodeID -> token -> 原始词频计数
	df   map[string]int            // token -> 文档频率（含该 token 的文档数）
	n    int                        // 文档数（distinct nodeID；= len(docs)）
}

// NewVectorIndex 空索引构造（NewStore 不自动建 Vec；调用方按需 New 后挂到 Store.Vec）。
func NewVectorIndex() *VectorIndex {
	return &VectorIndex{docs: map[string]map[string]int{}, df: map[string]int{}}
}

// Upsert 写入/更新一个节点文本；同 nodeID 重写先回滚旧 token 的 DF，再累计新的。
// 空文本视为删除该节点（不 panic）。
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

// Search 查询 top-k（余弦分数降序）；空索引/空 query 安全返回 nil，不 panic。
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
			dot += w * dv[t] // dv[t] 缺键即 0（Go map 零值）
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

// Size 已索引文档数（供监控/测试）。
func (v *VectorIndex) Size() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.n
}

// ---- 内部：分词 / IDF / 归一化向量 ----

// idf 平滑逆文档频率：log((N+1)/(df+1))+1（恒 >0，避免除零与零权重）。
func (v *VectorIndex) idf(t string) float64 {
	df := v.df[t]
	return math.Log(float64(v.n+1)/float64(df+1)) + 1
}

// vecOf 由原始词频计数构造 L2 归一化 TF-IDF 稀疏向量。
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

// tokenizeText 统一分词：ASCII 字母数字连成英文小写词；其余非分隔 rune 按中文字符流
// 吐 unigram + bigram；空白/标点作边界。无外部依赖。
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

// ---- 持久化（照 store.go writeJSON：tmp+rename 原子写）----

type vectorIndexSnapshot struct {
	N    int                        `json:"n"`
	DF   map[string]int             `json:"df"`
	Docs map[string]map[string]int `json:"docs"`
}

// Save 落盘（独立 vector.json；风格同 graph.go.Save）。
func (v *VectorIndex) Save(path string) error {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return writeJSON(path, vectorIndexSnapshot{N: v.n, DF: v.df, Docs: v.docs})
}

// Load 从 path 加载；文件不存在视为空索引（与 graph.go.Load 一致）。
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
