// sk11_criteria_test.go —— knowhow 映射必须**守恒**（OOB：7 + 0 != 11 就是静默丢弃）。
package skill

import "testing"

// 真服务实测的 arch-guardian knowhow（Lead 已复核，含 steps 4 条 ⇒ 合计 11）
func archGuardianFull() Knowhow {
	kh := archGuardianKnowhow()
	kh.Steps = []string{"澄清目标与边界", "提取质量属性并排序", "给出候选方案", "权衡后记录决策"}
	return kh
}

// ① 守恒：判据 + 模板 + 显式排除 == knowhow 总条数（当前应为 7 + 4 + 0 == 11）
func TestSK11MappingIsConservative(t *testing.T) {
	kh := archGuardianFull()
	m := MapKnowhow("arch-guardian", "v0.1.0", kh)
	if m.Total != 11 {
		t.Fatalf("[SK-11] knowhow 总数应为 11，实际 %d", m.Total)
	}
	sum := len(m.Criteria) + len(m.Templates) + len(m.Excluded)
	if sum != m.Total {
		t.Fatalf("[SK-11] 不守恒：判据 %d + 模板 %d + 排除 %d = %d，总 %d ⇒ 有静默丢弃",
			len(m.Criteria), len(m.Templates), len(m.Excluded), sum, m.Total)
	}
}

// ② steps 的归宿是**规划模板**（不是"被判据排除"）
func TestSK11StepsBecomeTemplates(t *testing.T) {
	m := MapKnowhow("arch-guardian", "v0.1.0", archGuardianFull())
	if len(m.Templates) != 4 {
		t.Fatalf("[SK-11] steps 应成 4 条规划模板，实际 %d", len(m.Templates))
	}
	for _, tm := range m.Templates {
		if tm.Skill != "arch-guardian" || tm.Text == "" || tm.Index == 0 {
			t.Errorf("[SK-11] 模板字段不全: %+v", tm)
		}
	}
	// 且它们**不得**同时出现在判据里（避免重复计入而"看起来守恒"）
	for _, c := range m.Criteria {
		for _, tm := range m.Templates {
			if c.Text == tm.Text {
				t.Errorf("[SK-11] 同一条既成判据又成模板（双重计入）: %q", c.Text)
			}
		}
	}
}

// ③ 排除项必须**带上理由**（有归宿的排除才允许；当前应为空）
func TestSK11ExcludedMustCarryReason(t *testing.T) {
	m := MapKnowhow("arch-guardian", "v0.1.0", archGuardianFull())
	for _, e := range m.Excluded {
		if e.Reason == "" {
			t.Errorf("[SK-11] 排除项无理由（静默丢弃）: %+v", e)
		}
	}
}
