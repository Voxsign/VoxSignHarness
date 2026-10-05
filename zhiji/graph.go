// graph.go —— 知己 · 记忆关系图（架构 v1.1 §6.2 / G3 落地）。
//
// 四类有类型边（semantic|temporal|causal|entity）叠在同一组节点上；
// 节点 ID 复用 MemoryItem.ID / raw 观察 ID（raw-N）。图独立落 graph.json，
// 不往 MemoryItem 塞 edges（避免双向同步冗余）；空图时 BudgetSearch 退化为 Search 超集。
package zhiji

import (
	"os"
	"sync"
)

// EdgeRel 边关系类型（四视图：语义/时序/因果/实体）。
type EdgeRel string

const (
	EdgeRelSemantic EdgeRel = "semantic" // 语义相关
	EdgeRelTemporal EdgeRel = "temporal" // 时间邻近/先后
	EdgeRelCausal   EdgeRel = "causal"   // 因果（必有向）
	EdgeRelEntity   EdgeRel = "entity"   // 共享实体
)

// Edge 一条有类型边（From/To 为节点 ID；causal 必为 Directed=true）。
type Edge struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Rel      EdgeRel `json:"rel"`
	Directed bool    `json:"directed"` // causal 必为 true
	Weight   float64 `json:"weight"`   // 写入时概率（≥0.6 阈值）/ 时间邻近度
}

// Graph 关系图（线程安全；独立持久化 graph.json）。
type Graph struct {
	mu    sync.RWMutex
	Edges []Edge `json:"edges"`
}

// NewGraph 空图构造（NewStore load 时若 graph.json 不存在则为空图）。
func NewGraph() *Graph {
	return &Graph{}
}

// Add 追加一条边；去重键=(From,To,Rel)，同键已存在则加权更新（取较大权重），不重复追加。
func (g *Graph) Add(e Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := range g.Edges {
		old := &g.Edges[i]
		if old.From == e.From && old.To == e.To && old.Rel == e.Rel {
			if e.Weight > old.Weight {
				old.Weight = e.Weight
			}
			return
		}
	}
	g.Edges = append(g.Edges, e)
}

// Neighbors 返回与 id 相连的边（双向都算邻居）；可选按 rel 过滤（多 rel 同时命中）。
func (g *Graph) Neighbors(id string, rel ...EdgeRel) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	match := func(r EdgeRel) bool {
		if len(rel) == 0 {
			return true
		}
		for _, x := range rel {
			if x == r {
				return true
			}
		}
		return false
	}
	var out []Edge
	for _, e := range g.Edges {
		if !match(e.Rel) {
			continue
		}
		if e.From == id || e.To == id {
			out = append(out, e)
		}
	}
	return out
}

// IsEmpty 图是否无边（空图时 BudgetSearch 退化为 Search 超集）。
func (g *Graph) IsEmpty() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.Edges) == 0
}

// Load 从 path 加载图；文件不存在视为空图（与 store.go 其他四 JSON 一致）。
func (g *Graph) Load(path string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := readJSON(path, g); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

// Save 落盘 graph.json（风格照 store.go 的 writeJSON：tmp+rename 原子写）。
func (g *Graph) Save(path string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return writeJSON(path, g)
}
