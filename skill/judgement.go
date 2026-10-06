// judgement.go -- SK-5: `knowhow.*` -> **    data**(onlystore base  inize). 
//
// ⚠️   needrequire: **   ize  now check;     tgt `manual` and formlistout**( allow    ). 
package skill

import "strings"

// Knowhow isserveserviceside   manifest    knowhow close . 
type Knowhow struct {
	Steps    []string `json:"steps"`
	Cautions []string `json:"cautions"`
	Style    []string `json:"style"`
	Judging  []string `json:"judging"`
	Basis    []string `json:"basis"`
}

// Criterion is  by knowhow produceout  data. 
type Criterion struct {
	ID      string `json:"id"`
	Skill   string `json:"source_skill"`
	Version string `json:"source_version"`
	Field   string `json:"field"` // judging | cautions | basis | style
	Text    string `json:"text"`
	Check   string `json:"check"`  //      name; manual tableshowhuman
	Manual  bool   `json:"manual"` // ** formtgtnote**
}

// Template is"rule   " obj(steps  pos   , VHS-SKILL-001 §3: steps -> rule   ). 
type Template struct {
	Skill   string `json:"source_skill"`
	Version string `json:"source_version"`
	Index   int    `json:"index"`
	Text    string `json:"text"`
}

// Excluded  form  ** has    knowhow  obj**(SK-11:  allow    ). 
type Excluded struct {
	Key    string `json:"key"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// Mapping is knowhow  finish   close (**  **:  classofand == knowhow   num). 
type Mapping struct {
	Criteria  []Criterion `json:"criteria"`
	Templates []Template  `json:"templates"` // steps -> rule   
	Excluded  []Excluded  `json:"excluded"`  //  form  (  by)
	Total     int         `json:"total"`     // knowhow  obj num
}

// KnowhowTotal numout knowhow  safety  obj(useat  disconnectlang). 
func KnowhowTotal(kh Knowhow) int {
	return len(kh.Steps) + len(kh.Judging) + len(kh.Cautions) + len(kh.Basis) + len(kh.Style)
}

// MapKnowhow is**  **  :    knowhow need become data, need become  , need  form  . 
func MapKnowhow(skillID, version string, kh Knowhow) Mapping {
	m := Mapping{Total: KnowhowTotal(kh)}
	// steps -> rule   ( is  )
	for i, t := range kh.Steps {
		m.Templates = append(m.Templates, Template{Skill: skillID, Version: version, Index: i + 1, Text: t})
	}
	m.Criteria = criteriaOnly(skillID, version, kh)
	//     :      dataalso      obj,    form  (curbefore  alreadyoverwritesafety  )
	sum := len(m.Criteria) + len(m.Templates) + len(m.Excluded)
	if sum != m.Total {
		m.Excluded = append(m.Excluded, Excluded{
			Key: "unmapped", Text: "", Reason: "映射缺口（守恒失败）—— 属实现缺陷，必须显式暴露",
		})
	}
	return m
}

// CriteriaFromKnowhow pipe knowhow   become data(compatin ). 
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
			// ** first  ize**: basis  "already   firstat  called" dayalready  occur ed  ,   . 
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

// AutomatedRatio returnback  ize example (automated, total) -- **  refertgt**. 
func AutomatedRatio(cs []Criterion) (int, int) {
	n := 0
	for _, c := range cs {
		if !c.Manual {
			n++
		}
	}
	return n, len(cs)
}

// Evidence is   data(basis ruleuse). 
type Evidence struct {
	Kind   string // verified(already  )| hearsay(  called)
	Detail string
}

// PreferVerified  now basis"**already   firstat  called**"; 
// if**onlyhas  called**, e.g. tgt degraded( allowcurbecomealready  ). 
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
