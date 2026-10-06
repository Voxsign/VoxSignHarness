//go:build vhsplan

// pl_criteria_test.go -- VHS-PLAN-001 §4: PL-1..PL-5(first ). 
//
// kindbase connectget PLAN-001 §7      (   ). 
//  data dependency type: onlyto Plan  close      ,  by   type type  timefirst first . 
package plan

import (
	"strings"
	"testing"
)

// empty   word: outnowi.e.     no   (PL-3). 
var vagueActions = []string{"分析", "优化一下", "处理一下", "看看", "研究一下"}

func plannerFixture(t *testing.T) (LocalPlanner, Manifest) {
	t.Helper()
	tr, sr := loadRegistries(t)
	return LocalPlanner{}, ExportManifest(tr, sr)
}

// PL-1 onlyuse    :    all    tolistin   . 
func TestPL1OnlyRealCapabilities(t *testing.T) {
	p, m := plannerFixture(t)
	got, err := p.Plan("把这个项目里所有 TODO 整理成一份文档", m)
	if err != nil {
		t.Fatalf("[PL-1] 规划器未产出计划（P1 桩，先红）: %v", err)
	}
	if len(got.Steps) == 0 {
		t.Fatal("[PL-1] 空计划")
	}
	real := m.toolSet()
	for i, s := range got.Steps {
		if s.Tool == "" {
			t.Errorf("[PL-1] 第 %d 步没有工具", i)
			continue
		}
		if _, ok := real[s.Tool]; !ok {
			t.Errorf("[PL-1] 第 %d 步用了清单外的工具 %q（幻觉）", i, s.Tool)
		}
	}
}

// PL-2   tothen   to(rev  ,  heavyneed   ). 
func TestPL2RefusesUnreachable(t *testing.T) {
	p, m := plannerFixture(t)
	got, err := p.Plan("帮我部署到生产服务器", m)
	if err != nil {
		t.Fatalf("[PL-2] 规划器未产出计划（P1 桩，先红）: %v", err)
	}
	if !got.Refused {
		t.Error("[PL-2] 对不可达目标（无 deploy 能力）未拒绝")
	}
	if len(got.Missing) == 0 {
		t.Error("[PL-2] 拒绝了却没说明缺什么")
	}
	if len(got.Steps) != 0 {
		t.Errorf("[PL-2] 拒绝却仍给出步骤（编了一个执行不了的计划）: %+v", got.Steps)
	}
}

// PL-3      : has   numandproduceout,   outnow"split  under" classempty . 
func TestPL3StepsExecutable(t *testing.T) {
	p, m := plannerFixture(t)
	got, err := p.Plan("先改这个文件，再提交", m)
	if err != nil {
		t.Fatalf("[PL-3] 规划器未产出计划（P1 桩，先红）: %v", err)
	}
	for i, s := range got.Steps {
		if len(s.Params) == 0 {
			t.Errorf("[PL-3] 第 %d 步没有参数（不可执行）", i)
		}
		if s.Output == "" {
			t.Errorf("[PL-3] 第 %d 步没有产出定义", i)
		}
		for _, v := range vagueActions {
			if strings.Contains(s.Action, v) {
				t.Errorf("[PL-3] 第 %d 步动作空泛 %q（无法执行）", i, s.Action)
			}
		}
	}
}

// PL-4   pos : firstmodifyafter  . 
func TestPL4Ordering(t *testing.T) {
	p, m := plannerFixture(t)
	got, err := p.Plan("先改这个文件，再提交", m)
	if err != nil {
		t.Fatalf("[PL-4] 规划器未产出计划（P1 桩，先红）: %v", err)
	}
	editIdx, commitIdx := -1, -1
	for i, s := range got.Steps {
		if s.Tool == "file" && hasCap(s.Caps, "write") && editIdx < 0 {
			editIdx = i
		}
		if s.Tool == "git" && hasCap(s.Caps, "commit") && commitIdx < 0 {
			commitIdx = i
		}
	}
	if editIdx < 0 || commitIdx < 0 {
		t.Fatalf("[PL-4] 计划缺少 修改(file.write) 或 提交(git.commit) 步骤: %+v", got.Steps)
	}
	if editIdx > commitIdx {
		t.Errorf("[PL-4] 顺序错误：先提交(#%d)后修改(#%d)", commitIdx, editIdx)
	}
}

// PL-5   at need:       need become  . 
func TestPL5NotLongerThanNecessary(t *testing.T) {
	p, m := plannerFixture(t)
	got, err := p.Plan("找到这个项目里的 TODO 文件", m)
	if err != nil {
		t.Fatalf("[PL-5] 规划器未产出计划（P1 桩，先红）: %v", err)
	}
	if len(got.Steps) == 0 {
		t.Fatal("[PL-5] 空计划")
	}
	if len(got.Steps) > 3 {
		t.Errorf("[PL-5] 单动作目标用了 %d 步（凑步骤）", len(got.Steps))
	}
}

func hasCap(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}
