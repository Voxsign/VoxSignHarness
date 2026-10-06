// workmem.go --        (WORKMEM-001 §2). 
//
//     raise  = "   faceon   "; P3   L0.5 needpipe    become JEV   situation. 
//
//	① casescenario  Situation   " pos    "
//	②    body  WorkingSet"  onhas  "
//	③  end  Constraints "      "(** decide **)
//	④  decide  OpenItems    " also     "(**  clarification**)
package plan

import (
	"sort"
	"strings"
)

// BoardItem is on   (WM-3:     has source). 
type BoardItem struct {
	Element  string `json:"element"`
	Source   string `json:"source"`
	Inferred bool   `json:"inferred"`
	// JudgedBy  emptytableshow  is" disconnect"but "  "(WM-4:   and  same ). 
	JudgedBy string `json:"judged_by,omitempty"`
}

// WorkingMemory is    +   . 
//
// Capacity default WMCapacityDefault; ** value UNVALIDATED**( tgt , onlyas  onboundary). 
type WorkingMemory struct {
	Situation   []BoardItem `json:"situation"`
	WorkingSet  []BoardItem `json:"working_set"`
	Constraints []BoardItem `json:"constraints"`
	OpenItems   []BoardItem `json:"open_items"`
	Capacity    int         `json:"capacity"`
	// DemandFloor is"objtgt  form to  bodynum"(0 ⇒ by   bodynum disconnect). 
	//  is WMCAP  form needrequireside;    bodynumisprovidegiveside -- ** er same   formempty **(    ). 
	DemandFloor int `json:"demand_floor"`

	DropTrace []string `json:"drop_trace,omitempty"` // WM-2:     
}

// WMCapacityDefault is** bot**default  (no WMCAP   timeuse). 
//
// ⚠️ UNVALIDATED:     8 alreadybe VHS-WMCAP-001   time formget (see WMCapConfig). 
const WMCapacityDefault = 8

// WMCapConfig is       time(VHS-WMCAP-001). 
//
//	curbefore   = clamp(needrequireunderlimit × (1 +     × Ratio), Min, Max)
//	  needrequireunderlimit = objtgtin form to  bodynum(  all   ⇒ 1)
//	        =  inbaselycache/ obj type  bodynum ÷   bodynum ∈ [0,1]
//
// ⚠️ **  num(3 / 20 / 1.0)is Lead  getvalue,    ** ⇒ **safety  UNVALIDATED**. 
// ⚠️ **"   "    base    **( incache !=   )-- is   form lybase, 
//
//	if    become ,  form be  but iscall . 
type WMCapConfig struct {
	Min   int     `json:"min"`
	Max   int     `json:"max"`
	Ratio float64 `json:"ratio"`
}

// DefaultWMCap is VHS-WMCAP-001  initstartgetvalue(**UNVALIDATED**). 
var DefaultWMCap = WMCapConfig{Min: 3, Max: 20, Ratio: 1.0}

// CapacityTrace is         (C1/C2/WMC-5:   changeize   resolve ). 
type CapacityTrace struct {
	Capacity    int      `json:"capacity"`
	DemandFloor int      `json:"demand_floor"`
	Familiarity float64  `json:"familiarity"`
	Capped      bool     `json:"capped"`
	DropCount   int      `json:"drop_count"`
	DropTrace   []string `json:"drop_trace,omitempty"`
}

// Capacity by form   and  . 
func (c WMCapConfig) Capacity(demand, totalEntities, familiarEntities int) CapacityTrace {
	if c.Min <= 0 {
		c = DefaultWMCap
	}
	if demand < 1 {
		demand = 1 //    bodyall   ⇒ needrequireunderlimit 1
	}
	fam := 0.0
	if totalEntities > 0 {
		fam = float64(familiarEntities) / float64(totalEntities)
	}
	if fam < 0 {
		fam = 0
	}
	if fam > 1 {
		fam = 1
	}
	raw := float64(demand) * (1 + fam*c.Ratio)
	capv := int(raw)
	capped := false
	if capv > c.Max {
		capv = c.Max
		capped = true
	}
	if capv < c.Min {
		capv = c.Min
	}
	return CapacityTrace{Capacity: capv, DemandFloor: demand, Familiarity: fam, Capped: capped}
}

