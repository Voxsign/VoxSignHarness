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

// CriteriaFromKnowhow 把 knowhow 映射成判据。
func CriteriaFromKnowhow(skillID, version string, kh Knowhow) []Criterion {
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
