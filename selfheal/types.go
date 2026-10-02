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

// Failure 是一条失败轨迹（诊断模型 user 输入的 failure 数组元素）。
// Model 仅在「模型类失败」（fast/diag 调用 err/空/慢超时）时填被调模型名，工具类失败留空。
type Failure struct {
	Tool   string `json:"tool"`
	Args   string `json:"args,omitempty"`
	Err    string `json:"err"`
	Stdout string `json:"stdout,omitempty"`
	Model  string `json:"model,omitempty"`
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

// diagnoseRequest 是发给问题定位模型的 user 负载（JSON 序列化后作为 user content）。
type diagnoseRequest struct {
	Task    string    `json:"task"`
	Intent  string    `json:"intent"`
	Failure []Failure `json:"failure"`
}

// encodeUser 把诊断输入编码为 user content JSON。
func encodeUser(task, intent string, failures []Failure) (string, error) {
	if len(failures) == 0 {
		failures = []Failure{{}}
	}
	b, err := json.Marshal(diagnoseRequest{Task: task, Intent: intent, Failure: failures})
	if err != nil {
		return "", err
	}
	return string(b), nil
}
