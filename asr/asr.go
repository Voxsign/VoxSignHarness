// Package asr 是 voice-sign harness 的 **ASR 个性化识别层**。
//
// 定位（Peter 2026-10-03 三次澄清后定稿）：
//   - voice-sign harness 是**主体**；
//   - 本包是 harness 的**内层能力**，代码属于 harness，可单独部署；
//   - 依赖方向**单向**：harness → asr，本包**不 import** harness 任何其它包。
//
// 当前阶段：**P1（判据先于实现）**。本文件只定义**接口与类型**，
// 行为由后续阶段实现；桩见 harness_test.go（现在应当是红的）。
//
// 设计文档：docs/vhs-asr-设计-v2.md
package asr

import "time"

// Engine 是 ASR 个性化识别的唯一入口。
//
// 设计要点：**Correct 是无状态纯函数** —— 好测、好并发、好回放、好跨进程。
// 个性化状态由 Observe 更新、由 Lexicon 可审计。**状态与计算分离。**
type Engine interface {
	// Correct 纠正一条 ASR 原始文本。无副作用。
	Correct(req CorrectRequest) CorrectResult
	// Observe 回传一次反馈（用户接受/改回），用于在线学习。可有副作用。
	Observe(fb Feedback) error
	// Lexicon 导出当前个性化状态，供审计。
	Lexicon(domain string) Lexicon
}

// CorrectRequest 是一次纠正请求。
type CorrectRequest struct {
	Raw     string   // ASR 原始输出。**必须留底**，不得因为"要好看"而丢弃
	Context []string // 近期上下文（同一段对话的前几句），影响候选排序
	Domain  string   // 当前域，影响热词优先级
}

// CorrectResult 是一次纠正结果。
type CorrectResult struct {
	Text        string        // 纠正后文本
	Corrections []Correction  // 每一处纠正都带证据（可为空 = 一处未改）
	Candidates  []Candidate   // 置信度接近时的候选（不为空表示"该问人/该问外部"）
	Latency     time.Duration // 本次纠正耗时
}

// Correction 是一处具体纠正 —— **必须可回溯到原文区间**。
type Correction struct {
	Start      int     // 在 Raw 中的起始**字节**下标
	End        int     // 结束**字节**下标（左闭右开）
	From       string  // 原文片段
	To         string  // 纠正后片段
	Kind       string  // filler | homophone | hotword | dictionary | truncation | punctuation
	Confidence float64 // 0..1
	Evidence   string  // 为什么这么纠（命中了哪条规则/哪个词条），供归因
}

// Candidate 是一个备选纠正。
type Candidate struct {
	Text       string
	Confidence float64
	Reason     string
}

// Feedback 是一次反馈，用于在线学习。
type Feedback struct {
	Raw       string
	Corrected string
	Accepted  bool   // 用户是否接受
	Source    string // user_edit | implicit | reviewer
}

// Lexicon 是个性化状态的可审计快照。
type Lexicon struct {
	Domain   string
	Hotwords []Hotword
	Version  string
}

// Hotword 是一条热词/个人词条。
type Hotword struct {
	Term    string
	Kind    string // person | project | term | command
	Weight  float64
	SeenCnt int
}

// ---------------------------------------------------------------------------
// 基线实现：Passthrough（原样返回，零纠正）
//
// 它同时是两样东西：
//  1. **合法的零纠正实现**（不是占位符）—— 一个诚实的下界；
//  2. **自比对的"改动前"那条臂** —— 桩跑在它上面，红的地方就是真缺口。
//
// 判据 C1（不该纠的不能纠）在它身上应当**天然通过**——
// 这正是双向反例里"不该动的没动"那一半。
// ---------------------------------------------------------------------------

// Passthrough 不做任何纠正。
type Passthrough struct{}

// Correct 原样返回，不产生任何 Correction。
func (Passthrough) Correct(req CorrectRequest) CorrectResult {
	return CorrectResult{Text: req.Raw}
}

// Observe 基线不学习。
func (Passthrough) Observe(Feedback) error { return nil }

// Lexicon 基线没有个性化状态。
func (Passthrough) Lexicon(string) Lexicon { return Lexicon{} }

// 编译期确认基线满足接口。
var _ Engine = Passthrough{}
