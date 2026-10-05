package selfheal

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"voicesign-harness/contract"
	"voicesign-harness/provider"
)

// MaxAutoRetries 是同一失败指纹的自动重试轮次上限（防死循环/越权）。
const MaxAutoRetries = 2

// retryBackoff 是指数退避序列（接口标准 v1：network/transient 类指数退避）。
// 第 1 轮重试前等 500ms，第 2 轮等 1s；超过 MaxAutoRetries 即停止。
var retryBackoff = []time.Duration{500 * time.Millisecond, 1 * time.Second}

// ToolRunner 重放一个只读工具动作（由 pipeline 注入 o.run）。诊断层只在判定安全时调用它。
type ToolRunner func(tool string, args map[string]any) contract.Receipt

// Attempt 是一次失败的工具执行（诊断 + 安全重放的输入，带原始 args 以便重放）。
type Attempt struct {
	Tool      string
	Args      map[string]any
	Receipt   contract.Receipt
	RequestID string // P0-4b 贯通：主链 request_id，带入诊断 trace（日志/轨迹贯通用）
}

// Service 是三环自愈层的运行时实例：知识库 + 可选问题定位模型 + （可选）工具重放器。
// 全部方法对"诊断不可用/失败"一律返回 nil，绝不向主链扩散错误。
type Service struct {
	KB     *KB               // 异常知识库（① 环，0 模型调用）
	Diag   provider.Provider // 问题定位模型（② 环）；nil = 未配置，跳过模型诊断
	Runner ToolRunner        // 只读工具重放器；nil = 只诊断不重放

	counters map[string]int // fingerprint → 已用自动重试轮次
	last     *Diagnosis     // 最近一次诊断结论（供归因替换静态建议）
}

// NewService 装配自愈层。kb 必传；diag/runner 可 nil（nil = 对应环跳过）。
func NewService(kb *KB, diag provider.Provider, runner ToolRunner) *Service {
	if kb == nil {
		kb = OpenKB("") // 空库兜底，绝不 nil 解引用
	}
	return &Service{KB: kb, Diag: diag, Runner: runner, counters: map[string]int{}}
}

// LastDiagnosis 返回最近一次诊断结论（供归因）；无则 nil。
func (s *Service) LastDiagnosis() *Diagnosis { return s.last }

// ResetLast 清空最近诊断（每次 Run 开头调用，避免跨任务串味）。
func (s *Service) ResetLast() { s.last = nil }

// RetryCount 返回某指纹已用的自动重试轮次（测试断言上限用）。
func (s *Service) RetryCount(fp string) int { return s.counters[fp] }

// BackoffAfter 返回第 round 轮重试前的退避时长（指数退避，限 MaxAutoRetries 轮）。
// 供 LLM 失败重试路径复用同一套退避节奏。
func BackoffAfter(round int) time.Duration {
	if round < 0 {
		round = 0
	}
	if round >= len(retryBackoff) {
		round = len(retryBackoff) - 1
	}
	return retryBackoff[round]
}

// Diagnose 跑 ①→② 环：先查知识库（命中即复用，0 模型调用）；未命中且 diag 可用才调模型。
// 任何失败（无 failures / 模型未配置 / Chat err / 超时 / JSON 解析失败）→ 返回 nil 跳过。
func (s *Service) Diagnose(ctx context.Context, task, intent string, traces []Trace) *Diagnosis {
	if len(traces) == 0 {
		return nil
	}
	primary := traces[0]
	fp := Fingerprint(intent, primary.Tool, primary.Raw)

	// ① 知识库命中。
	if d, ok := s.KB.Lookup(fp); ok {
		d.Fingerprint = fp
		s.last = &d
		return &d
	}

	// ② 模型未配置 → 跳过（零开销）。
	if s == nil || s.Diag == nil {
		return nil
	}

	user, err := encodeUser(task, intent, traces)
	if err != nil {
		return nil
	}
	resp, err := s.Diag.Chat(ctx, provider.ChatRequest{
		Messages: []contract.Message{
			{Role: "system", Content: SystemPrompt},
			{Role: "user", Content: user},
		},
		MaxTokens: 500,
	})
	if err != nil {
		// 404/5xx/超时/网络：薄降级。错误信息只含状态码+截断响应体（传输层已保证不含 key）。
		log.Printf("[selfheal] rid=%s diag Chat 失败，跳过诊断（不阻断主链）: %v", primary.RequestID, err)
		return nil
	}

	d, err := parseDiagnosis(resp.Content)
	if err != nil {
		log.Printf("[selfheal] rid=%s diag JSON 解析失败，跳过诊断: %v", primary.RequestID, err)
		return nil
	}
	d.Source = "model"
	d.Fingerprint = fp
	d.normalizeAction()
	d.friendlySuggestion()
	s.last = &d
	return &d
}

