// Package selfheal 实现 VoxSign「三环异常自愈」层（docs/异常自愈架构-问题定位模型.md /
// docs/异常处理接口标准-v1.md）：
//
//	① 异常知识库命中（0 模型调用）：错误指纹 → 已有根因/修复，直接复用建议；
//	② 问题定位模型（JEV，模型中心 OpenAI 兼容通道）：未命中才调，输出结构化 JSON 诊断；
//	③ 自愈执行器：按 action 执行，限 2 轮，只对只读工具族自动重放，修复成功回写知识库。
//
// 设计硬约束（任何路径都不得违反）：
//   - 诊断层【可选】：diag provider 未配置 / 无 key / 4xx / 5xx / 超时 / JSON 解析失败 →
//     返回 nil（跳过），绝不向主链扩散错误，现有降级路径逐字不变；
//   - 永不自动执行不可逆动作（git commit / deploy / note append / 文件写）；
//   - 动作五词白名单：retry|modify|fallback|ask|stop；
//   - API key 绝不进代码 / 日志 / commit / 错误信息。
//
// 本包零外部依赖（仅标准库 + 内部 config/contract/provider）。
package selfheal

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SystemPrompt 是发给问题定位模型的 system 文本（接口标准 v1 §2，逐字）。
const SystemPrompt = "你是 VoxSign 异常诊断模型。根据用户任务与执行失败轨迹，输出结构化 JSON 诊断。只输出 JSON，不要多余文字。"

// 异常分类枚举（接口标准 v1 §3，budget 居首）。
const (
	CatBudget      = "budget"       // 预算/额度用尽 → 友好降级，不误导为网络
	CatNetwork     = "network"      // 网络不可达/超时 → 指数退避重试
	CatAuth        = "auth"         // key 无效/权限 → stop，提示检查 key
	CatParam       = "param"        // 参数不被支持（如 max_tokens 400）→ modify 修正参数
	CatToolMissing = "tool_missing" // 工具/能力缺失 → ask/fallback
	CatPermission  = "permission"   // 执行被拒/无权限 → stop/ask
	CatTransient   = "transient"    // 瞬时抖动 → retry
	CatUnknown     = "unknown"      // 无法判断 → fallback 保持现有降级
)

// 自愈动作五词白名单。
const (
	ActionRetry    = "retry"    // 按建议重试（指数退避，限 2 轮）
	ActionModify   = "modify"   // 按 retry_params 修正参数/换模型后重试一次
	ActionFallback = "fallback" // 安全降级（如 QUERY 降级文案）
	ActionAsk      = "ask"      // 转回问用户
	ActionStop     = "stop"     // 终止并归因
)

// validCategory / validAction 做白名单校验（模型可能输出越界值，一律归一为 unknown/stop）。
var validCategory = map[string]bool{
	CatBudget: true, CatNetwork: true, CatAuth: true, CatParam: true,
	CatToolMissing: true, CatPermission: true, CatTransient: true, CatUnknown: true,
}

var validAction = map[string]bool{
	ActionRetry: true, ActionModify: true, ActionFallback: true, ActionAsk: true, ActionStop: true,
}

// Trace 是 v2 定稿的失败轨迹（user content 的 traces 数组元素，接口标准 v2 §2）。
// 替代 v1 的 {tool,args,err,stdout,model}：
//   - params：原 args 原样 map（不再压成字符串摘要）；
//   - error：{code,type}——code 优先取可解析的 HTTP 状态码，否则稳定占位；type 为错误类别；
//   - stdout：v2 无此字段，省略（如需可并入 params）；
//   - model：模型类失败（fast/jev 调用）填被调模型名，工具类失败留空。
type Trace struct {
	Tool      string         `json:"tool"`
	Params    map[string]any `json:"params,omitempty"`
	Error     *TraceError    `json:"error,omitempty"`
	Model     string         `json:"model,omitempty"`
	Raw       string         `json:"-"` // 原始错误文本，仅本地指纹用，不上送模型
	RequestID string         `json:"-"` // P0-4a：本地关联 request_id（日志/轨迹贯通用，不上送模型）
}

// TraceError 是 v2 轨迹里的错误结构。
type TraceError struct {
	Code string `json:"code"` // HTTP 状态码（如 "503"/"429"）；不可解析则 "ERR_UNKNOWN"
	Type string `json:"type"` // 错误类别（timeout/rate_limit/auth/not_found/overload…）
}

// NewTrace 从原始错误文本构造一条 v2 轨迹（自动归类 error.code/type）。
func NewTrace(tool, model string, params map[string]any, errText string) Trace {
	code, typ := classifyError(errText)
	return Trace{Tool: tool, Params: params, Model: model, Error: &TraceError{Code: code, Type: typ}, Raw: errText}
}

// classifyError 把 openaiClient/执行器错误文本归类为 v2 error{code,type}。
// openaiClient 错误形态为 "HTTP 404: …" / "HTTP 500: …"，优先取三位状态码。
func classifyError(s string) (code, typ string) {
	code = "ERR_UNKNOWN"
	typ = "unknown"
	if i := strings.Index(s, "HTTP "); i >= 0 {
		rest := s[i+len("HTTP "):]
		digits := ""
		for _, ch := range rest {
			if ch >= '0' && ch <= '9' {
				digits += string(ch)
			} else {
				break
			}
		}
		if len(digits) == 3 {
			code = digits
			switch digits {
			case "400", "422":
				typ = "invalid_request"
			case "401", "403":
				typ = "auth"
			case "404":
				typ = "not_found"
			case "429":
				typ = "rate_limit"
			default:
				if digits[0] == '5' {
					typ = "overload"
				}
			}
			return
		}
	}
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline"):
		code, typ = "ERR_TIMEOUT", "timeout"
	case strings.Contains(lower, "connection") || strings.Contains(lower, "dial") || strings.Contains(lower, "no such host"):
		code, typ = "ERR_NETWORK", "connection"
	}
	return
}

