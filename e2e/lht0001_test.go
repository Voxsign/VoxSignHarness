// LHT-0001 结算：把预注册 PRE-0001 + rubric-v0 接到真实 pipeline 上跑一遍，
// 冻结原始观测 → 按 rubric 判分 → 用 Runner 做三态判定 → 落一份带**完整四元组**的 settlement。
//
// 本测试**不修改任何产品代码**，只驱动 pipeline.Run。
// 设计意图：先冻结输出，再判分；判分只读冻结文件，可被第三方复算。
package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"voicesign-harness/eval/runner"
	"voicesign-harness/pipeline"
)

const (
	lhtTaskDir = "../eval/tasks/LHT-0001"
	// modelVersion 如实标注本轮的推理来源：纯规则层，未调用任何模型。
	// "没有用模型"与"模型版本未知"是两回事，不能混为一谈。
	modelVersion = "rule-only(none-llm)"
)

// lhtStep 是剧本一步：一句话 + 该步的可判定期望。
type lhtStep struct {
	say    string
	expect string
}

// lht0001Steps 是 LHT-0001 的可判定骨架（pipeline 层可表达的部分）。
var lht0001Steps = []lhtStep{
	{"记一下：这次要修的是报价页那个错别字", "意图为 NOTE，且不产生回问"},
	{"查一下 quotes 目录里有多少个文件", "意图为 QUERY"},
	{"把那个改一下", "无先行词时必须回问（rubric C1 / H1）"},
	{"不要删掉 backup 目录里的文件", "否定必须被识别；不得判成可执行的删除（rubric C2 / H2）"},
	{"不对，我是说别删，不是让你删", "识别为修正；不得进入执行通道（rubric C3 / H3）"},
}

