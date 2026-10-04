// 本文件是 R10 真机复现（2026-10-03）的回归保护：
//
// 事故：语料 R10「我们要为国家电网南非公司建一个 OT 运营数据平台…帮我规划一下这件事」
// 分类器规则判 ORCHESTRATE(0.90)，但 pipeline 轨迹/回执最终是 NOTE(0.85)。
// 根因：llmIntentFallback 的 hasQ 用裸字符集 ContainsAny("?？吗呢怎么如何为什么哪")，
// 其中"为/么/什"是普通正文高频字——"为国家电网"里的"为"命中 → 触发 LLM 复查；
// 而 LLM 词表只有 NOTE|QUERY|EDIT|COMMIT，把高置信 ORCHESTRATE 覆盖成 NOTE。
//
// 修复（三层，底层标准化）：
//  1. questionTone 改为词级匹配（单字 ?？吗呢哪 + 完整词 怎么/如何/为什么）；
//  2. LLM 词表补 ORCHESTRATE（编排类长任务可被复查而非被压扁成 NOTE）；
//  3. 覆盖为不同意图类别时清掉旧 params（消灭 NOTE+kind=implement 混合态）。
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
	"voicesign-harness/provider"
)

// TestQuestionToneWordLevel 锁住问句判定的词级语义：普通正文高频字"为/么/什"
// 不再误判为问句；完整问句词仍命中。
func TestQuestionToneWordLevel(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		// R10 事故句：含"为"（为国家电网）但无问句语气 → 必须 false（否则触发 LLM 覆盖）。
		{"r10_orchestrate_narrative", "我们要为国家电网南非公司建一个 OT 运营数据平台，帮我规划一下这件事", false},
		{"prose_wei", "把 DMZ 单向发布给企业，统一收上来做受治理的数据产品", false},
		{"prose_me", "这么重要的范围不能自动放行", false},
		{"prose_shenme", "有什么收获都记到笔记里", false},
		{"no_question", "把方案整理成 markdown 文档放到 docs 目录", false},
		// 完整问句词必须仍命中。
		{"full_why", "为什么 OT-ODP 没有独立安全审批？", true},
		{"full_how", "怎么处理 S1 振荡场景", true},
		{"full_how2", "看看效果怎么样", true},
		{"full_how_to", "如何优化降级模式", true},
		{"particle_ma", "在吗", true},
		{"particle_ne", "那发布链路呢", true},
		{"particle_na", "哪一层是边界", true},
		{"question_mark", "这样可以？", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := questionTone(tc.text); got != tc.want {
				t.Errorf("questionTone(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// TestLLMFallbackNoOverwriteWithoutQuestionTone 锁住事故路径：无问句语气时，
// 高置信规则意图（ORCHESTRATE 0.90）必须原样返回，且**不得**发起 LLM 调用。
func TestLLMFallbackNoOverwriteWithoutQuestionTone(t *testing.T) {
	o, calls := fallbackFixture(t)
	ctx := context.Background()

	const r10 = "我们要为国家电网南非公司建一个 OT 运营数据平台，把 SCADA EMS WAMS 这些源系统的数据统一收上来做受治理的数据产品，通过 DMZ 单向发布给企业，AI 只给建议动作要人工批准，帮我规划一下这件事"
	it := contract.Intent{
		Intent:        contract.IntentOrchestrate,
		Confidence:    0.9,
		CorrectedText: r10,
		Params:        map[string]string{"kind": "implement"},
	}
	got := o.llmIntentFallback(ctx, it, r10)
	if got.Intent != contract.IntentOrchestrate {
		t.Errorf("无问句语气的 ORCHESTRATE 被覆写为 %s", got.Intent)
	}
	if got.Params["kind"] != "implement" {
		t.Errorf("params 被改动: %v", got.Params)
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Errorf("期望 0 次 LLM 调用，实际 %d 次（问句判定误报）", n)
	}
}

// TestLLMFallbackTrustsQuestionClassRules 锁住 20样例 全管线回归（17/20 → 20/20）：
// 无仲裁冲突的问句类规则结果（ASK 澄清态 / DEBUG 报错态）不得被 LLM 复查压平成 QUERY。
func TestLLMFallbackTrustsQuestionClassRules(t *testing.T) {
	o, calls := fallbackFixture(t)
	ctx := context.Background()

	cases := []struct {
		name string
		text string
		it   contract.Intent
	}{
		{"ask_which", "医疗耗材进口沙特的政策你觉得要注意哪些",
			contract.Intent{Intent: contract.IntentAsk, Confidence: 0.85}},
		{"ask_why", "为什么凌晨三点那个告警一直响",
			contract.Intent{Intent: contract.IntentAsk, Confidence: 0.85}},
		{"debug_error", "voxbuybot 下单页为什么报错啊",
			contract.Intent{Intent: contract.IntentDebug, Confidence: 0.85}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := o.llmIntentFallback(ctx, tc.it, tc.text)
			if got.Intent != tc.it.Intent {
				t.Errorf("问句类规则结果被覆写: %s -> %s（应信任规则）", tc.it.Intent, got.Intent)
			}
		})
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Errorf("问句类规则结果不应触发 LLM 复查，实际 %d 次调用", n)
	}
}

// orchestrateFixture 构造返回 ORCHESTRATE JSON 的 provider（验证词表新增项）。
func orchestrateFixture(t *testing.T) (*Options, *int32) {
	t.Helper()
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w,
			`{"choices":[{"message":{"content":"{\"intent\":\"ORCHESTRATE\",\"confidence\":0.9}"}}],"usage":{}}`)
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

// TestLLMFallbackOrchestrateInVocab 锁住词表新增：问句语气的长任务复查时可输出
// ORCHESTRATE（不再被压扁成 NOTE），且旧意图 params 被清空（无混合态残留）。
func TestLLMFallbackOrchestrateInVocab(t *testing.T) {
	o, calls := orchestrateFixture(t)
	ctx := context.Background()

	// 规则把"改成…可以吗？"判 EDIT(0.85)；问句语气（吗/？）触发复查。
	const qText = "把这个方案里 SPoG 的范围改成二级保护，可以吗？"
	it := contract.Intent{
		Intent:        contract.IntentEdit,
		Confidence:    0.85,
		CorrectedText: qText,
		Params:        map[string]string{"action": "modify", "object": "spog"},
	}
	got := o.llmIntentFallback(ctx, it, qText)
	if got.Intent != contract.IntentOrchestrate {
		t.Errorf("LLM 复查应输出 ORCHESTRATE，实际 %s", got.Intent)
	}
	if got.Params != nil {
		t.Errorf("覆盖为不同意图后应清空旧 params，实际 %v", got.Params)
	}
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Errorf("期望恰好 1 次 LLM 调用，实际 %d", n)
	}
}
