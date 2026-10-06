package input

import (
	"strings"

	"voicesign-harness/contract"
)

//   triggersendwordtable(   §5.2 recv  , basestage now 6 classintent). 
var (
	timeTriggers     = []string{"几点", "时间", "日期", "星期"}
	fileListTriggers = []string{"列表", "目录", "文件夹", "有什么"}
	fileReadTriggers = []string{"打开文件", "看看内容", "读文件", "读"}
	shellTriggers    = []string{"运行", "执行", "跑一下", "命令"}
	// infoTriggers:     classtriggersendword(   §5.2:   / close/ need/  /  …),   type. 
	infoTriggers = []string{"翻译", "总结", "摘要", "问答", "搜索", "查一下", "解释", "研究"}
	// chatTriggers:   / rev /    (2026-10-04 useuser   freqorig ). 
	//  before class   in UNKNOWN -> triggersend" is      "clarification(useuserlinkcontinue    ptname). 
	// fix : andin INFO class  type/basely connectback , Ask   triggersend. 
	chatTriggers = []string{
		"糟糕", "乱七八糟", "一塌糊涂", "没有反馈", "啥都没有", "没有反应", "有没有反应",
		"懂我", "懂你", "效果怎么样", "有没有干活", "在干嘛", "干什么呢", "有什么用",
		"有什么意义", "没价值", "不好用", "怎么用", "你也不管", "没管", "老是", "一直提示",
		"拦截", "听不懂", "听不清", "没听懂", "不明白", "扯", "试试看", "测试一下",
		// 2026-10-04 useuser  " connectopenstart  / connect "be UNKNOWN clarification ->   ASK   back ,  again  clarification. 
		"直接干", "直接开始", "开始干", "动手吧", "直接动手", "赶紧", "马上开始",
	}
)

// pathVerbs is"file /obj /file"beforeword      wordbefore , by word before list   first split. 
var pathVerbs = []string{"打开", "查看", "读取", "看看", "显示", "读"}

//  classintent basely   baseapprove. 
const (
	baseTime           = 0.95
	baseFileList       = 0.8
	baseFileRead       = 0.8
	baseShell          = 0.7
	baseInfo           = 0.4
	baseUnknown        = 0.2
	missingSlotPenalty = 0.2
)

// askTemplate islow-confidence/  intent clarification  (   §5.2). 
const askTemplate = "你是想让我…？请再说一遍"

// Classifier use  ityrulepipecorrectionafter baseclassifyasintent JSON(   §5.2). 
type Classifier struct {
	ConfThreshold float64 //     value,  atand  TIME/INFO timeclarification
}

// NewClassifier   classify . 
func NewClassifier(conf float64) *Classifier {
	return &Classifier{ConfThreshold: conf}
}

// Classify tocorrectionafter base intentclassify: close wordtriggersend +    get +     + clarification  . 
// produceout contract.Intent, CorrectedText  fillasin  text. 
func (c *Classifier) Classify(text string) contract.Intent {
	intent := contract.Intent{
		CorrectedText: text,
	}

	switch {
	case containsAny(text, timeTriggers):
		// TIME: no need  ,   LLM   ; Ask   triggersend. 
		intent.Intent = contract.IntentTime
		intent.Confidence = clamp(baseTime)

	case containsAny(text, fileListTriggers):
		// FILE_LIST: path    ".",       . 
		path, found := extractPath(text)
		if !found {
			path = "."
		}
		intent.Intent = contract.IntentFileList
		intent.Slots = map[string]string{"path": path}
		intent.Confidence = clamp(baseFileList)

	case containsAny(text, fileReadTriggers):
		// FILE_READ: path as need  ,   then split. 
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
		// SHELL: cmd = first  shell triggersendwordofafter    base. 
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
		// INFO:     class in,   type(M2); Ask   triggersend. 
		intent.Intent = contract.IntentInfo
		intent.Confidence = clamp(baseInfo)

	case containsAny(text, chatTriggers):
		// CHAT/ rev :   ,   ,   class in("after has has  "" diff   ""   "). 
		// 2026-10-04 useuser    ptname" is      "clarification-- class     in UNKNOWN. 
		// andin INFO   type/basely connectback (e.g."   resolve is   …"), Ask   triggersend. 
		intent.Intent = contract.IntentInfo
		intent.Confidence = clamp(baseInfo)

	default:
		// UNKNOWN: TIME/FILE_*/SHELL/INFO  notriggersendword, Ask  triggersend. 
		intent.Intent = contract.IntentUnknown
		intent.Confidence = clamp(baseUnknown)
		intent.Ask = askTemplate
	}

	// low-confidenceclarification: UNKNOWN already on    Ask; its only  at valueand  TIME/INFO timeclarification. 
	if intent.Intent != contract.IntentUnknown &&
		intent.Intent != contract.IntentTime &&
		intent.Intent != contract.IntentInfo &&
		intent.Confidence < c.ConfThreshold {
		intent.Ask = askTemplate
	}

	return intent
}

// containsAny    text is   keywords in    (  write   ). 
func containsAny(text string, keywords []string) bool {
	lower := strings.ToLower(text)
	for _, k := range keywords {
		if strings.Contains(lower, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// extractPath  getfileclass   path, by first : 
//  1.  idinin (“” ""   id   id); 
//  2. by ~ or / openhead  token; 
//  3. "file /obj /file"before  word; 
//  4. all has -> found=false(bycalluse decide defaultvalue). 
func extractPath(text string) (path string, found bool) {
	// 1.  idin
	//    note : in  id"“""”"""""""is** charnode** UTF-8, 
	//     period nowuse text[i+1:i+1+j]  split,  pipeopen id   2, 3 charnode  close  
	//    (“  .docx” -> "\x80\x9c  .docx"), in  idin pathsafetybe  . 
	//      modifyasbyopen id   charnode    . 
	for _, q := range [][2]string{{"“", "”"}, {"「", "」"}, {"\"", "\""}, {"'", "'"}} {
		if i := strings.Index(text, q[0]); i >= 0 {
			rest := text[i+len(q[0]):]
			if j := strings.Index(rest, q[1]); j >= 0 {
				return strings.TrimSpace(rest[:j]), true
			}
		}
	}
	// 2. ~ or / openhead  token
	for _, tok := range strings.Fields(text) {
		if strings.HasPrefix(tok, "~") || strings.HasPrefix(tok, "/") {
			return tok, true
		}
	}
	// 3. "file /obj /file"before  word ->    wordbefore ( open|  |readget|  |read| show)
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

// stripPathVerbs     wordfirst  see wordbefore (e.g." openReport"->"Report"). 
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

// extractCmd returnbackfirst  shell triggersendwordofafter    base(trim after), asemptytableshow   . 
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

// clamp pipe    restrictto [0,1]. 
func clamp(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}
