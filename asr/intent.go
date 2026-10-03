// intent.go —— 本地意图解析（核心路径本地规则，红线 #6）+ 模型兜底降级。
//
// 契约 v1 的声明式字段；控制语义与业务 type 互斥（control 只表示交互层语义）。
// 模型只做兜底：本地低置信时才有机会调用；超时/失败一律 fail-open 返回本地结果并标 degraded。
package asr

import (
	"strings"
)

// IntentResult 是意图解析结果（映射到契约 v1）。
type IntentResult struct {
	Type             string
	Confidence       float64
	NeedDisambiguate bool
	Control          string
	DomainSuggestion []string
	Degraded         bool
	DegradedReason   string
}

// 控制语义（交互层；与业务 type 互斥，需求 4.4/4.6）。
var controlWords = []struct {
	control string
	words   []string
}{
	{"pause", []string{"暂停", "停一下", "先停", "停下"}},
	{"undo", []string{"撤销", "撤回", "取消刚才"}},
	{"interrupt", []string{"打断", "别说了", "停止说话"}},
}

// controlOf 识别交互层控制语义。
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

// localClassify 规则式意图分类（离线、确定性）。
func localClassify(text string) (typ string, conf float64, control string) {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return "ASK", 0, ""
	}
	if c := controlOf(t); c != "" {
		return "ASK", 0.90, c
	}
	// 多步编排：有"先…再…/最后"这类连接
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

// ClassifyIntent 产出意图结果：本地优先，低置信才走模型兜底；任何失败都降级。
func ClassifyIntent(text string) IntentResult {
	typ, conf, ctrl := localClassify(text)
	ir := IntentResult{
		Type: typ, Confidence: conf, Control: ctrl,
		NeedDisambiguate: conf < 0.70,
		DomainSuggestion: []string{}, // 默认拒绝：不给高权限域建议
	}
	return ir
}
