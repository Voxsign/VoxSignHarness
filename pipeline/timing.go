package pipeline

import (
	"encoding/json"
	"time"

	"voicesign-harness/provider"
	"voicesign-harness/trajectory"
)

// ---- Issue #30 Phase 0: per-task stage/LLM wall-clock instrumentation ----
// Additive only: nothing here changes existing event payloads or pipeline behavior.

// StageTiming records wall-clock time for one pipeline stage.
type StageTiming struct {
	Name string `json:"name"`
	Ms   int64  `json:"ms"`
}

// LLMCallTiming records wall-clock time and token usage for one LLM call.
// Provider name doubles as the model-tier label (fast/strong/center/...).
type LLMCallTiming struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	PromptTokens     int64  `json:"prompt_tokens,omitempty"`
	CompletionTokens int64  `json:"completion_tokens,omitempty"`
	FinishReason     string `json:"finish_reason,omitempty"`
	Err              string `json:"err,omitempty"` // non-empty => the call failed
	Ms               int64  `json:"ms"`
}

// TimingInfo is the additive per-task timing payload attached to Outcome.Timing
// and echoed into the done/failed SSE payloads and the trace JSONL.
type TimingInfo struct {
	Stages   []StageTiming   `json:"stages,omitempty"`
	LLMCalls []LLMCallTiming `json:"llm_calls,omitempty"`
	TotalMs  int64           `json:"total_ms"`
}

// beginStage starts a named stage timer.
func (o *Options) beginStage(name string) time.Time { return time.Now() }

// endStage records the elapsed wall-clock for the named stage.
func (o *Options) endStage(name string, start time.Time) {
	o.stageTimes = append(o.stageTimes, StageTiming{Name: name, Ms: time.Since(start).Milliseconds()})
}

// recordLLMCall is wired into provider.Registry.Wrapped so every Chat call
// (fallback or direct) is timed exactly once per task.
func (o *Options) recordLLMCall(name string, resp provider.ChatResponse, ms int64) {
	o.llmCalls = append(o.llmCalls, LLMCallTiming{
		Provider:         name,
		Model:            name,
		PromptTokens:     int64(resp.Usage.PromptTokens),
		CompletionTokens: int64(resp.Usage.CompletionTokens),
		FinishReason:     resp.FinishReason,
		Ms:               ms,
	})
}

// attachTiming assembles the collected stage/LLM timings into the Outcome and
// writes one additive "timing" trace event. Idempotent per call site.
func (o *Options) attachTiming(out *Outcome) {
	out.Timing = &TimingInfo{
		Stages:   o.stageTimes,
		LLMCalls: o.llmCalls,
		TotalMs:  out.LoopMs,
	}
	if tj, err := json.Marshal(out.Timing); err == nil {
		o.write(trajectory.Entry{RequestID: out.RequestID, Kind: trajectory.KindTiming, Content: string(tj)})
	}
}
