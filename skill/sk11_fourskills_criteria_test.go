// sk11_fourskills_criteria_test.go —— 4 技能守恒（**56**）+ basis 族四处点亮。
package skill

import "testing"

// Lead 真值：每技能的 knowhow 条数（与我自测拉取**逐项一致**）
var expectedTotals = map[string]int{
	"ai-native-architecture-design": 30,
	"arch-guardian":                 11,
	"deep-research":                 9,
	"arch-review":                   6,
}

// ① 单技能守恒 + 合计 **56**
func TestSK11FourSkillsConservation(t *testing.T) {
	khs := realKnowhow()
	if len(khs) != 4 {
		t.Fatalf("[SK-11] 应有 4 个技能，实际 %d", len(khs))
	}
	grand := 0
	for id, kh := range khs {
		m := MapKnowhow(id, "self-fetched", kh)
		want := expectedTotals[id]
		if m.Total != want {
			t.Errorf("[对账] %s 条数 %d ≠ 真值 %d（**不许调数字去凑**）", id, m.Total, want)
		}
		sum := len(m.Criteria) + len(m.Templates) + len(m.Excluded)
		if sum != m.Total {
			t.Errorf("[SK-11] %s 不守恒: %d + %d + %d = %d ≠ %d", id,
				len(m.Criteria), len(m.Templates), len(m.Excluded), sum, m.Total)
		}
		grand += m.Total
	}
	if grand != 56 {
		t.Errorf("[SK-11] 4 技能合计应为 56，实际 %d", grand)
	}
}

// ② basis 族**四处点亮**：每个技能的 basis 都进了判据（不再只有 1/4）
func TestBasisFamilyAppliesToAllFourSkills(t *testing.T) {
	khs := realKnowhow()
	basisCounts := map[string]int{}
	for id, kh := range khs {
		m := MapKnowhow(id, "self-fetched", kh)
		n := 0
		for _, c := range m.Criteria {
			if c.Field == "basis" {
				n++
			}
		}
		basisCounts[id] = n
		if n != len(kh.Basis) {
			t.Errorf("[basis 族] %s 的 basis 未全部成判据: %d/%d", id, n, len(kh.Basis))
		}
	}
	want := map[string]int{"ai-native-architecture-design": 5, "arch-guardian": 1, "deep-research": 1, "arch-review": 1}
	for id, w := range want {
		if basisCounts[id] != w {
			t.Errorf("[basis 族] %s basis 判据 %d ≠ %d", id, basisCounts[id], w)
		}
	}
}

// ③ 覆盖率：全部 knowhow 条目都有归宿（判据 or 模板），守恒口径下 100%
func TestSK11CoverageIsTotal(t *testing.T) {
	covered, total := 0, 0
	for id, kh := range realKnowhow() {
		m := MapKnowhow(id, "self-fetched", kh)
		covered += len(m.Criteria) + len(m.Templates)
		total += m.Total
	}
	if covered != total {
		t.Errorf("[SK-11] 覆盖率非 100%%: %d/%d", covered, total)
	}
}
