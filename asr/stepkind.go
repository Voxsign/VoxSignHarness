// stepkind.go —— 轨迹步骤的**具名类型**（ASR-EXEC-05 v2 ①）。
//
// 自由字符串步骤在**编译期就写不出来**："hotcache" 这类临时名字混不进来，
// 判据不必再维护"合法步骤名白名单"（白名单必然漏：加 hotcache → 加 teach → 加 learn…）。
package asr

// TraceStepKind 是轨迹步骤的类型（枚举）。
type TraceStepKind string

const (
	StepRetain    TraceStepKind = "retain"    // 原始输入留存
	StepCorrect   TraceStepKind = "correct"   // 正文纠错（Correct 纯函数）
	StepLexicon   TraceStepKind = "lexicon"   // 词典/词表纠错
	StepPunctuate TraceStepKind = "punctuate" // 标点恢复
	StepCache     TraceStepKind = "cache"     // 缓存改写（热词/别名/近音/教的词）
)

// kindForStep 把步骤名映射为具名类型（未知名字 ⇒ 空，判据据此报红）。
func kindForStep(name string) TraceStepKind {
	switch name {
	case "retain":
		return StepRetain
	case "clean":
		return StepCorrect
	case "dict":
		return StepLexicon
	case "hotcache":
		return StepCache
	case "punctuate":
		return StepPunctuate
	default:
		return ""
	}
}
