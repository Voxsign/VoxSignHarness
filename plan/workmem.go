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

	DropTrace []string `json:"drop_trace,omitempty"` // WM-2：丢弃留痕
}

// WMCapacityDefault 是默认容量。**UNVALIDATED**：如何标定"为什么是 8"尚无实测依据。
const WMCapacityDefault = 8

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

// cap 返回有效容量（≤0 用默认）。
func (w *WorkingMemory) cap() int {
	if w.Capacity <= 0 {
		return WMCapacityDefault
	}
	return w.Capacity
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
