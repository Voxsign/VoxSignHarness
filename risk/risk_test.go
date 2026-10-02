package risk

import (
	"testing"

	"voicesign-harness/contract"
)

func TestStaticImpact(t *testing.T) {
	cases := []struct {
		name string
		in   ImpactInput
		want string
	}{
		{"ref>=8 high", ImpactInput{RefCount: 8}, contract.ImpactHigh},
		{"ref>=3 medium", ImpactInput{RefCount: 3}, contract.ImpactMedium},
		{"ref<3 small", ImpactInput{RefCount: 1}, contract.ImpactSmall},
		{"heat boosts small->medium", ImpactInput{RefCount: 0, Heat: 10}, contract.ImpactMedium},
		{"low heat stays small", ImpactInput{RefCount: 0, Heat: 3}, contract.ImpactSmall},
	}
	for _, c := range cases {
		if got := StaticImpact(c.in); got != c.want {
			t.Errorf("%s: StaticImpact=%q want %q", c.name, got, c.want)
		}
	}
}

// 用例 9：不可逆 → human（硬门禁，无视机械信号）。
func TestEvaluate_IrreversibleHuman(t *testing.T) {
	for _, it := range []contract.Intent{
		{Intent: contract.IntentCommit, Risk: &contract.RiskBaseline{Reversible: true, Impact: contract.ImpactSmall}},
		{Intent: contract.IntentDeploy},
		{Intent: contract.IntentEdit, Params: map[string]string{"action": "delete"}},
		{Intent: contract.IntentEdit, Space: "vault-creds"},
	} {
		d := Evaluate(it, ImpactInput{RefCount: 0, Heat: 0})
		if d.Level != contract.ConfirmHuman {
			t.Errorf("intent=%+v: level=%q want human", it.Intent, d.Level)
		}
	}
}

// 用例 8：可逆 + 小影响 + 高置信 → auto。
func TestEvaluate_AutoSmallHigh(t *testing.T) {
	it := contract.Intent{
		Intent:     contract.IntentNote,
		Confidence: 0.9,
		Risk:       &contract.RiskBaseline{Reversible: true, Impact: contract.ImpactSmall},
	}
	d := Evaluate(it, ImpactInput{RefCount: 1, Heat: 0})
	if d.Level != contract.ConfirmAuto {
		t.Fatalf("level=%q want auto", d.Level)
	}
}

func TestEvaluate_AutoSmallLow(t *testing.T) {
	it := contract.Intent{
		Intent:     contract.IntentQuery,
		Confidence: 0.4,
		Risk:       &contract.RiskBaseline{Reversible: true},
	}
	d := Evaluate(it, ImpactInput{RefCount: 1})
	if d.Level != contract.ConfirmAuto || d.Reason == "" {
		t.Fatalf("level=%q reason=%q want auto+高亮", d.Level, d.Reason)
	}
}

func TestEvaluate_LightMedium(t *testing.T) {
	it := contract.Intent{Intent: contract.IntentEdit, Risk: &contract.RiskBaseline{Reversible: true}}
	d := Evaluate(it, ImpactInput{RefCount: 4}) // medium
	if d.Level != contract.ConfirmLight {
		t.Fatalf("level=%q want light", d.Level)
	}
}

func TestEvaluate_StrongHigh(t *testing.T) {
	it := contract.Intent{Intent: contract.IntentDebug, Risk: &contract.RiskBaseline{Reversible: true}}
	d := Evaluate(it, ImpactInput{RefCount: 10}) // high
	if d.Level != contract.ConfirmStrong {
		t.Fatalf("level=%q want strong", d.Level)
	}
}

func TestEvaluate_BaselineIrreversible(t *testing.T) {
	it := contract.Intent{Intent: contract.IntentNote, Risk: &contract.RiskBaseline{Reversible: false}}
	if d := Evaluate(it, ImpactInput{}); d.Level != contract.ConfirmHuman {
		t.Fatalf("level=%q want human (基线不可逆)", d.Level)
	}
}

func TestGuard_DowngradeAfterThree(t *testing.T) {
	g := NewGuard()
	for i := 1; i <= 3; i++ {
		if g.ShouldDowngrade("foo.go") {
			t.Fatalf("第 %d 次强确认不应降级", i)
		}
	}
	if !g.ShouldDowngrade("foo.go") { // 第 4 次
		t.Fatal("第 4 次同路径强确认应降级")
	}
}

func TestGuard_ResetsOnDifferentPath(t *testing.T) {
	g := NewGuard()
	g.ShouldDowngrade("a.go")
	g.ShouldDowngrade("a.go")
	g.ShouldDowngrade("b.go") // 换路径，计数重置
	if g.ShouldDowngrade("b.go") {
		t.Fatal("换路径后不应立即降级")
	}
	if !g.ShouldDowngrade("b.go") {
		// b.go 连续第 3 次仍未到 >3
	}
}
