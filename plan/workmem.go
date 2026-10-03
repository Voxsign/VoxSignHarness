// workmem.go —— 工作记忆四块板（WORKMEM-001 §2）。
//
// 四块板合起来 = "摊在桌面上的战场"；P3 的 L0.5 要把它们渲染成 JEV 的 situation。
//
//	① 情景板 Situation   「我正在做什么」
//	② 活跃实体板 WorkingSet「我手上有什么」
//	③ 约束板 Constraints 「我不能做什么」（**否决权**）
//	④ 待决板 OpenItems    「我还不知道什么」（**必须回问**）
package plan

import (
	"sort"
	"strings"
)

// BoardItem 是板上的一条（WM-3：每条必须有 source）。
type BoardItem struct {
	Element  string `json:"element"`
	Source   string `json:"source"`
	Inferred bool   `json:"inferred"`
	// JudgedBy 非空表示这条是"判断"而非"事实"（WM-4：不得与事实同形）。
	JudgedBy string `json:"judged_by,omitempty"`
}

// WorkingMemory 是四块板 + 容量。
//
// Capacity 默认 WMCapacityDefault；**该值 UNVALIDATED**（未标定，仅为可测上界）。
type WorkingMemory struct {
	Situation   []BoardItem `json:"situation"`
	WorkingSet  []BoardItem `json:"working_set"`
	Constraints []BoardItem `json:"constraints"`
	OpenItems   []BoardItem `json:"open_items"`
	Capacity    int         `json:"capacity"`
	// DemandFloor 是"目标里显式提到的实体数"（0 ⇒ 由活跃实体数推断）。
	// 它是 WMCAP 公式的需求侧；活跃实体数是供给侧 —— **两者混同会让公式空转**（实测教训）。
	DemandFloor int `json:"demand_floor"`

	DropTrace []string `json:"drop_trace,omitempty"` // WM-2：丢弃留痕
}

// WMCapacityDefault 是**兜底**默认容量（无 WMCAP 信息时用）。
//
// ⚠️ UNVALIDATED：旧常量 8 已被 VHS-WMCAP-001 的区间公式取代（见 WMCapConfig）。
const WMCapacityDefault = 8

// WMCapConfig 是工作记忆容量区间（VHS-WMCAP-001）。
//
//	当前容量 = clamp(需求下限 × (1 + 熟悉度 × Ratio), Min, Max)
//	  需求下限 = 目标中显式提到的实体数（一个都不提 ⇒ 1）
//	  熟悉度   = 命中本地缓存/项目模型的实体数 ÷ 总实体数 ∈ [0,1]
//
// ⚠️ **三个数（3 / 20 / 1.0）是 Lead 的取值，非实测** ⇒ **全部 UNVALIDATED**。
// ⚠️ **「熟悉度」这个度量本身未验证**（命中缓存 ≠ 真懂）——它是整个公式的地基，
//
//	若该假设不成立，公式应被废掉而不是调参。
type WMCapConfig struct {
	Min   int     `json:"min"`
	Max   int     `json:"max"`
	Ratio float64 `json:"ratio"`
}

// DefaultWMCap 是 VHS-WMCAP-001 的初始取值（**UNVALIDATED**）。
var DefaultWMCap = WMCapConfig{Min: 3, Max: 20, Ratio: 1.0}

// CapacityTrace 是一次容量计算的留痕（C1/C2/WMC-5：容量变化必须可解释）。
type CapacityTrace struct {
	Capacity    int      `json:"capacity"`
	DemandFloor int      `json:"demand_floor"`
	Familiarity float64  `json:"familiarity"`
	Capped      bool     `json:"capped"`
	DropCount   int      `json:"drop_count"`
	DropTrace   []string `json:"drop_trace,omitempty"`
}

// Capacity 按公式算容量并留痕。
func (c WMCapConfig) Capacity(demand, totalEntities, familiarEntities int) CapacityTrace {
	if c.Min <= 0 {
		c = DefaultWMCap
	}
	if demand < 1 {
		demand = 1 // 一个实体都不提 ⇒ 需求下限 1
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

// IsFamiliar 判断一条实体是否"命中本地缓存/项目模型"（熟悉度分子）。
func IsFamiliar(it BoardItem) bool {
	src := it.Source
	for _, k := range []string{"cache", "hotcache", "manifest", "services", "project-model", "wmc"} {
		if strings.Contains(src, k) {
			return true
		}
	}
	return false
}

// CapacityFor 按当前活跃实体算容量（并留痕）。
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
	// 超容量部分留痕（C2：封顶 = 有东西被丢掉，不许静默）
	if len(ws) > tr.Capacity {
		tr.DropCount = len(ws) - tr.Capacity
		for _, it := range ws[tr.Capacity:] {
			tr.DropTrace = append(tr.DropTrace, it.Element)
		}
	}
	return tr
}

// Remember 按板登记一条；无 source/Element 则忽略（WM-3 不可破）。
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

// Clear 清空四块板（WM-5：清空后规划质量必须下降）。
func (w *WorkingMemory) Clear() {
	w.Situation, w.WorkingSet, w.Constraints, w.OpenItems = nil, nil, nil, nil
	w.DropTrace = nil
}

// Items 返回所有要素（观测用）。
func (w *WorkingMemory) Items() []BoardItem {
	var out []BoardItem
	out = append(out, w.Situation...)
	out = append(out, w.WorkingSet...)
	out = append(out, w.Constraints...)
	out = append(out, w.OpenItems...)
	return out
}

// cap 返回有效容量：显式 Capacity 优先；否则按 WMCAP 公式算（无实体时退化为 Min）。
func (w *WorkingMemory) cap() int {
	if w.Capacity > 0 {
		return w.Capacity
	}
	if len(w.WorkingSet) > 0 {
		return w.CapacityFor().Capacity
	}
	return WMCapacityDefault
}

// BoundedWorkingSet 返回容量内的活跃实体；超出的进 DropTrace（WM-2：留痕）。
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

// Render 把四块板渲染成 situation 文本（P3：送给 JEV）。
// 判断类条目带 judged_by，与事实分开（WM-4）。
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

// Resolve 用活跃实体板做**指代消解**（P4 的三层里的第②层：工作记忆）。
// 硬判据：**唯一才消解**；多候选/无候选一律回问（不给脑补）。
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
