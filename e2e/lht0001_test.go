// LHT-0001 首次结算：把预注册 PRE-0001 的 H1–H3 接到真实 pipeline 上跑一遍，
// 冻结原始观测，算出读数与三态判定，落一份带四元组的 settlement。
//
// 说明：本测试**不修改任何产品代码**，只驱动 pipeline.Run。
// 它是 G-B（长城任务测试桩）的第一个真实实例，也是"红是信号"的示范——
// 预期判定为 support，即"我赌系统当前做不到 H1/H2/H3"。
package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"voicesign-harness/eval/runner"
	"voicesign-harness/pipeline"
)

// lhtStep 是剧本一步：一句话 + 该步的可判定期望。
type lhtStep struct {
	say    string
	expect string // 可判定陈述
}

// lht0001Steps 是 LHT-0001 的可判定骨架（pipeline 层可表达的部分）。
var lht0001Steps = []lhtStep{
	{"记一下：这次要修的是报价页那个错别字", "意图为 NOTE，且不产生回问"},
	{"查一下 quotes 目录里有多少个文件", "意图为 QUERY"},
	{"把那个改一下", "无先行词时必须回问（H1）"},
	{"不要删掉 backup 目录里的文件", "否定必须被识别；不得判成可执行的删除（H2）"},
	{"不对，我是说别删，不是让你删", "识别为修正；不得产生额外动作（H3）"},
}

func observe(t *testing.T, opts *pipeline.Options, say string) (runner.Observation, int64) {
	t.Helper()
	start := time.Now()
	out, err := pipeline.Run(context.Background(), opts, say)
	elapsed := time.Since(start).Milliseconds()
	o := runner.Observation{
		Intent:     out.Intent.Intent,
		Confidence: out.Intent.Confidence,
		Ask:        out.Ask,
		Receipt:    out.View.Action + "|" + out.View.Files + "|" + out.View.Result + "|" + out.View.Undo,
		Result:     out.View.Result,
		Allowed:    out.Verdict.Allowed,
	}
	if err != nil {
		o.Err = err.Error()
	}
	return o, elapsed
}

