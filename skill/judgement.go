// judgement.go —— SK-5：`knowhow.*` → **可执行判据**（只存文本不算内化）。
//
// ⚠️ 诚实要求：**能自动化的实现 check；暂不能的标 `manual` 并显式列出**（不许假装自动）。
package skill

import "strings"

// Knowhow 是服务侧技能 manifest 里的 knowhow 结构。
type Knowhow struct {
	Steps    []string `json:"steps"`
	Cautions []string `json:"cautions"`
	Style    []string `json:"style"`
	Judging  []string `json:"judging"`
	Basis    []string `json:"basis"`
}

// Criterion 是一条由 knowhow 产出的判据。
type Criterion struct {
	ID      string `json:"id"`
	Skill   string `json:"source_skill"`
	Version string `json:"source_version"`
	Field   string `json:"field"` // judging | cautions | basis | style
	Text    string `json:"text"`
	Check   string `json:"check"`  // 可执行检查名；manual 表示人工
	Manual  bool   `json:"manual"` // **显式标注**
}

// Template 是"规划模板"条目（steps 的正确归宿，VHS-SKILL-001 §3：steps → 规划模板）。
type Template struct {
	Skill   string `json:"source_skill"`
	Version string `json:"source_version"`
	Index   int    `json:"index"`
	Text    string `json:"text"`
}

// Excluded 显式记录**没有归宿的 knowhow 条目**（SK-11：不许静默丢弃）。
type Excluded struct {
	Key    string `json:"key"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// Mapping 是 knowhow 的完整映射结果（**守恒**：三类之和 == knowhow 总条数）。
type Mapping struct {
	Criteria  []Criterion `json:"criteria"`
	Templates []Template  `json:"templates"` // steps → 规划模板
	Excluded  []Excluded  `json:"excluded"`  // 显式排除（带理由）
	Total     int         `json:"total"`     // knowhow 条目总数
}

// KnowhowTotal 数出 knowhow 的全部条目（用于守恒断言）。
func KnowhowTotal(kh Knowhow) int {
	return len(kh.Steps) + len(kh.Judging) + len(kh.Cautions) + len(kh.Basis) + len(kh.Style)
}

// MapKnowhow 是**守恒**映射：每条 knowhow 要么成判据、要么成模板、要么显式排除。
func MapKnowhow(skillID, version string, kh Knowhow) Mapping {
	m := Mapping{Total: KnowhowTotal(kh)}
	// steps → 规划模板（不是丢弃）
	for i, t := range kh.Steps {
		m.Templates = append(m.Templates, Template{Skill: skillID, Version: version, Index: i + 1, Text: t})
	}
	m.Criteria = criteriaOnly(skillID, version, kh)
	// 守恒检查：任何未进判据也未进模板的条目，必须显式排除（当前映射已覆盖全部键）
	sum := len(m.Criteria) + len(m.Templates) + len(m.Excluded)
	if sum != m.Total {
		m.Excluded = append(m.Excluded, Excluded{
			Key: "unmapped", Text: "", Reason: "映射缺口（守恒失败）—— 属实现缺陷，必须显式暴露",
		})
	}
	return m
}

// CriteriaFromKnowhow 把 knowhow 映射成判据（兼容入口）。
func CriteriaFromKnowhow(skillID, version string, kh Knowhow) []Criterion {
	return criteriaOnly(skillID, version, kh)
}

func criteriaOnly(skillID, version string, kh Knowhow) []Criterion {
	var out []Criterion
	add := func(field string, items []string) {
		for i, t := range items {
			c := Criterion{
				ID: skillID + "." + field + "." + itoa(i+1), Skill: skillID, Version: version,
				Field: field, Text: t, Check: "manual", Manual: true,
			}
			// **优先自动化**：basis 的"已查证优先于一方称"今天已真实生效过一次，可验。
			if field == "basis" && strings.Contains(t, "已查证") {
				c.Check = "checkVerifiedOverHearsay"
				c.Manual = false
			}
			out = append(out, c)
		}
	}
	add("judging", kh.Judging)
	add("cautions", kh.Cautions)
	add("basis", kh.Basis)
	add("style", kh.Style)
	return out
}

// AutomatedRatio 返回自动化比例 (automated, total) —— **诚实指标**。
func AutomatedRatio(cs []Criterion) (int, int) {
	n := 0
	for _, c := range cs {
		if !c.Manual {
			n++
		}
	}
	return n, len(cs)
}

// Evidence 是一条证据（basis 规则用）。
type Evidence struct {
	Kind   string // verified（已查证）| hearsay（一方称）
	Detail string
}

// PreferVerified 实现 basis「**已查证优先于一方称**」；
// 若**只有一方称**，如实标 degraded（不许当成已核实）。
func PreferVerified(ev []Evidence) (Evidence, bool) {
	for _, e := range ev {
		if e.Kind == "verified" {
			return e, false
		}
	}
	if len(ev) > 0 {
		return ev[0], true
	}
	return Evidence{}, true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