// IsFamiliar  disconnect   bodyis " inbaselycache/ obj type"(   split ). 
func IsFamiliar(it BoardItem) bool {
	src := it.Source
	for _, k := range []string{"cache", "hotcache", "manifest", "services", "project-model", "wmc"} {
		if strings.Contains(src, k) {
			return true
		}
	}
	return false
}

// CapacityFor bycurbefore   body   (and  ). 
func (w *WorkingMemory) CapacityFor() CapacityTrace {
	ws := w.WorkingSet
	fam := 0
	for _, it := range ws {
		if IsFamiliar(it) {
			fam++
		}
	}
	demand := w.DemandFloor
	if demand <= 0 {
		demand = len(ws)
	}
	tr := DefaultWMCap.Capacity(demand, len(ws), fam)
	//     split  (C2:  top = has  be  ,  allow  )
	if len(ws) > tr.Capacity {
		tr.DropCount = len(ws) - tr.Capacity
		for _, it := range ws[tr.Capacity:] {
			tr.DropTrace = append(tr.DropTrace, it.Element)
		}
	}
	return tr
}

// Remember by     ; no source/Element then  (WM-3    ). 
func (w *WorkingMemory) Remember(board string, it BoardItem) {
	if it.Element == "" || it.Source == "" {
		return
	}
	switch board {
	case "situation":
		w.Situation = append(w.Situation, it)
	case "working_set":
		w.WorkingSet = append(w.WorkingSet, it)
	case "constraints":
		w.Constraints = append(w.Constraints, it)
	case "open_items":
		w.OpenItems = append(w.OpenItems, it)
	}
}

// Clear  empty   (WM-5:  emptyafterrule     under ). 
func (w *WorkingMemory) Clear() {
	w.Situation, w.WorkingSet, w.Constraints, w.OpenItems = nil, nil, nil, nil
	w.DropTrace = nil
}

// Items returnback hasneed (  use). 
func (w *WorkingMemory) Items() []BoardItem {
	var out []BoardItem
	out = append(out, w.Situation...)
	out = append(out, w.WorkingSet...)
	out = append(out, w.Constraints...)
	out = append(out, w.OpenItems...)
	return out
}

// cap returnbackhas   :  form Capacity  first;  thenby WMCAP  form (no bodytime izeas Min). 
func (w *WorkingMemory) cap() int {
	if w.Capacity > 0 {
		return w.Capacity
	}
	if len(w.WorkingSet) > 0 {
		return w.CapacityFor().Capacity
	}
	return WMCapacityDefault
}

// BoundedWorkingSet returnback  in    body;  out   DropTrace(WM-2:   ). 
func (w *WorkingMemory) BoundedWorkingSet() []BoardItem {
	c := w.cap()
	if len(w.WorkingSet) <= c {
		return append([]BoardItem(nil), w.WorkingSet...)
	}
	w.DropTrace = nil
	for _, it := range w.WorkingSet[c:] {
		w.DropTrace = append(w.DropTrace, it.Element)
	}
	return append([]BoardItem(nil), w.WorkingSet[:c]...)
}

// Render pipe     become situation  base(P3:  give JEV). 
//  disconnectclass obj  judged_by, and  splitopen(WM-4). 
func (w *WorkingMemory) Render() string {
	var b strings.Builder
	write := func(title string, items []BoardItem) {
		if len(items) == 0 {
			return
		}
		b.WriteString(title + ":\n")
		for _, it := range items {
			b.WriteString("- " + it.Element)
			if it.JudgedBy != "" {
				b.WriteString("（判断，judged_by=" + it.JudgedBy + "）")
			} else if it.Inferred {
				b.WriteString("（推断）")
			}
			b.WriteString(" [source=" + it.Source + "]\n")
		}
	}
	write("情景", w.Situation)
	write("活跃实体", w.BoundedWorkingSet())
	write("约束", w.Constraints)
	write("待决", w.OpenItems)
	return strings.TrimSpace(b.String())
}

// Resolve use   body  **coreference resolution**(P4       ② :     ). 
//   data: **uniqueonly resolve**;    /no    clarification( give patch). 
func (w *WorkingMemory) Resolve(mention string) (canonical string, ok bool) {
	ws := w.BoundedWorkingSet()
	var hits []string
	for _, it := range ws {
		if it.Element == mention || strings.Contains(it.Element, mention) {
			hits = append(hits, it.Element)
		}
	}
	sort.Strings(hits)
	if len(hits) == 1 {
		return hits[0], true
	}
	return "", false
}
