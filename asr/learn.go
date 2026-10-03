// learn.go —— 学习调用（`learn`）的**本地触发器**：从真实使用记录里判断"这里值得学"。
//
// 对应 ASR-MODEL-02 / `LEARN-01`：系统**自己**识别值得学习的片段，而不是等人告诉它。
// 本文件**零模型**：只用本地规则 + 已有能力（引擎/词典是否已覆盖这次纠正）做判断，
// 因此可以在 model 中心协议与底层模型确定之前先落地。
//
// 边界（与提案一致）：
//   - DetectLearnCandidates 是**纯函数**：只读输入、不写任何文件（R5）；
//   - 它**不得读取** UsageEvent.LearnLabel（那是语料里的"期望"，不是输入）——R4 由测试保证；
//   - 触发单位：一个 knowledge_key 一条候选（同 key 事件合并，证据条数有界）；
//   - 防自训练回路：kind == learn_derived 的事件**永不**触发（E5）。
//
// 它只**产出候选**；真正的"调用模型抽取规则 + 写回"属 LEARN-02 之后（等模型与协议）。
package asr

import "strings"

// 事件种类（触发器认识的白名单）。
const (
	eventUserCorrection = "user_correction"
	eventDictMiss       = "dict_miss"
	eventRuleOverride   = "rule_override"
	eventDiagnoseCause  = "diagnose_cause"
	eventLearnDerived   = "learn_derived" // 禁止再触发（防回路）
)

// learnMaxBatch 是同一 knowledge_key 最多保留的**证据条数**（候选仍有界）。
// 事件可以来很多条，但候选不会无限增长，证据也不会无界膨胀。
const learnMaxBatch = 8

// UsageEvent 是一次真实使用的记录（append-only 事件源 usage-events.jsonl）。
type UsageEvent struct {
	ID         string `json:"id"`
	At         string `json:"at,omitempty"`
	Kind       string `json:"kind"`
	Raw        string `json:"raw"`
	Final      string `json:"final,omitempty"`
	Confirmed  bool   `json:"confirmed"`
	Outcome    string `json:"outcome,omitempty"`
	TraceRef   string `json:"trace_ref,omitempty"`
	LearnLabel string `json:"learn_label,omitempty"` // **期望标签**：仅语料/测试使用，Detector 不得读取
	Provenance string `json:"provenance,omitempty"`
	Note       string `json:"note,omitempty"`
}

// EvidenceRef 是一条写回证据（L3：无来源不写回）。
type EvidenceRef struct {
	EventID  string `json:"event_id"`
	Kind     string `json:"kind,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
	TraceRef string `json:"trace_ref,omitempty"`
	At       string `json:"at,omitempty"`
}

// LearnCandidate 是一条"值得学"的候选。
type LearnCandidate struct {
	KnowledgeKey string        `json:"knowledge_key"`
	Kind         string        `json:"kind"`    // dictionary | pattern
	Trigger      string        `json:"trigger"` // 命中的触发规则名
	Reason       string        `json:"reason"`
	Evidence     []EvidenceRef `json:"evidence"`
}

// DetectLearnCandidates 扫描使用记录，返回"值得发起一次 learn 调用"的候选。
//
// P1 状态：**接口 + 类型先于实现**（判据 LEARN-01 先红）。
// 实现见 learn_detect.go；本函数只做参数校验后委派。
func DetectLearnCandidates(engine *Personalized, dict *Dictionary, events []UsageEvent) []LearnCandidate {
	return detectLearnCandidates(engine, dict, events)
}

// knowledgeKey 是候选的唯一键：同一处纠正（同 raw→final）永远归一到同一个键，
// 于是重复事件只合并证据，不会重复发起学习。
func knowledgeKey(raw, final string) string {
	return raw + "=>" + final
}

// learnableKind 报告事件种类是否属于"可能值得学"的类别（排除噪声/导航/自产物）。
func learnableKind(kind string) bool {
	switch kind {
	case eventUserCorrection, eventDictMiss, eventRuleOverride, eventDiagnoseCause:
		return true
	default:
		return false
	}
}

// normalizeText 去掉首尾空白（事件文本经常带空格）。
func normalizeText(s string) string { return strings.TrimSpace(s) }