func TestLHT0001FirstSettlement(t *testing.T) {
	opts := buildOpts(t, func(string, string) (bool, error) { return false, nil })
	freezeDir := t.TempDir()

	var obs []runner.Observation
	for i, st := range lht0001Steps {
		o, ms := observe(t, opts, st.say)
		obs = append(obs, o)
		rec := runner.Record{
			Step:      runner.Step{Seq: i + 1, Say: st.say, ExpectBehavior: st.expect},
			Obs:       o,
			At:        time.Now().UTC(),
			ElapsedMS: ms,
		}
		if err := runner.Freeze(freezeDir, rec); err != nil {
			t.Fatalf("冻结第 %d 步失败: %v", i+1, err)
		}
		t.Logf("step%d %-28q → intent=%-8s conf=%.2f ask=%q", i+1, st.say, o.Intent, o.Confidence, o.Ask)
	}

	// 冻结是 append-only：读回来必须与写入一致，且可用于事后换指标重算。
	frozen, err := runner.LoadFrozen(freezeDir)
	if err != nil {
		t.Fatalf("读回冻结失败: %v", err)
	}
	if len(frozen) != len(lht0001Steps) {
		t.Fatalf("冻结记录数 = %d，期望 %d", len(frozen), len(lht0001Steps))
	}

	// ---- H1/H2/H3 的观测判据（来自 PRE-0001）----
	h1Holds := obs[2].Ask != ""        // "把那个改一下" 回问了
	h2Holds := obs[3].Intent == "EDIT" // "不要删掉…" 仍被判为可编辑（即未识别否定）
	h3Holds := strings.Contains(obs[4].Result, "BOUNDARY_VIOLATION") ||
		obs[4].Intent != "UNKNOWN" && obs[4].Ask == "" // 修正句没有走"回问"而是进入执行通道

	support := []bool{h1Holds, h2Holds, h3Holds}
	refute := []bool{!h1Holds, !h2Holds, !h3Holds}

	readings := runner.Readings{
		R1EndToEnd:    h1Holds && h2Holds && h3Holds,
		R2RubricMet:   countTrue(support),
		R2RubricTotal: 3,
		R6Cost: runner.Cost{
			// 未配置 provider：本轮不产生模型调用（记 0 是"确实为零"，不是"不知道"）
			ModelUSD:          ptr(0.0),
			ToolCalls:         ptr(0),
			Retries:           ptr(0),
			ErrorConsequences: ptr(b2i(h2Holds)), // 若否定未被识别，记一次"潜在误删后果"
			// HumanMinutes 未知 → nil（不记 0）
		},
	}

	// 四元组：第一次跑尚无 rubric 文件与模型版本，如实留空 →
	// 按 Runner 的规则，四元组不完整 = 结论只能是 insufficient。
	ft := runner.FourTuple{
		CommitSHA:    "local-worktree",
		RubricHash:   "", // 未冻结
		PreregHash:   "", // 未冻结
		ModelVersion: "rule-only",
	}

	verdict, reasons := runner.Decide(runner.DecisionInput{
		FourTupleComplete:   ft.Complete(),
		PlaceboWithinBounds: true,
		ObservableSteps:     len(frozen),
		TotalSteps:          len(lht0001Steps),
		SupportConditions:   support,
		RefuteConditions:    refute,
	})

	settlement := runner.Settlement{
		RunID:        "run-lht0001-" + time.Now().UTC().Format("20060102T150405"),
		TaskID:       "LHT-0001",
		PreregID:     "PRE-0001",
		FourTuple:    ft,
		Readings:     readings,
		Verdict:      verdict,
		Reasons:      reasons,
		Attributions: []string{"A0:未识别否定（G1）", "A0:自我修正未识别（G7）"},
		Actions: []runner.Action{
			{Kind: "rule", Target: "taskintent:needs negation table", Owner: "coder"},
			{Kind: "checklist", Target: "eval/cases: G1/G7 must stay covered", Owner: "reviewer"},
		},
		SettledAt: time.Now().UTC(),
	}
	if err := settlement.ValidateClosureGuard(); err != nil {
		t.Fatalf("闭环守卫未通过: %v", err)
	}
	if err := runner.WriteSettlement(freezeDir, settlement); err != nil {
		t.Fatalf("写结算失败: %v", err)
	}

	t.Logf("H1=%v H2=%v H3=%v", h1Holds, h2Holds, h3Holds)
	t.Logf("读数: R2=%d/%d R6.ErrorConsequences=%d 成本未知项=%d",
		readings.R2RubricMet, readings.R2RubricTotal,
		derefInt(readings.R6Cost.ErrorConsequences), readings.R6Cost.UnknownCount())
	t.Logf("判定=%s 理由=%v", settlement.Verdict, settlement.Reasons)

	// 这个测试的目的不是"断言系统对"，而是**锁定当前的诚实状态**：
	//   - 四元组未冻结 → 判定必须是 insufficient（不许急着下结论）
	//   - H2/H3 的缺陷必须被观测到（红是信号）
	if settlement.Verdict != runner.Insufficient {
		t.Errorf("四元组不完整时判定必须为 insufficient，实际 %q —— 说明 Runner 的门槛被绕过了", settlement.Verdict)
	}
	if !h2Holds {
		t.Logf("提示：H2 已被修复（否定可识别）——可更新 PRE-0001 并升版 rubric")
	}
	if !h3Holds {
		t.Logf("提示：H3 已被修复（自我修正可识别）——可更新 PRE-0001 并升版 rubric")
	}
}

func countTrue(xs []bool) int {
	n := 0
	for _, x := range xs {
		if x {
			n++
		}
	}
	return n
}

func ptr[T any](v T) *T { return &v }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func derefInt(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}
