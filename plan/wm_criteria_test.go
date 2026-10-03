//go:build vhswm

// wm_criteria_test.go —— WORKMEM-001 的可测 W 与 WM-1..WM-5。
//
// 现状（先红纪律）：
//
//	WM-2（丢弃留痕）/ WM-3（每条有 source）= **绿**（观测面已实现）
//	WM-1（扩记忆→链长变长）/ WM-5（清空记忆→规划质量下降）= **红**（四块板尚未实现）
//	WM-4（JEV 判断带 judged_by）= **红**（P3 未接）
//
// 运行：go test -tags vhswm ./plan
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

// WM-3：工作记忆里每条必须有 source（推断的标 inferred）。
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

// WM-2：W_drop 必须可观测——丢弃要留痕，且计数与留痕一致。
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

// WM-1：同一**多实体**目标，扩工作记忆（容量）后链长必须变长。
//
// 诚实边界：本判据只在"目标本身是多实体"时成立；单实体目标扩容量**不会**变长
// （见 docs 登记：WM-1 是假设，不是普适规律）。
func TestWM1LargerMemoryYieldsLongerChain(t *testing.T) {
	m := wmFixture(t)
	goal := "改这些模块|a|b|c|d|e|f|g|h|i|j|k|l"
	small, err := LocalPlanner{}.PlanWithMemory(goal, m, 4)
	if err != nil {
		t.Fatalf("[WM-1] 小容量规划失败: %v", err)
	}
	large, err := LocalPlanner{}.PlanWithMemory(goal, m, 12)
	if err != nil {
		t.Fatalf("[WM-1] 大容量规划失败: %v", err)
	}
	if len(large.Steps) <= len(small.Steps) {
		t.Errorf("[WM-1] 扩容后链长未变长: %d → %d（假设可能不成立，如实报告）", len(small.Steps), len(large.Steps))
	}
	if small.WM.Drop == 0 {
		t.Errorf("[WM-1] 小容量应发生丢弃且留痕: %+v", small.WM)
	}
}

// WM-5（关键）：清空工作记忆后**规划质量必须下降**，否则是死代码。
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

// WM-4：判断入记忆必须带 judged_by，**不得与事实同形**。
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
