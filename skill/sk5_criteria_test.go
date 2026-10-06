// sk5_criteria_test.go -- SK-5: inize  produceout**   ** data(prevent"  "). 
package skill

import "testing"

func archGuardianKnowhow() Knowhow {
	return Knowhow{
		Judging:  []string{"约束未越界", "决策有 ADR"},
		Cautions: []string{"不臆造假设", "关键歧义一次问清"},
		Basis:    []string{"已查证优先于一方称"},
		Style:    []string{"结论先行", "证据支撑"},
	}
}

func TestSK5ProducesAtLeastFiveCriteria(t *testing.T) {
	cs := CriteriaFromKnowhow("arch-guardian", "v0.1.0", archGuardianKnowhow())
	if len(cs) < 5 {
		t.Fatalf("[SK-5] 应产出 >=5 条判据，实际 %d", len(cs))
	}
	for _, c := range cs {
		if c.ID == "" || c.Skill != "arch-guardian" || c.Field == "" || c.Text == "" || c.Check == "" {
			t.Errorf("[SK-5] 判据字段不全: %+v", c)
		}
	}
}

func TestSK5BasisIsAutomatedAndCheckable(t *testing.T) {
	cs := CriteriaFromKnowhow("arch-guardian", "v0.1.0", archGuardianKnowhow())
	found := false
	for _, c := range cs {
		if c.Field == "basis" {
			found = true
			if c.Manual || c.Check != "checkVerifiedOverHearsay" {
				t.Errorf("[SK-5] basis 未自动化: %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("[SK-5] 未产出 basis 判据")
	}
	got, degraded := PreferVerified([]Evidence{{Kind: "hearsay"}, {Kind: "verified"}})
	if got.Kind != "verified" || degraded {
		t.Errorf("[SK-5] 未优先已查证: %+v deg=%v", got, degraded)
	}
	only, deg2 := PreferVerified([]Evidence{{Kind: "hearsay"}})
	if !deg2 || only.Kind != "hearsay" {
		t.Errorf("[SK-5] 只有一方称却未标 degraded: %+v deg=%v", only, deg2)
	}
}

func TestSK5ManualRatioIsReported(t *testing.T) {
	cs := CriteriaFromKnowhow("arch-guardian", "v0.1.0", archGuardianKnowhow())
	auto, total := AutomatedRatio(cs)
	if total != len(cs) {
		t.Fatalf("[SK-5] 比例口径错: %d/%d", auto, total)
	}
	if auto == 0 {
		t.Errorf("[SK-5] 一条自动判据都没有")
	}
	if auto == total {
		t.Errorf("[SK-5] 全自动不合预期")
	}
	for _, c := range cs {
		if c.Manual && c.Check != "manual" {
			t.Errorf("[SK-5] manual 标注与 check 不一致: %+v", c)
		}
	}
}
