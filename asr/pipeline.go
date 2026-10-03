// pipeline.go —— 本地理解管线的编排层（需求 3.1 的 1→N 步）。
//
// 定位：把已有的纯函数（Engine.Correct）、独立结构（Dictionary、Tracer）**编排**起来，
// 并给每一步打点留痕。**它不修改 Correct 的语义**——Correct 仍是无状态纯函数，
// 轨迹/词典/热加载全部在它的外层，符合"状态与计算分离"。
//
// 步序（对齐需求 3.1）：留底 → 清洗（Correct）→ 词典 → 标点 → 输出。
// 上下文/指代/意图/域建议属第 2 批，本层暂不实现（保持 unverified）。
package asr

import "time"

// Step 是管线中一步的运行摘要（同时用于 HTTP 响应与轨迹）。
type Step struct {
	Step   string  `json:"step"`
	Ms     float64 `json:"ms"`
	Source string  `json:"source"`
	Detail string  `json:"detail,omitempty"`
}

// Pipeline 是本地理解管线的执行者。
type Pipeline struct {
	Engine *Personalized
	Dict   *Dictionary
	Tracer *Tracer
	// Hot 是可选的**缓存改写**钩子（热词/别名/近音）。接口定义在本包、实现由外层提供，
	// 以避免 asr → hotcache 的反向依赖（hotcache 复用 asr 的拼音表）。
	Hot TextRewriter
	now func() time.Time
}

// TextRewriter 用缓存改写文本（CACHE-001 K9）。实现方：recog.Rewriter。
type TextRewriter interface {
	Rewrite(text string) (string, []Correction)
}

// NewPipeline 组装管线。Engine 必填；Dict/Tracer 可为 nil（则不启用对应能力）。
func NewPipeline(engine *Personalized, dict *Dictionary, tracer *Tracer) *Pipeline {
	if engine == nil {
		engine = NewEngine()
	}
	return &Pipeline{Engine: engine, Dict: dict, Tracer: tracer, now: time.Now}
}

// ProcessResult 是一次管线运行的结果（HTTP 层再决定暴露哪些字段）。
type ProcessResult struct {
	Raw                    string
	Text                   string
	Punctuated             string
	Corrections            []Correction
	PunctuationCorrections []Correction
	Candidates             []Candidate
	Steps                  []Step
	RequestID              string
}

// Process 跑一遍本地管线，并逐步写入轨迹。
//
// 说明：轨迹里 retain 步会带上 input_raw（需求 4.1 的"原始留底"），
// 这使 traces-asr.jsonl 成为 append-only 的输入留底；原始文本永不因"要好看"而丢弃。
func (p *Pipeline) Process(raw, session string) ProcessResult {
	res := ProcessResult{Raw: raw}
	res.RequestID = p.Tracer.NextRequestID("req")

	emit := func(name, source, detail string, d time.Duration) {
		ms := float64(d.Microseconds()) / 1000.0
		res.Steps = append(res.Steps, Step{Step: name, Ms: ms, Source: source, Detail: detail})
		if p.Tracer != nil {
			_ = p.Tracer.Append(TraceRecord{
				RequestID: res.RequestID, SessionID: session,
				Step: name, Ms: ms, Source: source, Detail: detail,
			})
		}
	}

	// 1) 留底：input_raw 原样进入轨迹（append-only）。
	emit("retain", "store", "input_raw="+raw, 0)

	// 2) 热加载：JSON 词典变更即时生效（不依赖 fsnotify，零第三方依赖）。
	t := p.now()
	reloaded := false
	if p.Dict != nil {
		if changed, err := p.Dict.Reload(); err == nil && changed {
			reloaded = true
		}
	}
	hotMs := p.now().Sub(t)

	// 3) 清洗：正文纠错（Correct 纯函数）。
	t = p.now()
	cr := p.Engine.Correct(CorrectRequest{Raw: raw})
	emit("clean", "rule:filler+homophone+truncation", "正文纠错（Correct 纯函数）", p.now().Sub(t))
	res.Text = cr.Text
	res.Corrections = cr.Corrections
	res.Candidates = cr.Candidates

	// 4) 词典：精确 + 目标近音。
	t = p.now()
	detail := "词典纠错（JSON 快照）"
	if reloaded {
		detail += "；已热加载新词典"
	}
	if p.Dict != nil {
		text, corrs := p.Dict.Apply(res.Text)
		res.Text = text
		res.Corrections = append(res.Corrections, corrs...)
	}
	emit("dict", "dictionary", detail, p.now().Sub(t)+hotMs)

	// 4b) 缓存改写（热词/别名/近音，CACHE-001 K9）：**真的改变输出**才算接上。
	if p.Hot != nil {
		t = p.now()
		rewritten, corrs := p.Hot.Rewrite(res.Text)
		res.Text = rewritten
		res.Corrections = append(res.Corrections, corrs...)
		emit("hotcache", "cache:hotword+alias+edit", "按热词/别名/近音改写（K9）", p.now().Sub(t))
	}

	// 5) 标点恢复：只写 Punctuated（正文不动，C1 v2）。
	//    无内容噪声不恢复标点（与 Engine 的 pureNoise 守卫一致，C3）。
	t = p.now()
	punctDetail := "标点恢复（独立字段，不改正文）"
	if isNoiseResult(res.Candidates) {
		res.Punctuated = res.Text
		res.PunctuationCorrections = nil
		punctDetail = "纯噪声不恢复标点（C3 无内容守卫）"
	} else {
		res.Punctuated, res.PunctuationCorrections = punctuate(res.Text)
	}
	emit("punctuate", "rule:punctuate", punctDetail, p.now().Sub(t))

	return res
}

// isNoiseResult 报告引擎是否判定"整句无可用内容"（据此不恢复标点）。
func isNoiseResult(cands []Candidate) bool {
	for _, c := range cands {
		if len(c.Reason) >= len(askNoiseReasonPrefix) && c.Reason[:len(askNoiseReasonPrefix)] == askNoiseReasonPrefix {
			return true
		}
	}
	return false
}
