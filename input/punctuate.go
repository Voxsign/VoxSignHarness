package input

import (
	"strings"
	"unicode"
)

// punctuate.go —— M7 server 端标点后处理兜底（备路径，零用户配置）。
//
// 背景：ASR 标点全自动，iOS 侧 addsPunctuation=true 为主路径（并行实施中）。
// 本文件实现 server 端兜底：若系统开关/locale 实测影响识别标点，文本进意图分类前
// 自动补句末标点，用户完全无感。**当前不接线 pipeline**——备路径，由组织者在 iOS
// 侧验证结果出来后决定是否在 Clean→Correct→Classify 之间插入本函数。
//
// 设计：
//   - 规则优先（纯中文标点，无外部依赖）；
//   - LLM 精修仅预留接口（LMRefiner，未配 key 恒为 nil → 纯规则）；
//   - 幂等：对标点已完整的文本不加改，重复调用结果一致。

// questionWords 是中文疑问触发词（命中即在句末落「？」）。
var questionWords = []string{"谁", "什么", "为什么", "怎么", "哪", "吗", "呢", "能不能", "可不可以", "是否", "多少", "几"}

// sentenceEndPunct 是视为「句末已有标点」的字符（中英文句读）。
var sentenceEndPunct = map[rune]bool{
	'。': true, '？': true, '！': true, '；': true, '…': true,
	'.': true, '?': true, '!': true, ';': true,
}

// LMRefiner 是可选 LLM 精修接口预留。
//
// 【预留，本任务不实配】：provider 未配 key 时全局 LMRefiner 恒为 nil，Punctuate 纯规则；
// 将来配 key 后由组织者注入一个实现，Punctuate 在规则落标点后可选调用它精修内部停顿。
// 接口签名冻结：输入待修文本，返回精修后文本（出错时返回原文，不得阻断主流程）。
type LMRefiner interface {
	RefinePunctuation(text string) (string, error)
}

// lmRefiner 是 LLM 精修的全局注入点；nil = 纯规则（默认）。
var lmRefiner LMRefiner = nil

// SetLMRefiner 注入 LLM 精修器（可选；传 nil 回到纯规则）。
// 由组织者在接线 M7 时调用；未调用前 Punctuate 完全是规则函数。
func SetLMRefiner(r LMRefiner) { lmRefiner = r }

// Punctuate 对 ASR 文本做句末标点兜底（中文为主）：
//   - 已有句末标点（。！？.!? 等）→ 原样返回（幂等）；
//   - 纯代码/数字/英文/URL/路径（无中文字符）→ 原样返回，不加标点；
//   - 陈述句末 → 补「。」；命中疑问词（谁/什么/为什么/吗/呢/能不能/可不可以…）→ 补「？」。
//
// 内部停顿/列表的逗号插入**刻意保持保守**（本版不做激进切分），避免将来接线后扰动
// 下游触发词子串匹配；内部停顿交由 LMRefiner 预留接口或 iOS 主路径处理。
func Punctuate(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return ""
	}

	// 幂等：句末已是句读标点 → 不动。
	runes := []rune(s)
	if sentenceEndPunct[runes[len(runes)-1]] {
		return s
	}

	// 无中文字符 → 视为代码/数字/英文/URL/路径，不加标点。
	if !hasChinese(runes) {
		return s
	}

	// 规则：疑问词 → ？，否则 → 。
	var end string
	if containsAny(s, questionWords) {
		end = "？"
	} else {
		end = "。"
	}
	out := s + end

	// LLM 精修预留（未配 key 时跳过；出错回退规则结果）。
	if lmRefiner != nil {
		if refined, err := lmRefiner.RefinePunctuation(out); err == nil && strings.TrimSpace(refined) != "" {
			return strings.TrimSpace(refined)
		}
	}
	return out
}

// hasChinese 报告 rune 序列是否含至少一个 CJK 统一表意文字。
func hasChinese(runes []rune) bool {
	for _, r := range runes {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
