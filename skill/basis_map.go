// basis_map.go —— `basis 原文 → evidenceRanking 族规则` 映射表（Lead 批准）。
//
// 动因：56 条 knowhow 里 basis 有 8 条，**全都是"证据优先级"** ⇒ 若能映到族里 4 条规则，
// 自动化率从 1 抬到 8。**但"能映射"必须经得起推敲 —— 不许为抬比例硬凑。**
//
// 口径（沿用 SK-11）：**basis 条数 == 已映射 + 显式不能映射**（不许静默丢）；
// 不能映射的标 `manual` + **理由**。
package skill

import "strings"

// 族规则名（与 evidenceRanking 的 4 条规则对应）。
const (
	RuleTraceable = "traceable_over_untraceable" // ① 可追溯 > 不可追溯
	RuleExecuted  = "executed_over_claimed"      // ② 已执行 check > 口头称
	RuleVerified  = "verified_over_hearsay"      // ③ 已查证 > 一方称
	RuleParadigm  = "paradigm_default"           // ④ 范式优先（经 check 认定）
)

// basisKeywords 是**保守**的映射规则：命中才映射，命中多条按优先级取第一条。
var basisKeywords = []struct {
	rule  string
	words []string
}{
	{RuleVerified, []string{"已查证", "一方称", "证据分级", "证据强度"}},
	{RuleTraceable, []string{"可追溯", "来源", "可核验", "引文", "出处"}},
	{RuleExecuted, []string{"已执行", "check", "实测", "真跑"}},
	{RuleParadigm, []string{"范式", "旧经验", "内核", "默认正确"}},
}

// MappedBasis 是一条成功映射的 basis。
type MappedBasis struct {
	Skill   string `json:"source_skill"`
	Version string `json:"source_version"`
	Text    string `json:"text"`
	Rule    string `json:"rule"`
}

// ManualBasis 是一条**无法映射**的 basis（显式记录 + 理由，不许静默丢）。
type ManualBasis struct {
	Skill   string `json:"source_skill"`
	Version string `json:"source_version"`
	Text    string `json:"text"`
	Reason  string `json:"reason"`
}

// BasisMapping 是映射结果（**守恒**）。
type BasisMapping struct {
	Mapped []MappedBasis `json:"mapped"`
	Manual []ManualBasis `json:"manual"`
	Total  int           `json:"total"`
}

// RuleOfBasis 返回 basis 原文对应的族规则；ok=false 表示**不能映射**（附理由）。
//
// ⚠️ 保守优先：宁可不映射，也不硬凑（例如"结论先行"属 style，不属证据优先级）。
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

// MapBasis 做**守恒**映射。
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

// BasisAutomationRatio 返回 (已映射数, 总数) —— **唯一的能力指标**。
func BasisAutomationRatio(ms []BasisMapping) (int, int) {
	ok, total := 0, 0
	for _, m := range ms {
		ok += len(m.Mapped)
		total += m.Total
	}
	return ok, total
}
