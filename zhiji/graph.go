// graph.go --    ·   close  (   v1.1 §6.2 / G3  ly). 
//
//  classhasclasstype (semantic|temporal|causal|entity)  same  nodepton; 
// nodept ID  use MemoryItem.ID / raw    ID(raw-N).      graph.json, 
//    MemoryItem   edges(   tosame   ); empty time BudgetSearch  izeas Search   . 
package zhiji

import (
	"os"
	"sync"
)

// EdgeRel  close classtype(   : semantic/time /because / body). 
type EdgeRel string

const (
	EdgeRelSemantic EdgeRel = "semantic" // semantic close
	EdgeRelTemporal EdgeRel = "temporal" // timetime  /firstafter
	EdgeRelCausal   EdgeRel = "causal"   // because ( hasto)
	EdgeRelEntity   EdgeRel = "entity"   //    body
)

// Edge   hasclasstype (From/To asnodept ID; causal  as Directed=true). 
type Edge struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Rel      EdgeRel `json:"rel"`
	Directed bool    `json:"directed"` // causal  as true
	Weight   float64 `json:"weight"`   // writetime rate(>=0.6  value)/ timetime   
}

// Graph close  (line safesafety;   keep ize graph.json). 
type Graph struct {
	mu    sync.RWMutex
	Edges []Edge `json:"edges"`
}

// NewGraph empty   (NewStore load timeif graph.json  store thenasempty ). 
func NewGraph() *Graph {
	return &Graph{}
}

// Add      ;  heavy =(From,To,Rel), same alreadystore then  changenew(get   heavy),  heavy   . 
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

// Neighbors returnbackand id  link  ( toall   );   by rel ed (  rel sametime in). 
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

// IsEmpty  is no (empty time BudgetSearch  izeas Search   ). 
func (g *Graph) IsEmpty() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.Edges) == 0
}

// Load from path    ; file store  asempty (and store.go its   JSON   ). 
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

// Save    graph.json(    store.go   writeJSON: tmp+rename orig write). 
func (g *Graph) Save(path string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return writeJSON(path, g)
}
