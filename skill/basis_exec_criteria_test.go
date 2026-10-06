// basis_exec_criteria_test.go -- basis  data** posconnectto  **(prevent"only     "). 
package skill

import "testing"

// ①   out rule ⇒ **     and  **(   out"because rulebutmodifychange  " useexample)
func TestBasisRuleActuallyDrivesRanking(t *testing.T) {
	paradigm, err := ExecutedParadigmCheck("c1", true, "经 check 认定的范式")
	if err != nil {
		t.Fatal(err)
	}
	plain := ClaimedByCaller("普通主张")
	claims := []Claim{plain, paradigm}

	// use  out rule (  RuleParadigm)⇒  form firstoccur 
	rules := RulesFromMapping([]BasisMapping{{Total: 1, Mapped: []MappedBasis{{Rule: RuleParadigm}}}})
	got, _ := RankByEvidenceWith(rules, claims)
	if got.Detail != paradigm.Detail {
		t.Errorf("[联动] 映射出的 RuleParadigm 未参与判定: %+v", got)
	}

	// same in, **  use  rule** ⇒ keepkeeporig (  "rule  beuseto")
	got2, _ := RankByEvidenceWith(nil, claims)
	if got2.Detail != plain.Detail {
		t.Errorf("[联动] 未应用规则时应保持原序，实际 %+v", got2)
	}
}

// ② revexample: **only  ,     ⇒    **(curbefore nowunder:  giverulethen occur )
func TestBasisMappingAloneDoesNotAffectRanking(t *testing.T) {
	mapped := []BasisMapping{{Total: 1, Mapped: []MappedBasis{{Rule: RuleTraceable}}}}
	// has only   ,  piperule give   ⇒     " empty"accept  
	traceable := ClaimedByCaller("可追溯").WithTraceable(true)
	untraceable := ClaimedByCaller("不可追溯")
	got, _ := RankByEvidenceWith(nil, []Claim{untraceable, traceable})
	if got.Detail != untraceable.Detail {
		t.Errorf("[反例] 未联动却影响了排序（说明排序在偷偷用全局规则）: %+v", got)
	}
	// but    , ruleoccur 
	got2, _ := RankByEvidenceWith(RulesFromMapping(mapped), []Claim{untraceable, traceable})
	if got2.Detail != traceable.Detail {
		t.Errorf("[联动] RuleTraceable 生效后应选可追溯项: %+v", got2)
	}
}

// ③ manual  **  **    ( allow     )
func TestManualBasisDoesNotParticipate(t *testing.T) {
	ms := []BasisMapping{{
		Total:  2,
		Mapped: []MappedBasis{{Rule: RuleVerified}},
		Manual: []ManualBasis{{Text: "知识是活资产", Reason: "不属于证据优先级族"}},
	}}
	rules := RulesFromMapping(ms)
	if len(rules) != 1 || rules[0] != RuleVerified {
		t.Errorf("[联动] manual 条混进了执行规则集: %v", rules)
	}
}

// ④    knowhow    close  ⇒ rule  emptyandsafety has now
func TestRealBasisMappingRulesAreImplemented(t *testing.T) {
	var ms []BasisMapping
	for id, kh := range realKnowhow() {
		ms = append(ms, MapBasis(id, "self-fetched", kh.Basis))
	}
	rules := RulesFromMapping(ms)
	if len(rules) == 0 {
		t.Fatal("[联动] 真实 knowhow 未映射出任何规则")
	}
	for _, r := range rules {
		if _, ok := ruleAppliers[r]; !ok {
			t.Errorf("[联动] 规则 %q 无实现（映射了却不执行）", r)
		}
	}
	t.Logf("真实可执行规则集: %v", rules)
}