// parseDiagnosis 解析模型 JSON 输出（容忍首尾空白）。缺关键字段/非法 JSON → error。
func parseDiagnosis(content string) (Diagnosis, error) {
	var d Diagnosis
	if err := json.Unmarshal([]byte(content), &d); err != nil {
		return Diagnosis{}, err
	}
	if d.Category == "" || d.Action == "" {
		return Diagnosis{}, errMissingFields
	}
	return d, nil
}

// errMissingFields 是诊断 JSON 缺必填字段的哨兵错误。
var errMissingFields = &diagError{"诊断 JSON 缺 category/action 字段"}

type diagError struct{ s string }

func (e *diagError) Error() string { return e.s }

// SafeRetry 跑 ③ 环：对一次失败工具执行做诊断 + 安全重放。
// 返回 (newReceipt, diagnosis)：
//   - newReceipt 非空 = 安全重放成功（已回写知识库），由主链合并进 Receipts；
//   - diagnosis 始终返回（含 KB 命中）供归因；不自动重放/诊断失败时为 nil。
//
// 安全不变量：写类/不可逆工具、recoverable=false、超过 2 轮上限、无 Runner → 绝不重放。
func (s *Service) SafeRetry(ctx context.Context, task, intent string, att Attempt) (*contract.Receipt, *Diagnosis) {
	if s == nil {
		return nil, nil
	}
	errText := att.Receipt.Err
	if errText == "" {
		errText = att.Receipt.Stderr
	}
	tr := NewTrace(att.Tool, "", att.Args, errText)
	tr.RequestID = att.RequestID // P0-4b：诊断 trace 带主链 request_id（日志/轨迹贯通）
	d := s.Diagnose(ctx, task, intent, []Trace{tr})
	if d == nil {
		return nil, nil
	}

	switch d.Action {
	case ActionRetry, ActionModify:
		// 只读族才允许自动重放；否则结论只进归因。
		if !IsReadOnlyTool(att.Tool, att.Args) {
			return nil, d
		}
		if s.Runner == nil {
			return nil, d
		}
		fp := d.Fingerprint
		// 轮次上限：硬性 2 轮；retry_params.max_retries 可收紧但不突破上限。
		if s.counters[fp] >= d.effectiveMaxRetries() {
			return nil, d
		}
		// 退避：优先 retry_params.backoff_seconds（指数 base*2^round）+ cooldown_seconds；
		// 缺省用内置默认退避（500ms/1s）。尊重 ctx 取消。
		wait := waitForRound(d, s.counters[fp])
		select {
		case <-ctx.Done():
			return nil, d
		case <-time.After(wait):
		}
		args := att.Args
		if d.Action == ActionModify {
			args = applyRetryParams(att.Args, d.RetryParams)
		}
		s.counters[fp]++
		nr := s.Runner(att.Tool, args)
		if nr.OK {
			// 修复成功 → 学习回写知识库。
			s.KB.Remember(*d)
			return &nr, d
		}
		// 重放失败：不回写 KB（避免把未验证结论固化）。
		return nil, d
	default:
		// fallback / ask / stop：不再执行，结论供归因。
		return nil, d
	}
}

// waitForRound 计算第 round 轮重试前等待时长（v2 retry_params）。
func waitForRound(d *Diagnosis, round int) time.Duration {
	var w time.Duration
	if base := d.backoffSeconds(); base > 0 {
		mult := 1 << round // 指数：第 round 轮 = base * 2^round
		w = time.Duration(base * float64(mult) * float64(time.Second))
	} else {
		if round >= len(retryBackoff) {
			round = len(retryBackoff) - 1
		}
		w = retryBackoff[round]
	}
	if cd := d.cooldownSeconds(); cd > 0 {
		w += time.Duration(cd * float64(time.Second))
	}
	return w
}

// retryControlKeys 是 v2 retry_params 里的 harness 控制键，不是工具参数，modify 时不得注入工具 args。
var retryControlKeys = map[string]bool{
	"backoff_seconds": true, "max_retries": true, "cooldown_seconds": true,
}

// applyRetryParams 把 retry_params 里的标量修正合并进 args（modify 动作）。
// 只合入简单标量、且跳过 harness 控制键，防模型注入任意结构；未知 key 忽略。
func applyRetryParams(args map[string]any, params map[string]any) map[string]any {
	out := make(map[string]any, len(args)+len(params))
	for k, v := range args {
		out[k] = v
	}
	for k, v := range params {
		if retryControlKeys[k] {
			continue
		}
		switch v.(type) {
		case string, float64, bool:
			out[k] = v
		}
	}
	return out
}
