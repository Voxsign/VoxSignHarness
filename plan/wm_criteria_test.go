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

// WM-1：扩工作记忆后**链长必须变长**（否则缓存没用）。四块板未实现 → 先红。
func TestWM1LargerMemoryYieldsLongerChain(t *testing.T) {
	m := wmFixture(t)
	small, _ := LocalPlanner{}.Plan("把这个项目里所有 TODO 整理成一份文档", m)
	// 扩容：把容量显式提高后重规划（当前实现容量固定，无扩容入口）
	large, err := LocalPlanner{}.PlanWithMemory("把这个项目里所有 TODO 整理成一份文档", m, wmCapacity*2)
	if err != nil {
		t.Fatalf("[WM-1] 无扩容入口（四块板未实现，先红）: %v", err)
	}
	if len(large.Steps) <= len(small.Steps) {
		t.Errorf("[WM-1] 扩容后链长未变长: %d → %d", len(small.Steps), len(large.Steps))
	}
}

// WM-5（关键）：清空工作记忆后**规划质量必须下降**，否则是死代码。先红。
func TestWM5ClearMemoryDegradesPlanning(t *testing.T) {
	m := wmFixture(t)
	pl := &WorkingMemory{}
	pl.Remember(ConsideredItem{Element: "实体:报价模块", Source: "session", Inferred: false})
	full, err := LocalPlanner{}.PlanWithWorkingMemory("把那个模块改了", m, pl)
	if err != nil {
		t.Fatalf("[WM-5] 带工作记忆的规划入口未实现（四块板未做，先红）: %v", err)
	}
	pl.Clear()
	empty, err := LocalPlanner{}.PlanWithWorkingMemory("把那个模块改了", m, pl)
	if err != nil {
		t.Fatalf("[WM-5] 清空后重规划失败: %v", err)
	}
	if len(empty.Steps) >= len(full.Steps) {
		t.Errorf("[WM-5] 清空工作记忆后规划质量未下降（死代码嫌疑）: full=%d empty=%d", len(full.Steps), len(empty.Steps))
	}
	_ = strings.TrimSpace
}
