// 手机 App ASR 模拟 · 契约与回归套件（绿）。
//
// 场景：用户对 iPhone 说话 → 系统 ASR 出文本 → App 把文本发给 harness。
// 本文件只喂"ASR 之后的文本"，模拟 App 这一侧；不依赖音频、不依赖真实 LLM
// （Options.Providers 为 nil，纯规则层，确定性可重放）。
//
// 本文件锁定的是**当前已成立且必须保持**的契约（防回归）。
// 已知缺口（当前为红）在同目录 asrsim_gaps_test.go，命名以 TestASRSimGap 开头，
// 可用 `go test ./e2e -skip TestASRSimGap` 单独保持门禁绿。
package e2e

import (
	"context"
	"strings"
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/pipeline"
)

// simRun 模拟 App 投递一条 ASR 文本，返回真实管线结果。
func simRun(t *testing.T, opts *pipeline.Options, text string) pipeline.Outcome {
	t.Helper()
	out, err := pipeline.Run(context.Background(), opts, text)
	if err != nil {
		t.Fatalf("管线执行失败（不应发生）: %v", err)
	}
	return out
}

// simOpts 装配一个允许低风险执行、拒绝人工确认的沙箱 Options。
// 拒绝是刻意的：让不可逆意图在测试里走到"未放行"分支，而不是真的提交/部署。
func simOpts(t *testing.T) *pipeline.Options {
	t.Helper()
	return buildOpts(t, func(string, string) (bool, error) { return false, nil })
}

// TestASRSimEmptyUtteranceAsksWithoutExecuting 空指令（ASR 没听清）必须回问且不执行。
func TestASRSimEmptyUtteranceAsksWithoutExecuting(t *testing.T) {
	opts := simOpts(t)
	for _, text := range []string{"", "   ", "\n"} {
		out := simRun(t, opts, text)
		if out.Ask == "" {
			t.Errorf("%q: 空指令必须回问，实际 Ask 为空", text)
		}
		if out.Verdict.Allowed {
			t.Errorf("%q: 空指令不得放行执行", text)
		}
	}
}

// TestASRSimReceiptAlwaysFourLines 任何非空结果都要给手机四行回执（动作/文件/结果/撤销）。
func TestASRSimReceiptAlwaysFourLines(t *testing.T) {
	opts := simOpts(t)
	for _, text := range []string{"记一下明天开会", "查一下库存", "改一下", "开始测试"} {
		out := simRun(t, opts, text)
		for name, value := range map[string]string{
			"动作": out.View.Action,
			"文件": out.View.Files,
			"结果": out.View.Result,
			"撤销": out.View.Undo,
		} {
			if strings.TrimSpace(value) == "" {
				t.Errorf("%q: 回执行 %q 为空", text, name)
			}
		}
	}
}

// TestASRSimIrreversibleNeverExecutesWithoutHuman 不可逆意图（提交/部署）在未获人工放行时不得执行。
//
// 两条防线任一挡住都算合格：域门（无归属域 → 直接拒绝）或风险闸（human 确认）。
// 唯一不可接受的是"已确认执行"。
func TestASRSimIrreversibleNeverExecutesWithoutHuman(t *testing.T) {
	opts := simOpts(t)
	for _, text := range []string{"在 proj 里提交所有改动并推上去", "提交这批改动", "部署到服务器"} {
		out := simRun(t, opts, text)
		if out.Confirmed {
			t.Errorf("%q: 未获人工放行却已确认执行（decision=%q verdict=%+v）",
				text, out.Decision.Level, out.Verdict)
		}
		if out.Decision.Level != "" && out.Decision.Level != "human" {
			t.Errorf("%q: 不可逆意图裁决应为 human，实际 %q", text, out.Decision.Level)
		}
		if out.Intent.Intent != contract.IntentCommit && out.Intent.Intent != contract.IntentDeploy {
			t.Errorf("%q: 意图 = %q，期望 COMMIT/DEPLOY", text, out.Intent.Intent)
		}
	}
}

