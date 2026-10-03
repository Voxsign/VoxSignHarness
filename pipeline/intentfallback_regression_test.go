// 本文件是 builder 的回归保护，不属于 stubsmith 的判据桩；判据桩见 e2e/arch_test.go。
//
// 覆盖对象：pipeline/pipeline.go 的 llmIntentFallback 豁免条件。本轮把它从
// 白名单 switch（negation/meta/conditional/multi_action）改成结构性条件：
//
//	Conflict != "" && Ask != ""   → 仲裁已发生且落在 Ask 确认态，下游不得覆写。
//
// 为什么必须有本文件（技能 §9「无样本就报绿」）：
//
//	这条路径此前**零测试覆盖**。`go test ./...` 全绿，却掩盖了一个功能性失效：
//	若条件只写 `Ask != ""`，因为 ClassifyTask 初始化即带
//	`Ask: taskAskTemplate`（UNKNOWN 必然 Ask 非空），回退会**永不触发** ——
//	M7 ①「规则判 UNKNOWN/低置信且像问句 → 调 fast 补分类」静默死亡。
//
// 因此这里按**四类**做双向反例（技能 §3 硬要求 4：至少一个"该收未收"和一个
// "不该收却收了"）：
//
//	① 豁免       —— 仲裁 Ask（含原白名单漏收的 debug_plan）绝不被覆写；
//	② 可回退     —— Conflict 非空但 Ask 为空的合法路径（delete 等）仍能回退；
//	③ 原本漏收   —— debug_plan 是已存在的真实漏洞，新条件必须补上豁免；
//	④ 默认 UNKNOWN —— Ask 非空但 Conflict 为空，回退仍必须被调用并救回意图。
//
// 只断言"该拦的拦住了"会漏掉"不该拦的也被拦住了"——这正是本轮事故的形态。
package pipeline

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/input"
	"voicesign-harness/provider"
)

// intentJSONQuery 是 fast provider 固定返回的合法分类 JSON（注意：作为 JSON 字符串
// 内容嵌入请求响应体，故引号需转义）。
const intentJSONQuery = `{\"intent\":\"QUERY\",\"confidence\":0.9}`

// fallbackFixture 构造一个返回固定意图 JSON 的 fast provider，并返回调用计数器。
func fallbackFixture(t *testing.T) (*Options, *int32) {
	t.Helper()
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w,
			`{"choices":[{"message":{"content":"`+intentJSONQuery+`"}}],"usage":{}}`)
	}))
	t.Cleanup(ts.Close)

	trueVal := true
	cfg := config.Config{
		Global: config.Global{LLMTimeoutMs: 5000, FastResponseMs: 10000},
		Providers: []config.Provider{
			{Name: "fast", Kind: config.OpenAIKind, Endpoint: ts.URL, Model: "gpt-test",
				APIKey: "k", ResponseFormat: &trueVal,
				Params: map[string]any{"use_max_completion_tokens": true}},
		},
	}
	reg, err := provider.NewRegistry(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &Options{Cfg: &cfg, Providers: reg}, &calls
}

// TestIntentFallbackExemptsArbitrationAsks 锁住类别 ①/③：
// 任何"Conflict 非空且 Ask 非空"的仲裁都必须原样返回，且**不得**发起 LLM 调用。
func TestIntentFallbackExemptsArbitrationAsks(t *testing.T) {
	o, calls := fallbackFixture(t)
	ctx := context.Background()
	// 传一个带问句特征的 text，确保"假如没被豁免，回退一定会触发" ——
	// 否则测试可能因为 hasQ=false 而假绿。
	const questionLike = "这样行吗？"

	cases := []struct {
		name string
		it   contract.Intent
	}{
		{"negation", contract.Intent{Intent: contract.IntentAsk, Conflict: contract.ConflictNegation,
			Confidence: 0.9, Ask: "我听到的是「不要」——确认不执行这个动作吗？"}},
		{"meta", contract.Intent{Intent: contract.IntentAsk, Conflict: contract.ConflictMeta,
			Confidence: 0.9, Ask: "「开始」是让我继续推进，还是要我现在就执行后面的动作？"}},
		{"conditional", contract.Intent{Intent: contract.IntentAsk, Conflict: contract.ConflictConditional,
			Confidence: 0.9, Ask: "「如果测试通过」是带前提的动作。请先完成前提。"}},
		{"multi_action", contract.Intent{Intent: contract.IntentAsk, Conflict: contract.ConflictMultiAction,
			Confidence: 0.9, Ask: "这句里有两件以上的事（查、提交）。请先说先做哪个。"}},
		// 类别 ③：原白名单**漏收**的 debug_plan（真实漏洞）。
		{"debug_plan_originally_missed", contract.Intent{Intent: contract.IntentAsk,
			Conflict: contract.ConflictDebugPlan, Confidence: 0.5,
			Ask: "你是想让我给修 bug 的思路，还是直接动手修？"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := o.llmIntentFallback(ctx, tc.it, questionLike)
			if got.Intent != tc.it.Intent || got.Ask != tc.it.Ask || got.Conflict != tc.it.Conflict {
				t.Errorf("仲裁 Ask 被回退覆写: intent=%s ask=%q conflict=%q（应原样保留）",
					got.Intent, got.Ask, got.Conflict)
			}
		})
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Errorf("仲裁 Ask 豁免失效: 期望 0 次 LLM 调用，实际 %d 次", n)
	}
}