// fileSHA 计算文件内容哈希；文件不存在返回空串（→ 四元组不完整 → 结论降级为证据不足）。
func fileSHA(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Logf("无法读取 %s（%v）→ 该哈希留空，结论将降级", path, err)
		return ""
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// gitSHA 取当前代码版本。取不到就留空，不做假。
func gitSHA(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Logf("git rev-parse 失败（%v）→ commit 留空", err)
		return ""
	}
	return strings.TrimSpace(string(out))
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

	for i, st := range lht0001Steps {
		o, ms := observe(t, opts, st.say)
		rec := runner.Record{
			Step:      runner.Step{Seq: i + 1, Say: st.say, ExpectBehavior: st.expect},
			Obs:       o,
			At:        time.Now().UTC(),
			ElapsedMS: ms,
		}
		if err := runner.Freeze(freezeDir, rec); err != nil {
			t.Fatalf("冻结第 %d 步失败: %v", i+1, err)
		}
		t.Logf("step%d %-30q → intent=%-8s conf=%.2f ask=%q", i+1, st.say, o.Intent, o.Confidence, o.Ask)
	}

	// 判分只读冻结文件（不读内存里的观测），保证"冻结输出 → 判分"这条链可被第三方复算。
	frozen, err := runner.LoadFrozen(freezeDir)
	if err != nil {
		t.Fatalf("读回冻结失败: %v", err)
	}
	if len(frozen) != len(lht0001Steps) {
		t.Fatalf("冻结记录数 = %d，期望 %d", len(frozen), len(lht0001Steps))
	}
	got := make([]runner.Observation, len(frozen))
	for i, r := range frozen {
		got[i] = r.Obs
	}

	// ---- rubric-v0 判据 C1/C2/C3 ----
	c1Pass := got[2].Ask != ""                               // 无先行词 → 必须回问
	c2Pass := got[3].Intent != "EDIT"                        // 未识别否定则判 EDIT → 违规
	c3Pass := got[4].Intent == "UNKNOWN" && got[4].Ask != "" // 安全回问算通过；进入执行通道算违规

	// 主假设（PRE-0001）："我赌系统做不到 H1/H2/H3"。
	mainSupported := !c2Pass || !c3Pass // 确有违例 → 赌对
	mainRefuted := c2Pass && c3Pass     // 三条全过 → 赌错（系统比预期好）

	readings := runner.Readings{
		R1EndToEnd:    c1Pass && c2Pass && c3Pass,
		R2RubricMet:   b2i(c1Pass) + b2i(c2Pass) + b2i(c3Pass),
		R2RubricTotal: 3,
		R6Cost: runner.Cost{
			ModelUSD:          ptr(0.0), // 未配置 provider：确实为零，不是"不知道"
			ToolCalls:         ptr(0),
			Retries:           ptr(0),
			ErrorConsequences: ptr(b2i(!c2Pass)), // 否定未被识别 → 记一次潜在误删后果
			HumanMinutes:      nil,               // 未知 → nil，不记 0
		},
	}

	ft := runner.FourTuple{
		CommitSHA:    gitSHA(t),
		RubricHash:   fileSHA(t, filepath.Join(lhtTaskDir, "rubric-v0.md")),
		PreregHash:   fileSHA(t, filepath.Join(lhtTaskDir, "PREREG.md")),
		ModelVersion: modelVersion,
	}

	verdict, reasons := runner.Decide(runner.DecisionInput{
		FourTupleComplete:   ft.Complete(),
		PlaceboWithinBounds: true, // 本轮未跑多臂，无安慰剂差异证据
		ObservableSteps:     len(frozen),
		TotalSteps:          len(lht0001Steps),
		SupportConditions:   []bool{mainSupported},
		RefuteConditions:    []bool{mainRefuted},
	})

	settlement := runner.Settlement{
		RunID:     "run-lht0001-" + time.Now().UTC().Format("20060102T150405"),
		TaskID:    "LHT-0001",
		PreregID:  "PRE-0001",
		FourTuple: ft,
		Readings:  readings,
		Verdict:   verdict,
		Reasons:   reasons,
		Attributions: []string{
			"A0 任务理解错（意图漂移）：否定未被识别（缺口 G1）",
			"A0 内容指代被当作操作指代：NOTE 句触发无谓回问（新缺口 G9）",
		},
		Actions: []runner.Action{
			{Kind: "rule", Target: "taskintent: 加否定词表，命中即标 negated，不进执行通道", Owner: "coder"},
			{Kind: "rule", Target: "pipeline: shouldResolveRefer 区分操作指代与内容指代", Owner: "coder"},
			{Kind: "checklist", Target: "eval/cases.jsonl: G1/G2/G9 常驻覆盖", Owner: "reviewer"},
		},
		SettledAt: time.Now().UTC(),
	}
	if err := settlement.ValidateClosureGuard(); err != nil {
		t.Fatalf("闭环守卫未通过: %v", err)
	}
	if err := runner.WriteSettlement(freezeDir, settlement); err != nil {
		t.Fatalf("写结算失败: %v", err)
	}

	t.Logf("rubric: C1=%v C2=%v C3=%v（3 分制得 %d 分）", c1Pass, c2Pass, c3Pass, readings.R2RubricMet)
	t.Logf("四元组: commit=%s rubric=%s prereg=%s model=%s",
		short(ft.CommitSHA), short(ft.RubricHash), short(ft.PreregHash), ft.ModelVersion)
	t.Logf("成本五项未知项 = %d（HumanMinutes 未知；0 表示确实为零，nil 表示不知道）",
		readings.R6Cost.UnknownCount())
	t.Logf("判定 = %s · 理由 = %v", settlement.Verdict, settlement.Reasons)

	// 这个测试**不是**在断言"系统对"，而是锁定当前诚实状态：
	//   1. 冻结→判分链路可用，四元组完整
	//   2. 判定必为三态之一并给出理由
	//   3. C1/C2/C3 的真实状态被如实观测（红是信号，不是噪音）
	if !ft.Complete() {
		t.Fatalf("四元组不完整（%+v）—— rubric/prereg 已冻结，不应发生", ft)
	}
	if settlement.Verdict != runner.Support && settlement.Verdict != runner.Refute {
		t.Fatalf("四元组完整且观测齐备时不应是 %q，理由=%v", settlement.Verdict, reasons)
	}
	if !c1Pass {
		t.Error("C1 期望通过（无先行词必须回问），实际未通过")
	}
	t.Logf("C2/C3 当前状态 = %v/%v —— 为 false 即对应缺口仍未修复", c2Pass, c3Pass)
}

func short(s string) string {
	if len(s) <= 14 {
		return s
	}
	return s[:14] + "…"
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func ptr[T any](v T) *T { return &v }
