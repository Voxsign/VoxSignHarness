// basis_map_criteria_test.go -- basis-> rule  ( **prevent  **revexample). 
package skill

import "testing"

// ①    8   basis:   (   +      ==  num), and   ize example
func TestBasisMapIsConservativeOnRealKnowhow(t *testing.T) {
	var ms []BasisMapping
	for id, kh := range realKnowhow() {
		ms = append(ms, MapBasis(id, "self-fetched", kh.Basis))
	}
	total := 0
	for _, m := range ms {
		if len(m.Mapped)+len(m.Manual) != m.Total {
			t.Errorf("[basis 映射] 不守恒: %d + %d ≠ %d", len(m.Mapped), len(m.Manual), m.Total)
		}
		total += m.Total
	}
	if total != 8 {
		t.Fatalf("[basis 映射] 真实 basis 应为 8 条，实际 %d", total)
	}
	ok, all := BasisAutomationRatio(ms)
	t.Logf("basis 自动化：%d/%d", ok, all)
	for _, m := range ms {
		for _, mb := range m.Manual {
			if mb.Reason == "" {
				t.Errorf("[basis 映射] 不能映射却无理由（静默丢）: %+v", mb)
			}
		}
		for _, mp := range m.Mapped {
			switch mp.Rule {
			case RuleTraceable, RuleExecuted, RuleVerified, RuleParadigm:
			default:
				t.Errorf("[basis 映射] 映射到了不存在的族规则: %+v", mp)
			}
		}
	}
}

// ② **prevent  revexample**:     at data first   base ⇒ **  reject  **
func TestBasisMapRejectsUnrelatedText(t *testing.T) {
	for _, bad := range []string{
		"结论先行",                  // style
		"图示优先 Mermaid 格式",       // deliverrule 
		"用脚手架 python scripts/x", //   use 
		"一页以内",                  //   
	} {
		if rule, ok, _ := RuleOfBasis(bad); ok {
			t.Errorf("[basis 映射 反例] 明显不相关的文本被硬凑进族规则 %q: %q", rule, bad)
		}
	}
}

// ③ posexample:   rule   be   base in
func TestBasisMapHitsAllFourRules(t *testing.T) {
	cases := map[string]string{
		"已查证优先于一方称":       RuleVerified,
		"可追溯来源优先":         RuleTraceable,
		"已执行 check 高于口头称": RuleExecuted,
		"范式优先原则：先问新范式内核":  RuleParadigm,
	}
	for text, want := range cases {
		got, ok, _ := RuleOfBasis(text)
		if !ok || got != want {
			t.Errorf("[basis 映射] %q → %q ok=%v，期望 %q", text, got, ok, want)
		}
	}
}