// Diagnosis 是一次诊断结论（知识库命中或模型输出归一化后的形状）。
type Diagnosis struct {
	Category    string         `json:"category"`
	RootCause   string         `json:"root_cause"`
	Confidence  float64        `json:"confidence"`
	Recoverable bool           `json:"recoverable"`
	Suggestion  string         `json:"suggestion"`
	Action      string         `json:"action"`
	RetryParams map[string]any `json:"retry_params,omitempty"`
	Source      string         `json:"source"` // "kb" | "model"
	Fingerprint string         `json:"fingerprint"`
}

// normalizeAction 落实判定规则：recoverable=false 时任何 action 都转 stop；
// action 越界归一为 stop；category 越界归一为 unknown。
func (d *Diagnosis) normalizeAction() {
	if !validCategory[d.Category] {
		d.Category = CatUnknown
	}
	if !validAction[d.Action] {
		d.Action = ActionStop
	}
	if !d.Recoverable {
		d.Action = ActionStop
	}
}

// friendlySuggestion 按分类补友好文案（budget 说「预算用尽」而非笼统网络；
// param 提示按 retry_params 修正参数）。无诊断时主链保持原文案，不受影响。
func (d *Diagnosis) friendlySuggestion() {
	switch d.Category {
	case CatBudget:
		if !strings.Contains(d.Suggestion, "预算") {
			d.Suggestion = "模型预算/额度已用尽：" + d.Suggestion
		}
	case CatParam:
		if len(d.RetryParams) > 0 && !strings.Contains(d.Suggestion, "参数") {
			d.Suggestion = "按修正参数重试：" + d.Suggestion
		}
	}
}

// errFirstSegment 取错误首段：首行去空白；超过 80 字符截断到 80。
// （指纹输入的稳定性来源：同一类错误的措辞抖动不影响命中。）
func errFirstSegment(s string) string {
	line := strings.TrimSpace(s)
	if i := strings.IndexAny(line, "\n\r"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if len(line) > 80 {
		line = line[:80]
	}
	return line
}

// Fingerprint 计算错误指纹 = sha256(intent + "|" + tool + "|" + err首段)。
// 知识库按指纹命中，命中即 0 模型调用复用结论。
func Fingerprint(intent, tool, errText string) string {
	seg := errFirstSegment(errText)
	sum := sha256.Sum256([]byte(intent + "|" + tool + "|" + seg))
	return fmt.Sprintf("%x", sum)
}

// diagnoseRequest 是发给问题定位模型的 user 负载（v2：traces 结构化数组）。
type diagnoseRequest struct {
	Task   string  `json:"task"`
	Intent string  `json:"intent"`
	Traces []Trace `json:"traces"`
}

// encodeUser 把诊断输入编码为 user content JSON。
func encodeUser(task, intent string, traces []Trace) (string, error) {
	if len(traces) == 0 {
		traces = []Trace{{}}
	}
	b, err := json.Marshal(diagnoseRequest{Task: task, Intent: intent, Traces: traces})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// retry_params 读取助手（v2 字段名：backoff_seconds / max_retries / cooldown_seconds）。

// backoffSeconds 取 retry_params.backoff_seconds（指数退避基数，秒）；缺省返回 0（用内置默认）。
func (d *Diagnosis) backoffSeconds() float64 {
	if d == nil || d.RetryParams == nil {
		return 0
	}
	if v, ok := toFloat(d.RetryParams["backoff_seconds"]); ok && v > 0 {
		return v
	}
	return 0
}

// cooldownSeconds 取 retry_params.cooldown_seconds（重试前一次性冷却，秒）。
func (d *Diagnosis) cooldownSeconds() float64 {
	if d == nil || d.RetryParams == nil {
		return 0
	}
	if v, ok := toFloat(d.RetryParams["cooldown_seconds"]); ok && v > 0 {
		return v
	}
	return 0
}

// effectiveMaxRetries 取 retry_params.max_retries，但硬性不超过 MaxAutoRetries（2 轮）。
func (d *Diagnosis) effectiveMaxRetries() int {
	limit := MaxAutoRetries
	if d != nil && d.RetryParams != nil {
		if v, ok := toFloat(d.RetryParams["max_retries"]); ok {
			m := int(v)
			if m >= 0 && m < limit {
				limit = m
			}
		}
	}
	return limit
}

// toFloat 把 JSON 数字（float64）/整数断言为 float64。
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

// MaxRetries 返回本次诊断允许的重试轮次上限（retry_params.max_retries，硬性不超过 2）。
func (d *Diagnosis) MaxRetries() int { return d.effectiveMaxRetries() }

// Wait 返回第 round 轮重试前等待时长（v2 retry_params.backoff_seconds 指数 + cooldown）。
func (d *Diagnosis) Wait(round int) time.Duration { return waitForRound(d, round) }
