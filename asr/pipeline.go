// pipeline.go -- basely resolvemanageline orchestrate (needrequire 3.1   1->N  ). 
//
//   : pipealreadyhas   num(Engine.Correct),   close (Dictionary, Tracer)**orchestrate**raise , 
// andgive    pt  . **  modify Correct  semantic**--Correct  isnostatus  num, 
// trace/word /   safety    out ,   "statusand  split ". 
//
//   (to needrequire 3.1):  bot -> clean(Correct)-> word  -> tgtpt ->  out. 
// onunder /coreference/intent/domain     2 approve, this layer   now(keepkeep unverified). 
package asr

import (
	"strconv"
	"strings"
	"time"
)

// Step ismanagelinein      need(sametimeuseat HTTP   andtrace). 
type Step struct {
	Step   string        `json:"step"`
	Kind   TraceStepKind `json:"kind"` //  nameclasstype(EXEC-05 v2:  bychar    write out )
	Ms     float64       `json:"ms"`
	Source string        `json:"source"`
	Detail string        `json:"detail,omitempty"`
}

// Pipeline isbasely resolvemanageline   er. 
type Pipeline struct {
	Engine *Personalized
	Dict   *Dictionary
	Tracer *Tracer
	// Hot is   **cachemodifywrite**  ( word/diffname/ audio). connect define this package,  nowbyout  provide, 
	// by   asr -> hotcache  revtodependency(hotcache  use asr   audiotable). 
	Hot TextRewriter
	now func() time.Time
}

// TextRewriter usecachemodifywrite base(CACHE-001 K9).  now : recog.Rewriter. 
type TextRewriter interface {
	Rewrite(text string) (string, []Correction)
}

// NewPipeline   manageline. Engine   ; Dict/Tracer  as nil(then startuseto   ). 
func NewPipeline(engine *Personalized, dict *Dictionary, tracer *Tracer) *Pipeline {
	if engine == nil {
		engine = NewEngine()
	}
	return &Pipeline{Engine: engine, Dict: dict, Tracer: tracer, now: time.Now}
}

// ProcessResult is  manageline   close (HTTP  againdecide     charseg). 
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

// Process    baselymanageline, and  writetrace. 
//
//   : trace  retain    on input_raw(needrequire 4.1  "origstart bot"), 
//    traces-asr.jsonl becomeas append-only   in bot; origstart base  because"need  "but  . 
func (p *Pipeline) Process(raw, session string) ProcessResult {
	res := ProcessResult{Raw: raw}
	res.RequestID = p.Tracer.NextRequestID("req")

	emit := func(name, source, detail string, d time.Duration) {
		ms := float64(d.Microseconds()) / 1000.0
		res.Steps = append(res.Steps, Step{Step: name, Kind: kindForStep(name), Ms: ms, Source: source, Detail: detail})
		if p.Tracer != nil {
			_ = p.Tracer.Append(TraceRecord{
				RequestID: res.RequestID, SessionID: session,
				Step: name, Kind: kindForStep(name), Ms: ms, Source: source, Detail: detail,
			})
		}
	}

	// 1)  bot: input_raw origkind intrace(append-only). 
	emit("retain", "store", "input_raw="+raw, 0)

	// 2)    : JSON word changechangei.e.timeoccur ( dependency fsnotify,     dependency). 
	t := p.now()
	reloaded := false
	if p.Dict != nil {
		if changed, err := p.Dict.Reload(); err == nil && changed {
			reloaded = true
		}
	}
	hotMs := p.now().Sub(t)

	// 3) clean: pos correction(Correct   num). 
	t = p.now()
	cr := p.Engine.Correct(CorrectRequest{Raw: raw})
	emit("clean", "rule:filler+homophone+truncation", "正文纠错（Correct 纯函数）", p.now().Sub(t))
	res.Text = cr.Text
	res.Corrections = cr.Corrections
	res.Candidates = cr.Candidates

	// 4) word :    + objtgt audio. 
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

	// 4b) cachemodifywrite( word/diffname/ audio, CACHE-001 K9): **  modifychange out**only connecton. 
	if p.Hot != nil {
		t = p.now()
		rewritten, corrs := p.Hot.Rewrite(res.Text)
		res.Text = rewritten
		res.Corrections = append(res.Corrections, corrs...)
		// modifywrite  (§5.1   5  ): detail   ** bodymodify  word**(From->To), 
		//  "  modify " be  refer ; num hasboundary(before 5 place), preventnolimit . 
		detail := "按热词/别名/近音改写（K9）"
		if tr := rewriteTrace(corrs); tr != "" {
			detail += "：改写留痕 " + tr
		}
		emit("hotcache", "cache:hotword+alias+edit", detail, p.now().Sub(t))
	}

	// 5) tgtpt  : onlywrite Punctuated(pos   , C1 v2). 
	//    noin  voice   tgtpt(and Engine   pureNoise     , C3). 
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

// isNoiseResult     is   " sentno usein "(data    tgtpt). 
func isNoiseResult(cands []Candidate) bool {
	for _, c := range cands {
		if len(c.Reason) >= len(askNoiseReasonPrefix) && c.Reason[:len(askNoiseReasonPrefix)] == askNoiseReasonPrefix {
			return true
		}
	}
	return false
}

// rewriteTrace pipe  modifywrite and  From->To  listize(hasboundary: before 5 place; empty From/To   and). 
//  is"modifywrite  " pos : ofafteruseuserpt   modify  time,  faceuse corrections refer  bodyword. 
func rewriteTrace(corrs []Correction) string {
	if len(corrs) == 0 {
		return ""
	}
	const max = 5
	var parts []string
	for _, c := range corrs {
		if c.From == "" || c.To == "" {
			continue
		}
		parts = append(parts, c.From+"→"+c.To)
		if len(parts) >= max {
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	s := strings.Join(parts, "、")
	if len(corrs) > max {
		s += "…（共 " + strconv.Itoa(len(corrs)) + " 处）"
	}
	return s
}
