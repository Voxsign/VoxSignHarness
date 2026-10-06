// basis_map.go -- `basis orig  -> evidenceRanking  rule`   table(Lead approveapprove). 
//
//  because: 56   knowhow   basis has 8  , **safetyallis" data first "** ⇒ if  to   4  rule, 
//   izeratefrom 1  to 8. **but"   "    raise   --  allowas  example  . **
//
//  path( use SK-11): **basis  num == already   +  form    **( allow   ); 
//      tgt `manual` + ** by**. 
package skill

import "strings"

//  rulename(and evidenceRanking   4  ruleto ). 
const (
	RuleTraceable = "traceable_over_untraceable" // ①     >     
	RuleExecuted  = "executed_over_claimed"      // ② already   check >  headcalled
	RuleVerified  = "verified_over_hearsay"      // ③ already   >   called
	RuleParadigm  = "paradigm_default"           // ④  form first(  check   )
)

// basisKeywords is**keep **   rule:  inonly  ,  in  by first get   . 
var basisKeywords = []struct {
	rule  string
	words []string
}{
	{RuleVerified, []string{"已查证", "一方称", "证据分级", "证据强度"}},
	{RuleTraceable, []string{"可追溯", "来源", "可核验", "引文", "出处"}},
	{RuleExecuted, []string{"已执行", "check", "实测", "真跑"}},
	{RuleParadigm, []string{"范式", "旧经验", "内核", "默认正确"}},
}

// MappedBasis is  become     basis. 
type MappedBasis struct {
	Skill   string `json:"source_skill"`
	Version string `json:"source_version"`
	Text    string `json:"text"`
	Rule    string `json:"rule"`
}

// ManualBasis is  **no   **  basis( form   +  by,  allow   ). 
type ManualBasis struct {
	Skill   string `json:"source_skill"`
	Version string `json:"source_version"`
	Text    string `json:"text"`
	Reason  string `json:"reason"`
}

// BasisMapping is  close (**  **). 
type BasisMapping struct {
	Mapped []MappedBasis `json:"mapped"`
	Manual []ManualBasis `json:"manual"`
	Total  int           `json:"total"`
}

// RuleOfBasis returnback basis orig to   rule; ok=false tableshow**    **(  by). 
//
// ⚠️ keep  first:      , also   (examplee.g."close first "  style,    data first ). 
func RuleOfBasis(text string) (string, bool, string) {
	t := strings.ToLower(text)
	for _, k := range basisKeywords {
		for _, w := range k.words {
			if strings.Contains(t, strings.ToLower(w)) {
				return k.rule, true, ""
			}
		}
	}
	return "", false, "不属于证据优先级族（四条规则均不匹配）—— 按 manual 处理"
}

// MapBasis  **  **  . 
func MapBasis(skillID, version string, basis []string) BasisMapping {
	m := BasisMapping{Total: len(basis)}
	for _, t := range basis {
		if rule, ok, _ := RuleOfBasis(t); ok {
			m.Mapped = append(m.Mapped, MappedBasis{Skill: skillID, Version: version, Text: t, Rule: rule})
			continue
		}
		_, _, reason := RuleOfBasis(t)
		m.Manual = append(m.Manual, ManualBasis{Skill: skillID, Version: version, Text: t, Reason: reason})
	}
	return m
}

// BasisAutomationRatio returnback (already  num,  num) -- **unique   refertgt**. 
func BasisAutomationRatio(ms []BasisMapping) (int, int) {
	ok, total := 0, 0
	for _, m := range ms {
		ok += len(m.Mapped)
		total += m.Total
	}
	return ok, total
}
