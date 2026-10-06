//go:build vhswm

// wm_criteria_test.go -- WORKMEM-001     W and WM-1..WM-5. 
//
// nowstatus(first   ): 
//
//	WM-2(    )/ WM-3(  has source)= ** **(  facealready now)
//	WM-1(   ->chain change )/ WM-5( empty  ->rule   under )= ** **(      now)
//	WM-4(JEV  disconnect  judged_by)= ** **(P3  connect)
//
//   : go test -tags vhswm ./plan
package plan

import (
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/space"
	"voicesign-harness/tools"
)

func wmFixture(t *testing.T) Manifest {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "none")
	tr, err := tools.LoadContracts(missing)
	if err != nil {
		t.Fatal(err)
	}
	sr, err := space.Load(missing)
	if err != nil {
		t.Fatal(err)
	}
	return ExportManifest(tr, sr)
}

// WM-3:          has source( disconnect tgt inferred). 
func TestWM3EveryConsideredHasSource(t *testing.T) {
	m := wmFixture(t)
	for _, goal := range []string{"把这个项目里所有 TODO 整理成一份文档", "帮我部署到生产服务器", "把这个仓库推到 GitHub 远端分支"} {
		p, _ := LocalPlanner{}.Plan(goal, m)
		if len(p.Considered) == 0 {
			t.Errorf("[WM-3] %q 的观测面为空（没记录考虑了哪些要素）", goal)
			continue
		}
		for _, it := range p.Considered {
			if it.Element == "" || it.Source == "" {
				t.Errorf("[WM-3] %q 有要素缺 source: %+v", goal, it)
			}
		}
	}
}

// WM-2: W_drop      --  need  , and numand    . 
func TestWM2DropIsTracedAndBounded(t *testing.T) {
	m := wmFixture(t)
	p, _ := LocalPlanner{}.Plan("帮我部署到生产服务器", m)
	wm := p.WM
	if wm.Max <= 0 {
		t.Fatalf("[WM-2] W_max 未设置: %+v", wm)
	}
	if wm.Used > wm.Max {
		t.Errorf("[WM-2] W_used(%d) > W_max(%d)", wm.Used, wm.Max)
	}
	if wm.Drop != len(wm.DropTrace) {
		t.Errorf("[WM-2] W_drop(%d) 与留痕条数(%d) 不一致 —— 静默丢弃", wm.Drop, len(wm.DropTrace))
	}
	if wm.ChainLen != len(p.Steps) {
		t.Errorf("[WM-2] 链长(%d) 与步数(%d) 不一致", wm.ChainLen, len(p.Steps))
	}
}

// WM-1(new state, VHS-WMCAP-001): **   ↑ ⇒   ↑ ⇒ chain ↑**. 
//
// if   become  ⇒ "  "  change tobase  nouse,      but iscall (e.g.   ). 
func TestWM1FamiliarityRaisesCapacityAndChain(t *testing.T) {
	m := wmFixture(t)
	const n = 30
	build := func(familiar bool) *WorkingMemory {
		w := &WorkingMemory{DemandFloor: 2} // objtgt form to 2   body
		for i := 0; i < n; i++ {
			src := "session"
			if familiar {
				src = "hotcache" //  inbaselycache =   
			}
			w.Remember("working_set", BoardItem{Element: "模块" + string(rune('A'+i%26)) + string(rune('a'+i/26)), Source: src})
		}
		return w
	}
	cold := build(false)
	hot := build(true)
	capCold := cold.CapacityFor()
	capHot := hot.CapacityFor()
	if capHot.Capacity <= capCold.Capacity {
		t.Fatalf("[WMC-2] 熟悉度↑ 容量未↑: %d → %d", capCold.Capacity, capHot.Capacity)
	}
	pCold, err := LocalPlanner{}.PlanWithWorkingMemory("改这两个模块", m, cold)
	if err != nil {
		t.Fatal(err)
	}
	pHot, err := LocalPlanner{}.PlanWithWorkingMemory("改这两个模块", m, hot)
	if err != nil {
		t.Fatal(err)
	}
	if len(pHot.Steps) <= len(pCold.Steps) {
		t.Errorf("[WM-1新] 熟悉度↑ 容量↑（%d→%d）但链长未变长（%d→%d）⇒ 容量变量可能无用",
			capCold.Capacity, capHot.Capacity, len(pCold.Steps), len(pHot.Steps))
	}
}

// WMC-1:     0 ->      [3,10]. 
func TestWMC1ColdCapacityInLowBand(t *testing.T) {
	tr := DefaultWMCap.Capacity(1, 10, 0)
	if tr.Capacity < 3 || tr.Capacity > 10 {
		t.Errorf("[WMC-1] 熟悉度 0 容量=%d，应在 [3,10]", tr.Capacity)
	}
}

