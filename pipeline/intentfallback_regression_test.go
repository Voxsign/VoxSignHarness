// basefileis builder  back protect,   at stubsmith   data ;  data see e2e/arch_test.go. 
//
// overwriteto : pipeline/pipeline.go   llmIntentFallback     . base pipe from
//  name  switch(negation/meta/conditional/multi_action)modifybecomeclose ity  : 
//
//	Conflict != "" && Ask != ""   ->   alreadysendoccurand   Ask confirmstate, under    write. 
//
// as    hasbasefile(   §9"nokindbasethen  "): 
//
//	  path before**   overwrite**. `go test ./...` safety , but      ity  : 
//	if  onlywrite `Ask != ""`, becauseas ClassifyTask initstartizei.e. 
//	`Ask: taskAskTemplate`(UNKNOWN  however Ask  empty), back  **  triggersend** --
//	M7 ①"rule  UNKNOWN/low-confidenceand  sent -> call fast patchclassify"    . 
//
// because   by** class**  torevexample(   §3  needrequire 4:     " recv recv"and  
// "  recvbutrecv"): 
//
//	①          --    Ask( orig name  recv  debug_plan)  be write; 
//	②  back      -- Conflict  emptybut Ask asempty   path(delete etc)  back ; 
//	③ origbase recv   -- debug_plan isalreadystore      , new    patchon  ; 
//	④ default UNKNOWN -- Ask  emptybut Conflict asempty, back    becalluseand backintent. 
//
// onlydisconnectlang"     "   "    alsobe  "-- posisbase  thus  state. 
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

// intentJSONQuery is fast provider   returnback   classify JSON(note :  as JSON char  
// in  in require  body, thus idneed  ). 
const intentJSONQuery = `{\"intent\":\"QUERY\",\"confidence\":0.9}`

// fallbackFixture     returnback  intent JSON   fast provider, andreturnbackcalluse num . 
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

// TestIntentFallbackExemptsArbitrationAsks   classdiff ①/③: 
//   "Conflict  emptyand Ask  empty"   all  origkindreturnback, and**  **sendraise LLM calluse. 
func TestIntentFallbackExemptsArbitrationAsks(t *testing.T) {
	o, calls := fallbackFixture(t)
	ctx := context.Background()
	//      sent    text,  keep" e.g. be  , back    triggersend" --
	//  then    becauseas hasQ=false but  . 
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
		// classdiff ③: orig name ** recv**  debug_plan(    ). 
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

// TestIntentFallbackStillReachableWhenAskEmpty   classdiff ②:
// Conflict  emptybut Ask asempty **     path**  beclose ity     -- back  needsendoccur.
//
// 2026-10-08 (distillation R2): the gate was rewritten. Ask=="" no longer blocks the
// LLM fallback for UNKNOWN/low-confidence inputs (that is the regression this test pins);
// at the same time confident rule hits (>=0.6) are kept without a wasted LLM call, so a
// high-confidence EDIT/NOTE/ASK with an empty Ask stays rule-handled.
func TestIntentFallbackStillReachableWhenAskEmpty(t *testing.T) {
	o, calls := fallbackFixture(t)
	ctx := context.Background()
	const questionLike = "这样行吗？"

	t.Run("unknown_ask_empty_still_falls_back", func(t *testing.T) {
		before := atomic.LoadInt32(calls)
		got := o.llmIntentFallback(ctx, contract.Intent{
			Intent: contract.IntentUnknown, Confidence: 0.3, Ask: "",
		}, questionLike)
		if atomic.LoadInt32(calls) == before {
			t.Fatalf("Ask 为空不应阻断 LLM 回退: UNKNOWN 未触发 LLM（蒸馏 R2 门禁回归）")
		}
		if got.Intent == contract.IntentUnknown {
			t.Errorf("回退后意图仍为 UNKNOWN, 应被覆写为 QUERY")
		}
	})

	t.Run("confident_rule_kept_no_wasted_call", func(t *testing.T) {
		cases := []struct {
			name string
			it   contract.Intent
		}{
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
				if atomic.LoadInt32(calls) != before {
					t.Fatalf("高置信规则命中不应浪费 LLM 调用（蒸馏 R2 门禁）: %s", tc.name)
				}
				if got.Intent != tc.it.Intent {
					t.Errorf("%s 规则意图不应被覆写, got %s", tc.name, got.Intent)
				}
			})
		}
	})
}

// TestIntentFallbackRescuesDefaultUnknown   classdiff ④(base  thus  connectback ): 
// classify to"     "    outis UNKNOWN + ** empty Ask**(defaultclarification  ). 
//  is"defaultclassify Ask"but is"   Ask"(Conflict asempty), because **  **be   --
// back   becalluseandpipeintent back .   disconnectlangthenispreventstop `Ask != ""`     . 
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