// TestIntentFallbackStillReachableWhenAskEmpty 锁住类别 ②：
// Conflict 非空但 Ask 为空的**合法可执行路径**不得被结构性条件误伤 —— 回退仍要发生。
func TestIntentFallbackStillReachableWhenAskEmpty(t *testing.T) {
	o, calls := fallbackFixture(t)
	ctx := context.Background()
	const questionLike = "这样行吗？"

	cases := []struct {
		name string
		it   contract.Intent
	}{
		// 删除仲裁：Intent=EDIT + Ask 空，由下游域/风险门禁管，不该在这里被挡。
		{"delete", contract.Intent{Intent: contract.IntentEdit, Conflict: contract.ConflictDelete,
			Confidence: 0.9, Ask: ""}},
		{"note_vs_deploy", contract.Intent{Intent: contract.IntentNote, Conflict: contract.ConflictNoteVsDeploy,
			Confidence: 0.9, Ask: ""}},
		{"ask_vs_op", contract.Intent{Intent: contract.IntentAsk, Conflict: contract.ConflictAskVsOp,
			Confidence: 0.9, Ask: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := atomic.LoadInt32(calls)
			got := o.llmIntentFallback(ctx, tc.it, questionLike)
			if atomic.LoadInt32(calls) == before {
				t.Fatalf("Ask 为空的合法路径被错误挡在回退之外: %s 未触发 LLM", tc.name)
			}
			if got.Intent != contract.IntentQuery {
				t.Errorf("%s 回退应覆写意图为 QUERY, got %s", tc.name, got.Intent)
			}
		})
	}
}

// TestIntentFallbackRescuesDefaultUnknown 锁住类别 ④（本轮事故的直接回归）：
// 分类器对"嗯那个呃？"的真实输出是 UNKNOWN + **非空 Ask**（默认回问模板）。
// 它是"默认分类 Ask"而不是"仲裁 Ask"（Conflict 为空），因此**不得**被豁免 ——
// 回退必须被调用并把意图救回来。这条断言就是防止 `Ask != ""` 那版复辟。
func TestIntentFallbackRescuesDefaultUnknown(t *testing.T) {
	o, calls := fallbackFixture(t)
	ctx := context.Background()

	text := "嗯那个呃？"
	it := input.NewTaskClassifier(0.6, nil).ClassifyTask(text)
	if it.Intent != contract.IntentUnknown {
		t.Fatalf("前置条件不成立: %q 应判 UNKNOWN, got %s", text, it.Intent)
	}
	if it.Ask == "" {
		t.Fatalf("前置条件不成立: UNKNOWN 应带默认回问 Ask（这正是本轮陷阱所在）")
	}
	if it.Conflict != "" {
		t.Fatalf("前置条件不成立: 默认 UNKNOWN 的 Conflict 应为空, got %q", it.Conflict)
	}

	got := o.llmIntentFallback(ctx, it, text)
	if atomic.LoadInt32(calls) == 0 {
		t.Fatal("默认 UNKNOWN 回退未被调用 —— M7 ① 静默失效（`Ask != \"\"` 版复辟）")
	}
	if got.Intent != contract.IntentQuery {
		t.Errorf("默认 UNKNOWN 未被救回: intent=%s（期望 QUERY）", got.Intent)
	}
	if got.Ask != "" {
		t.Errorf("回退覆写后应清掉旧 UNKNOWN 澄清残留, got ask=%q", got.Ask)
	}
}
