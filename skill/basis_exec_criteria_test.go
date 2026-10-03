// basis_exec_criteria_test.go —— basis 判据**真正接到执行**（防"只映射不联动"）。
package skill

import "testing"

// ① 映射出的规则 ⇒ **必须真的参与判定**（能构造出"因该规则而改变排序"的用例）
func TestBasisRuleActuallyDrivesRanking(t *testing.T) {
	paradigm, err := ExecutedParadigmCheck("c1", true, "经 check 认定的范式")
	if err != nil {
		t.Fatal(err)
	}
	plain := ClaimedByCaller("普通主张")
	claims := []Claim{plain, paradigm}

	// 用映射出的规则集（含 RuleParadigm）⇒ 范式优先生效
	rules := RulesFromMapping([]BasisMapping{{Total: 1, Mapped: []MappedBasis{{Rule: RuleParadigm}}}})
	got, _ := RankByEvidenceWith(rules, claims)
	if got.Detail != paradigm.Detail {
		t.Errorf("[联动] 映射出的 RuleParadigm 未参与判定: %+v", got)
	}

	// 同输入、**不应用任何规则** ⇒ 保持原序（证明"规则真的被用到了"）
	got2, _ := RankByEvidenceWith(nil, claims)
	if got2.Detail != plain.Detail {
		t.Errorf("[联动] 未应用规则时应保持原序，实际 %+v", got2)
	}
}

// ② 反例：**只映射、不联动 ⇒ 必须红**（当前实现下：不给规则就不生效）
func TestBasisMappingAloneDoesNotAffectRanking(t *testing.T) {
	mapped := []BasisMapping{{Total: 1, Mapped: []MappedBasis{{Rule: RuleTraceable}}}}
	// 有人只做了映射、忘了把规则喂给排序 ⇒ 排序不该"凭空"受影响
	traceable := ClaimedByCaller("可追溯").WithTraceable(true)
	untraceable := ClaimedByCaller("不可追溯")
	got, _ := RankByEvidenceWith(nil, []Claim{untraceable, traceable})
	if got.Detail != untraceable.Detail {
		t.Errorf("[反例] 未联动却影响了排序（说明排序在偷偷用全局规则）: %+v", got)
	}
	// 而一旦联动，规则生效
	got2, _ := RankByEvidenceWith(RulesFromMapping(mapped), []Claim{untraceable, traceable})
	if got2.Detail != traceable.Detail {
		t.Errorf("[联动] RuleTraceable 生效后应选可追溯项: %+v", got2)
	}
}

// ③ manual 条**不得**影响执行（不许假装它在跑）
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

// ④ 真实 knowhow 的映射结果 ⇒ 规则集非空且全部有实现
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
