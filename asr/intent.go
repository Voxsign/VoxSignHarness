// intent.go -- baselyintentresolve (  pathbaselyrule,  line #6)+  type bot  . 
//
//    v1  voice formcharseg; controlsemanticand service type mutex(control onlytableshow   semantic). 
//  typeonly  bot: baselylow-confidencetimeonlyhas  calluse;  time/     fail-open returnbackbaselyclose andtgt degraded. 
package asr

import (
	"strings"
)

// IntentResult isintentresolve close (  to   v1). 
type IntentResult struct {
	Type             string
	Confidence       float64
	NeedDisambiguate bool
	Control          string
	DomainSuggestion []string
	Degraded         bool
	DegradedReason   string
}

// controlsemantic(   ; and service type mutex, needrequire 4.4/4.6). 
var controlWords = []struct {
	control string
	words   []string
}{
	{"pause", []string{"暂停", "停一下", "先停", "停下"}},
	{"undo", []string{"撤销", "撤回", "取消刚才"}},
	{"interrupt", []string{"打断", "别说了", "停止说话"}},
}

// controlOf  diff   controlsemantic. 
func controlOf(t string) string {
	for _, c := range controlWords {
		for _, w := range c.words {
			if strings.Contains(t, w) {
				return c.control
			}
		}
	}
	return ""
}

// localClassify ruleformintentclassify( line,   ity). 
func localClassify(text string) (typ string, conf float64, control string) {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return "ASK", 0, ""
	}
	if c := controlOf(t); c != "" {
		return "ASK", 0.90, c
	}
	//   orchestrate: has"first…again…/ after" classlinkconnect
	if strings.Contains(t, "先") && (strings.Contains(t, "再") || strings.Contains(t, "最后")) {
		return "ORCHESTRATE", 0.80, ""
	}
	switch {
	case strings.Contains(t, "提交"), strings.Contains(t, "commit"):
		return "COMMIT", 0.85, ""
	case strings.Contains(t, "部署"), strings.Contains(t, "上线"), strings.Contains(t, "发布"):
		return "DEPLOY", 0.85, ""
	case strings.Contains(t, "测试"), strings.Contains(t, "go test"):
		return "TEST", 0.85, ""
	case strings.Contains(t, "记一下"), strings.Contains(t, "记下"), strings.Contains(t, "记录"), strings.Contains(t, "笔记"):
		return "NOTE", 0.85, ""
	case strings.Contains(t, "改成"), strings.Contains(t, "修改"), strings.Contains(t, "编辑"), strings.Contains(t, "换成"):
		return "EDIT", 0.80, ""
	case strings.Contains(t, "报错"), strings.Contains(t, "调试"), strings.Contains(t, "为什么失败"), strings.Contains(t, "修bug"), strings.Contains(t, "修 bug"):
		return "DEBUG", 0.80, ""
	case strings.Contains(t, "查"), strings.Contains(t, "库存"), strings.Contains(t, "多少"), strings.Contains(t, "看一下"):
		return "QUERY", 0.70, ""
	case strings.Contains(t, "怎么"), strings.Contains(t, "为什么"), strings.Contains(t, "如何"), strings.Contains(t, "你觉得"):
		return "ASK", 0.70, ""
	default:
		return "ASK", 0.30, ""
	}
}

// ClassifyIntent produceoutintentclose : basely first, low-confidenceonly  type bot;     all  . 
func ClassifyIntent(text string) IntentResult {
	typ, conf, ctrl := localClassify(text)
	ir := IntentResult{
		Type: typ, Confidence: conf, Control: ctrl,
		NeedDisambiguate: conf < 0.70,
		DomainSuggestion: []string{}, // defaultreject:  give  limitdomain  
	}
	return ir
}
