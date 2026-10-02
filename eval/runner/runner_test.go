package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDecideThreeState 逐条锁定三态判定的优先级。
// 这是整个 Runner 最不能出错的地方：判错方向比指标难看严重得多。
func TestDecideThreeState(t *testing.T) {
	ok := DecisionInput{
		FourTupleComplete:   true,
		PlaceboWithinBounds: true,
		ObservableSteps:     10,
		TotalSteps:          10,
		SupportConditions:   []bool{true, true},
		RefuteConditions:    []bool{false, false},
	}
	cases := []struct {
		name string
		mut  func(*DecisionInput)
		want Verdict
	}{
		{"全绿→支持", func(d *DecisionInput) {}, Support},
		{"四元组不全→证据不足（即使指标全绿）", func(d *DecisionInput) { d.FourTupleComplete = false }, Insufficient},
		{"安慰剂臂越界→证据不足（整轮无效）", func(d *DecisionInput) { d.PlaceboWithinBounds = false }, Insufficient},
		{"观察覆盖不足→证据不足（不等于通过）", func(d *DecisionInput) { d.ObservableSteps = 9 }, Insufficient},
		{"有推翻条件→推翻", func(d *DecisionInput) { d.RefuteConditions[1] = true }, Refute},
		{"推翻优先于支持", func(d *DecisionInput) { d.RefuteConditions[0] = true; d.SupportConditions = []bool{true} }, Refute},
		{"支持条件未满足且无推翻→证据不足", func(d *DecisionInput) { d.SupportConditions[1] = false }, Insufficient},
		{"未注册支持条件→证据不足", func(d *DecisionInput) { d.SupportConditions = nil }, Insufficient},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := ok
			in.SupportConditions = append([]bool(nil), ok.SupportConditions...)
			in.RefuteConditions = append([]bool(nil), ok.RefuteConditions...)
			tc.mut(&in)
			got, reasons := Decide(in)
			if got != tc.want {
				t.Fatalf("判定 = %q，期望 %q；理由=%v", got, tc.want, reasons)
			}
			if len(reasons) == 0 {
				t.Fatal("任何判定都必须给出理由")
			}
		})
	}
}

// TestFourTupleComplete 四元组任一缺项即不可用。
func TestFourTupleComplete(t *testing.T) {
	full := FourTuple{"sha1", "rubric", "prereg", "gpt-x"}
	if !full.Complete() {
		t.Fatal("完整四元组应判定为完整")
	}
	for name, ft := range map[string]FourTuple{
		"缺 commit": {"", "r", "p", "m"},
		"缺 rubric": {"c", "", "p", "m"},
		"缺 prereg": {"c", "r", "", "m"},
		"缺 model":  {"c", "r", "p", ""},
	} {
		if ft.Complete() {
			t.Errorf("%s：应判定为不完整", name)
		}
	}
}

// TestCostUnknownIsNotZero 成本未知必须记 nil（不知道），不能记 0（免费）。
func TestCostUnknownIsNotZero(t *testing.T) {
	zero := 0.0
	c := Cost{ModelUSD: &zero}
	if c.UnknownCount() != 4 {
		t.Fatalf("UnknownCount = %d，期望 4（只有 ModelUSD 是已知的 0）", c.UnknownCount())
	}
	if c.ModelUSD == nil || *c.ModelUSD != 0 {
		t.Fatal("已知的 0 必须保留为 0，不能被当成未知")
	}
}

// TestClosureGuard 一次失败没有变成规则/清单/预警 = 死循环，不是校准。
func TestClosureGuard(t *testing.T) {
	failedNoAction := Settlement{Verdict: Insufficient}
	if err := failedNoAction.ValidateClosureGuard(); err == nil {
		t.Fatal("非 support 且无 action 应被闭环守卫拦下")
	}
	failedWithAction := Settlement{
		Verdict: Refute,
		Actions: []Action{{Kind: "rule", Target: "taskintent.negation", Owner: "coder"}},
	}
	if err := failedWithAction.ValidateClosureGuard(); err != nil {
		t.Fatalf("有 action 的非 support 判定应放行: %v", err)
	}
	if err := (Settlement{Verdict: Support}).ValidateClosureGuard(); err != nil {
		t.Fatalf("support 判定不要求 action: %v", err)
	}
}

// TestFreezeIsAppendOnly 冻结是追加语义：重放写入不得改写已有记录。
func TestFreezeIsAppendOnly(t *testing.T) {
	dir := t.TempDir()
	mk := func(seq int, say string) Record {
		return Record{
			Step: Step{Seq: seq, Say: say},
			Obs:  Observation{Intent: "NOTE", Receipt: "动作：NOTE"},
			At:   time.Unix(int64(seq), 0).UTC(),
		}
	}
	if err := Freeze(dir, mk(1, "第一句")); err != nil {
		t.Fatal(err)
	}
	if err := Freeze(dir, mk(2, "第二句")); err != nil {
		t.Fatal(err)
	}
	// 再写一次 seq=1：必须追加成第三行，而不是覆盖
	if err := Freeze(dir, mk(1, "第一句重放")); err != nil {
		t.Fatal(err)
	}

	recs, err := LoadFrozen(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("记录数 = %d，期望 3（追加语义）", len(recs))
	}
	if recs[0].Step.Say != "第一句" {
		t.Fatalf("第一条被改写了：%q（append-only 被破坏）", recs[0].Step.Say)
	}
	if recs[2].Step.Say != "第一句重放" {
		t.Fatalf("第三条 = %q，期望 '第一句重放'", recs[2].Step.Say)
	}
	if !recs[0].At.Equal(recs[2].At) {
		t.Fatal("同一 seq 的两条记录时间戳应各自独立保留")
	}
}

// TestLoadFrozenMissingFile 未运行过时读冻结文件应报错，而不是静默返回空。
// 静默返回空会让"没跑"看起来像"跑了但没数据"。
func TestLoadFrozenMissingFile(t *testing.T) {
	if _, err := LoadFrozen(t.TempDir()); err == nil {
		t.Fatal("冻结文件不存在时应报错")
	}
}

// TestWriteSettlement 结算是小文件，可入库。
func TestWriteSettlement(t *testing.T) {
	dir := t.TempDir()
	s := Settlement{
		RunID:     "run-1",
		TaskID:    "LHT-0001",
		PreregID:  "PRE-0001",
		FourTuple: FourTuple{"c", "r", "p", "m"},
		Verdict:   Support,
		Reasons:   []string{"ok"},
	}
	if err := WriteSettlement(dir, s); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "verdict.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("结算文件为空")
	}
}