// WMC-2:    ↑ ⇒   ↑, and   max. 
func TestWMC2CapacityIncreasesWithFamiliarity(t *testing.T) {
	prev := 0
	for _, fam := range []int{0, 3, 6, 9, 10} {
		tr := DefaultWMCap.Capacity(5, 10, fam)
		if tr.Capacity < prev {
			t.Errorf("[WMC-2] 熟悉度上升但容量下降: %d → %d", prev, tr.Capacity)
		}
		if tr.Capacity > DefaultWMCap.Max {
			t.Errorf("[WMC-2] 容量超 max: %d", tr.Capacity)
		}
		prev = tr.Capacity
	}
}

// WMC-3(close ): **    ↓ ⇒      ing ↓**(only    =  izebecome   max). 
func TestWMC3CapacityDecreasesWhenFamiliarityDrops(t *testing.T) {
	hot := DefaultWMCap.Capacity(8, 10, 9)
	cold := DefaultWMCap.Capacity(8, 10, 1)
	if cold.Capacity >= hot.Capacity {
		t.Fatalf("[WMC-3] 熟悉度下降容量未降: hot=%d cold=%d ⇒ 动态退化", hot.Capacity, cold.Capacity)
	}
}

// WMC-4:  toptime capped=true and DropTrace has  . 
func TestWMC4CappedIsVisibleWithDropTrace(t *testing.T) {
	tr := DefaultWMCap.Capacity(20, 20, 20) // needrequire 20 × (1+1) = 40 > max 20
	if !tr.Capped {
		t.Errorf("[WMC-4] 封顶未标记: %+v", tr)
	}
	if tr.Capacity != DefaultWMCap.Max {
		t.Errorf("[WMC-4] 封顶后应等于 max: %+v", tr)
	}
	w := &WorkingMemory{DemandFloor: 30}
	for i := 0; i < 30; i++ {
		w.Remember("working_set", BoardItem{Element: "e" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Source: "hotcache"})
	}
	ct := w.CapacityFor()
	if ct.DropCount == 0 || len(ct.DropTrace) != ct.DropCount {
		t.Errorf("[WMC-4] 溢出未留痕: %+v", ct)
	}
}

// WMC-5:   changeizesafety   (capacity / needrequireunderlimit /     / is  top). 
func TestWMC5CapacityTraceIsComplete(t *testing.T) {
	w := &WorkingMemory{DemandFloor: 4}
	for i := 0; i < 10; i++ {
		src, el := "session", "x"+string(rune('a'+i))
		if i%2 == 0 {
			src = "manifest" //     
		}
		w.Remember("working_set", BoardItem{Element: el, Source: src})
	}
	m := wmFixture(t)
	p, err := LocalPlanner{}.PlanWithWorkingMemory("改这个", m, w)
	if err != nil {
		t.Fatal(err)
	}
	if p.WM.Capacity == 0 || p.WM.DemandFloor == 0 {
		t.Errorf("[WMC-5] 容量/需求下限未留痕: %+v", p.WM)
	}
	if p.WM.Familiarity <= 0 || p.WM.Familiarity > 1 {
		t.Errorf("[WMC-5] 熟悉度未留痕或越界: %v", p.WM.Familiarity)
	}
}

// WM-5(close ):  empty    after**rule     under **,  thenis  code. 
func TestWM5ClearMemoryDegradesPlanning(t *testing.T) {
	m := wmFixture(t)
	w := &WorkingMemory{}
	w.Remember("working_set", BoardItem{Element: "模块:报价模块", Source: "session"})
	full, err := LocalPlanner{}.PlanWithWorkingMemory("把那个模块改了", m, w)
	if err != nil {
		t.Fatalf("[WM-5] 带记忆规划失败: %v", err)
	}
	w.Clear()
	empty, err := LocalPlanner{}.PlanWithWorkingMemory("把那个模块改了", m, w)
	if err != nil {
		t.Fatalf("[WM-5] 清空后规划失败: %v", err)
	}
	if len(full.Steps) == 0 {
		t.Errorf("[WM-5] 有记忆时应能排链: %+v", full)
	}
	if len(empty.Steps) >= len(full.Steps) {
		t.Errorf("[WM-5] 清空工作记忆后规划质量未下降（死代码嫌疑）: full=%d empty=%d", len(full.Steps), len(empty.Steps))
	}
	if !empty.Refused {
		t.Errorf("[WM-5] 清空后应拒绝并回问: %+v", empty)
	}
}

// WM-4:  disconnectin      judged_by, **  and  same **. 
func TestWM4JudgementIsMarkedNotFact(t *testing.T) {
	w := &WorkingMemory{}
	w.Remember("working_set", BoardItem{Element: "实体:aiops", Source: "services", Inferred: false})
	w.Remember("working_set", BoardItem{Element: "判断:应走网关", Source: "jev", Inferred: false, JudgedBy: "jev"})
	text := w.Render()
	if !strings.Contains(text, "judged_by=jev") {
		t.Errorf("[WM-4] 判断未标 judged_by，与事实同形: %q", text)
	}
	if !strings.Contains(text, "实体:aiops") || strings.Contains(strings.Split(text, "判断:应走网关")[0], "judged_by") {
		t.Errorf("[WM-4] 事实条目不应带 judged_by: %q", text)
	}
}
