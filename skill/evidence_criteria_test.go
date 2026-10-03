// evidence_criteria_test.go —— basis 判据族 + `verified` 来源定义（先红→绿）。
package skill

import "testing"

// ① **只有已执行的 check 能产生 verified**；调用方自称一律 hearsay
func TestBasisVerifiedOnlyFromExecutedCheck(t *testing.T) {
	if c := ClaimedByCaller("我保证这个是对的"); c.IsVerified() {
		t.Errorf("[basis] 调用方口头主张被判成 verified（架空 basis 规则）: %+v", c)
	}
	ok, err := ExecutedCheck("checkVerifiedOverHearsay", true, "真跑过")
	if err != nil || !ok.IsVerified() {
		t.Errorf("[basis] 已执行 check 应产生 verified: %+v err=%v", ok, err)
	}
	// 未通过 ⇒ 不算已查证
	fail, _ := ExecutedCheck("checkX", false, "跑失败")
	if fail.IsVerified() {
		t.Errorf("[basis] 未通过的 check 不应算 verified")
	}
	// checkID 缺失 ⇒ 报错（verified 必须有来源）
	if _, err := ExecutedCheck("", true, "无来源"); err == nil {
		t.Errorf("[basis] 缺 checkID 却接受了 verified")
	}
}

// ② 族规则：可追溯 > 不可追溯；已执行 > 口头；范式优先只在打平时生效
func TestBasisFamilyRanking(t *testing.T) {
	traceable := ClaimedByCaller("有 URL 的转述").withTraceable(true)
	untraceable := ClaimedByCaller("听说").withTraceable(false)
	got, degraded := RankByEvidence([]Claim{untraceable, traceable})
	if got.Detail != traceable.Detail || !degraded {
		t.Errorf("[basis] 可追溯未优先（或无已执行证据时应 degraded）: %+v deg=%v", got, degraded)
	}

	executed, _ := ExecutedCheck("checkA", true, "真跑")
	got2, deg2 := RankByEvidence([]Claim{ClaimedByCaller("口头").withTraceable(true), executed})
	if got2.Detail != executed.Detail || deg2 {
		t.Errorf("[basis] 已执行未优先于可追溯口头: %+v deg=%v", got2, deg2)
	}

	// ④ 范式优先：两者都只是口头且都不可追溯时
	paradigm := ClaimedByCaller("核电旧经验").withParadigm(true)
	other := ClaimedByCaller("新的猜测")
	got3, _ := RankByEvidence([]Claim{other, paradigm})
	if got3.Detail != paradigm.Detail {
		t.Errorf("[basis] 范式优先未生效: %+v", got3)
	}
}

// ③ 反例：**全都只有口头主张 ⇒ 必须 degraded=true**（不许当成已核实）
func TestBasisOnlyHearsayIsDegraded(t *testing.T) {
	a := ClaimedByCaller("一方称 A").withTraceable(true)
	b := ClaimedByCaller("一方称 B")
	_, degraded := RankByEvidence([]Claim{a, b})
	if !degraded {
		t.Errorf("[basis 反例] 全为口头主张却未标 degraded")
	}
	// 空证据 ⇒ degraded
	if _, d := RankByEvidence(nil); !d {
		t.Errorf("[basis 反例] 空证据应 degraded")
	}
}
