package input

import (
	"strings"

	"voicesign-harness/contract"
)

// 骨架触发词表（架构 §5.2 收敛版，本阶段实现 6 类意图）。
var (
	timeTriggers     = []string{"几点", "时间", "日期", "星期"}
	fileListTriggers = []string{"列表", "目录", "文件夹", "有什么"}
	fileReadTriggers = []string{"打开文件", "看看内容", "读文件", "读"}
	shellTriggers    = []string{"运行", "执行", "跑一下", "命令"}
	// infoTriggers：知识问答类触发词（架构 §5.2：翻译/总结/摘要/问答/研究…），交模型。
	infoTriggers = []string{"翻译", "总结", "摘要", "问答", "搜索", "查一下", "解释", "研究"}
)

// pathVerbs 是「文件夹/目录/文件」前词可能残留的动词前缀，按长词在前排列避免抢先切分。
var pathVerbs = []string{"打开", "查看", "读取", "看看", "显示", "读"}

// 各类意图的本地置信度基准。
const (
	baseTime           = 0.95
	baseFileList       = 0.8
	baseFileRead       = 0.8
	baseShell          = 0.7
	baseInfo           = 0.4
	baseUnknown        = 0.2
	missingSlotPenalty = 0.2
)

// askTemplate 是低置信/未知意图的回问文案（架构 §5.2）。
const askTemplate = "你是想让我…？请再说一遍"

// Classifier 用确定性规则把纠错后文本分类为意图 JSON（架构 §5.2）。
type Classifier struct {
	ConfThreshold float64 // 置信度阈值，低于且非 TIME/INFO 时回问
}

// NewClassifier 构造分类器。
func NewClassifier(conf float64) *Classifier {
	return &Classifier{ConfThreshold: conf}
}

// Classify 对纠错后文本做意图分类：关键词触发 + 槽位抽取 + 置信度 + 回问判定。
// 产出 contract.Intent，CorrectedText 填充为入参 text。
func (c *Classifier) Classify(text string) contract.Intent {
	intent := contract.Intent{
		CorrectedText: text,
	}

	switch {
	case containsAny(text, timeTriggers):
		// TIME：无必需槽位，零 LLM 直通；Ask 永不触发。
		intent.Intent = contract.IntentTime
		intent.Confidence = clamp(baseTime)

	case containsAny(text, fileListTriggers):
		// FILE_LIST：path 缺省 "."，不算缺失槽位。
		path, found := extractPath(text)
		if !found {
			path = "."
		}
		intent.Intent = contract.IntentFileList
		intent.Slots = map[string]string{"path": path}
		intent.Confidence = clamp(baseFileList)

	case containsAny(text, fileReadTriggers):
		// FILE_READ：path 为必需槽位，缺失则扣分。
		missing := 0
		path, found := extractPath(text)
		slots := map[string]string{}
		if found {
			slots["path"] = path
		} else {
			missing++
		}
		intent.Intent = contract.IntentFileRead
		if len(slots) > 0 {
			intent.Slots = slots
		}
		intent.Confidence = clamp(baseFileRead - missingSlotPenalty*float64(missing))

	case containsAny(text, shellTriggers):
		// SHELL：cmd = 首个 shell 触发词之后的剩余文本。
		cmd := extractCmd(text)
		missing := 0
		slots := map[string]string{}
		if cmd != "" {
			slots["cmd"] = cmd
		} else {
			missing++
		}
		intent.Intent = contract.IntentShell
		if len(slots) > 0 {
			intent.Slots = slots
		}
		intent.Confidence = clamp(baseShell - missingSlotPenalty*float64(missing))

	case containsAny(text, infoTriggers):
		// INFO：知识问答类输入，交模型（M2）；Ask 永不触发。
		intent.Intent = contract.IntentInfo
		intent.Confidence = clamp(baseInfo)

	default:
		// UNKNOWN：TIME/FILE_*/SHELL/INFO 均无触发词，Ask 必触发。
		intent.Intent = contract.IntentUnknown
		intent.Confidence = clamp(baseUnknown)
		intent.Ask = askTemplate
	}

	// 低置信回问：UNKNOWN 已在上方填好 Ask；其余仅在低于阈值且非 TIME/INFO 时回问。
	if intent.Intent != contract.IntentUnknown &&
		intent.Intent != contract.IntentTime &&
		intent.Intent != contract.IntentInfo &&
		intent.Confidence < c.ConfThreshold {
		intent.Ask = askTemplate
	}

	return intent
}

// containsAny 报告 text 是否含 keywords 中任一子串（大小写不敏感）。
func containsAny(text string, keywords []string) bool {
	lower := strings.ToLower(text)
	for _, k := range keywords {
		if strings.Contains(lower, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// extractPath 抽取文件类槽位 path，按优先级：
//  1. 引号内内容（“” 「」 双引号 单引号）；
//  2. 以 ~ 或 / 开头的 token；
//  3. 「文件夹/目录/文件」前一个词；
//  4. 都没有 → found=false（由调用方决定默认值）。
func extractPath(text string) (path string, found bool) {
	// 1. 引号内
	//    注意：中文引号「“」「”」「「」「」」是**多字节** UTF-8，
	//    早期实现用 text[i+1:i+1+j] 切分，会把开引号的第 2、3 字节留在结果里
	//    （“报告.docx” → "\x80\x9c报告.docx"），中文引号内的路径全被切坏。
	//    这里改为按开引号的实际字节长度推进。
	for _, q := range [][2]string{{"“", "”"}, {"「", "」"}, {"\"", "\""}, {"'", "'"}} {
		if i := strings.Index(text, q[0]); i >= 0 {
			rest := text[i+len(q[0]):]
			if j := strings.Index(rest, q[1]); j >= 0 {
				return strings.TrimSpace(rest[:j]), true
			}
		}
	}
	// 2. ~ 或 / 开头的 token
	for _, tok := range strings.Fields(text) {
		if strings.HasPrefix(tok, "~") || strings.HasPrefix(tok, "/") {
			return tok, true
		}
	}
	// 3. 「文件夹/目录/文件」前一个词 → 剥离动词前缀（打开|查看|读取|看看|读|显示）
	for _, kw := range []string{"文件夹", "目录", "文件"} {
		if i := strings.Index(text, kw); i > 0 {
			before := text[:i]
			fields := strings.Fields(before)
			if len(fields) > 0 {
				w := strings.Trim(fields[len(fields)-1], "，。、的是 ")
				w = stripPathVerbs(w)
				if w != "" {
					return w, true
				}
			}
		}
	}
	return "", false
}

// stripPathVerbs 循环剥离词首的常见动词前缀（如「打开Mansour」→「Mansour」）。
func stripPathVerbs(w string) string {
	for changed := true; changed; {
		changed = false
		for _, v := range pathVerbs {
			if strings.HasPrefix(w, v) {
				w = w[len(v):]
				changed = true
				break
			}
		}
	}
	return w
}

// extractCmd 返回首个 shell 触发词之后的剩余文本（trim 后），为空表示缺槽位。
func extractCmd(text string) string {
	lower := strings.ToLower(text)
	cut := -1
	for _, k := range shellTriggers {
		if i := strings.Index(lower, strings.ToLower(k)); i >= 0 {
			if cut < 0 || i < cut {
				cut = i + len(k)
			}
		}
	}
	if cut < 0 {
		return ""
	}
	return strings.TrimSpace(text[cut:])
}

// clamp 把置信度钳制到 [0,1]。
func clamp(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}
