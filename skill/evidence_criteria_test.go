// evidence_criteria_test.go -- basis  data  + `verified`   define(first -> ). 
package skill

import "testing"

// ① **onlyhasalready    check  produceoccur verified**; calluse  called   hearsay
func TestBasisVerifiedOnlyFromExecutedCheck(t *testing.T) {
	if c := ClaimedByCaller("我保证这个是对的"); c.IsVerified() {
		t.Errorf("[basis] 调用方口头主张被判成 verified（架空 basis 规则）: %+v", c)
	}
	ok, err := ExecutedCheck("checkVerifiedOverHearsay", true, "真跑过")
	if err != nil || !ok.IsVerified() {
		t.Errorf("[basis] 已执行 check 应产生 verified: %+v err=%v", ok, err)
	}
	//   ed ⇒   already  
	fail, _ := ExecutedCheck("checkX", false, "跑失败")
	if fail.IsVerified() {
		t.Errorf("[basis] 未通过的 check 不应算 verified")
	}
	// checkID    ⇒   (verified   has  )
	if _, err := ExecutedCheck("", true, "无来源"); err == nil {
		t.Errorf("[basis] 缺 checkID 却接受了 verified")
	}
}

// ②  rule:     >     ; already   >  head;  form firstonly   timeoccur 
func TestBasisFamilyRanking(t *testing.T) {
	traceable := ClaimedByCaller("有 URL 的转述").WithTraceable(true)
	untraceable := ClaimedByCaller("听说").WithTraceable(false)
	got, degraded := RankByEvidence([]Claim{untraceable, traceable})
	if got.Detail != traceable.Detail || !degraded {
		t.Errorf("[basis] 可追溯未优先（或无已执行证据时应 degraded）: %+v deg=%v", got, degraded)
	}

	executed, _ := ExecutedCheck("checkA", true, "真跑")
	got2, deg2 := RankByEvidence([]Claim{ClaimedByCaller("口头").WithTraceable(true), executed})
	if got2.Detail != executed.Detail || deg2 {
		t.Errorf("[basis] 已执行未优先于可追溯口头: %+v deg=%v", got2, deg2)
	}

	// ④  form first: **    already    check**( head called  )
	paradigm, err := ExecutedParadigmCheck("checkParadigm", true, "核电旧经验（经 check 认定）")
	if err != nil {
		t.Fatal(err)
	}
	other := ClaimedByCaller("自称新范式的猜测")
	got3, _ := RankByEvidence([]Claim{other, paradigm})
	if got3.Detail != paradigm.Detail {
		t.Errorf("[basis] 范式优先未生效: %+v", got3)
	}
}

// ③ revexample: **safetyallonlyhas head   ⇒    degraded=true**( allowcurbecomealready  )
func TestBasisOnlyHearsayIsDegraded(t *testing.T) {
	a := ClaimedByCaller("一方称 A").WithTraceable(true)
	b := ClaimedByCaller("一方称 B")
	_, degraded := RankByEvidence([]Claim{a, b})
	if !degraded {
		t.Errorf("[basis 反例] 全为口头主张却未标 degraded")
	}
	// empty data ⇒ degraded
	if _, d := RankByEvidence(nil); !d {
		t.Errorf("[basis 反例] 空证据应 degraded")
	}
}

// ④ revexample(Lead  decide): **" isnew form"   head  ** -- onlyhasalready   check    paradigm. 
func TestBasisParadigmCannotBeClaimed(t *testing.T) {
	fake := ClaimedByCaller("我这个是 AI 原生范式")
	if fake.IsParadigm() {
		t.Errorf("[basis] 口头主张被判成范式（架空④）: %+v", fake)
	}
	ok, err := ExecutedParadigmCheck("checkParadigm", true, "经 check 认定")
	if err != nil || !ok.IsParadigm() {
		t.Errorf("[basis] 已执行 check 应能认定范式: %+v err=%v", ok, err)
	}
	failed, _ := ExecutedParadigmCheck("checkParadigm", false, "未通过")
	if failed.IsParadigm() {
		t.Errorf("[basis] 未通过的 check 不应认定范式")
	}
	if _, err := ExecutedParadigmCheck("", true, "无来源"); err == nil {
		t.Errorf("[basis] 缺 checkID 却接受了范式认定")
	}
}