// TestASRSimDomainGateDeniesOutOfScope 默认拒绝：不在任何域声明范围内的工具调用必须被域门拦下。
func TestASRSimDomainGateDeniesOutOfScope(t *testing.T) {
	opts := simOpts(t)
	for _, text := range []string{"改一下", "修一下", "开始测试"} {
		out := simRun(t, opts, text)
		if out.Intent.NeedsClarification() {
			continue // 先回问的更安全，不算违约
		}
		if out.Verdict.Allowed {
			t.Errorf("%q: 无域归属的写操作被放行，默认拒绝失效: %+v", text, out.Verdict)
		}
	}
}

// fuzzyCase 是模糊识别用例：ASR 文本本身含糊，可接受"判对"或"回问"，不允许别的。
type fuzzyCase struct {
	id         string
	text       string
	acceptable []string // 可接受意图
	mustAsk    bool     // 是否必须回问
}

// TestASRSimFuzzyAcceptsIntentOrAsk 模糊输入只允许两种结局：判到可接受意图，或明确回问。
// 这条锁的是"宁可回问，不可猜错"的产品红线（设计共识：模糊→精确）。
func TestASRSimFuzzyAcceptsIntentOrAsk(t *testing.T) {
	opts := simOpts(t)
	cases := []fuzzyCase{
		{"fz-01", "推进项目", []string{contract.IntentUnknown}, true},
		{"fz-02", "继续", []string{contract.IntentUnknown}, true},
		{"fz-03", "接下来做什么", []string{contract.IntentUnknown}, true},
		{"fz-04", "那个功能还行", []string{contract.IntentUnknown}, true},
		{"fz-05", "刚才说的那个呢", []string{contract.IntentUnknown}, true},
		{"fz-06", "我要那个", []string{contract.IntentUnknown}, true},
		{"fz-07", "是不是可以了", []string{contract.IntentUnknown}, true},
		{"fz-08", "我想想", []string{contract.IntentUnknown}, true},
		{"fz-09", "先这样吧", []string{contract.IntentUnknown}, true},
		{"fz-10", "嗯那个呃记一下", nil, true}, // 填充词噪声：意图可争议，但必须回问
		{"fz-11", "把这个改一下", nil, true},  // 纯指代无先行词：必须回问
	}
	for _, tc := range cases {
		out := simRun(t, opts, tc.text)
		asked := out.Ask != "" || out.Intent.NeedsClarification()
		if tc.mustAsk && !asked {
			t.Errorf("%s %q: 模糊输入未回问（intent=%s conf=%.2f），会静默执行",
				tc.id, tc.text, out.Intent.Intent, out.Intent.Confidence)
			continue
		}
		if len(tc.acceptable) > 0 && !asked {
			ok := false
			for _, want := range tc.acceptable {
				if out.Intent.Intent == want {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%s %q: 意图 = %q，既不在可接受集合 %v 也未回问",
					tc.id, tc.text, out.Intent.Intent, tc.acceptable)
			}
		}
		if asked && out.Ask != "" && out.Intent.Confidence >= 0.8 &&
			out.Intent.Intent != contract.IntentUnknown {
			// 高置信 + 回问：允许（refer 歧义），但要确认问题文本真的给了用户
			if len([]rune(out.Ask)) < 4 {
				t.Errorf("%s %q: 回问文本过短，用户无法回答: %q", tc.id, tc.text, out.Ask)
			}
		}
	}
}

// TestASRSimMetaInstructionNotTreatedAsRunnableCommand 元指令（"开始测试/推进项目"）不是可执行命令。
// 当前期望：不得产出"无回问 + 放行执行"的组合。
func TestASRSimMetaInstructionNotTreatedAsRunnableCommand(t *testing.T) {
	opts := simOpts(t)
	for _, text := range []string{"推进项目", "开始测试"} {
		out := simRun(t, opts, text)
		if out.Ask == "" && out.Verdict.Allowed && out.Confirmed {
			t.Errorf("%q: 元指令被当作已执行的命令（intent=%s），用户会看到假执行",
				text, out.Intent.Intent)
		}
	}
}
