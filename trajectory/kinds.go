// kinds.go —— 轨迹 kind 的**单一枚举来源**（判据⑪的地基）。
//
// 动因（Lead 2026-10-03，由实现方警告升级为判据）：
//
//	若判据引用了**不存在的 kind 名**，它会"因为匹配不到任何轨迹而空过" ——
//	**判据看起来在断言，实际断言了一个不存在的符号**。这比一般假绿更隐蔽。
//
// ⇒ 修的是**结构**：kind 必须有单一枚举；写出时遇到未登记的 kind **必须报错**（不许忽略）。
package trajectory

import "fmt"

// KindIntentSource 记录"意图来源"（值域 asr-intent | text-fallback）。
//
// ⚠️ **必须在 KindIntent 之前写入**（否则"意图来源"late-bind，读轨迹时看不到因果）。
const KindIntentSource = "intent_source"

// IntentSourceASR / IntentSourceTextFallback 是 KindIntentSource 的**枚举值域**。
const (
	IntentSourceASR          = "asr-intent"
	IntentSourceTextFallback = "text-fallback"
)

// KindTaskMetrics 记录结构化任务指标（#52 摘要聚合源：loop/net/space/attr/ok）。
// 由 pipeline.writeTaskMetrics 写入，Summary 按此聚合。
const KindTaskMetrics = "task_metrics"

// Kinds 是全部合法 kind 的**唯一真值**（新增 kind 必须在此登记）。
var Kinds = map[string]bool{
	KindTaskMetrics:  true,
	KindInputRaw:     true,
	KindInputClean:   true,
	KindInputCorrec:  true,
	KindIntentSource: true, // 本轮新增
	KindIntent:       true,
	KindStart:        true,
	KindModel:        true,
	KindActions:      true,
	KindReceipts:     true,
	KindFinal:        true,
	KindError:        true,
	// P0-1：13 阶段中间判定事件登记（此前 pipeline 写入点引用这些 kind 但未登记 ⇒ 被 Validate 静默丢弃）。
	KindRefer:       true,
	KindSpaceCheck:  true,
	KindRisk:        true,
	KindConfirm:     true,
	KindVerify:      true,
	KindAttribution: true,
	KindReplyGen:    true, // Phase 1：回答生成（provider 调用日志）
}

// KnownKind 报告 kind 是否已登记。
func KnownKind(kind string) bool { return Kinds[kind] }

// ValidIntentSource 报告意图来源是否在值域内（不许自由文本）。
func ValidIntentSource(v string) bool {
	return v == IntentSourceASR || v == IntentSourceTextFallback
}

// Validate 校验一条轨迹条目；**未登记的 kind ⇒ 报错**（不许静默丢弃/忽略）。
func Validate(e Entry) error {
	if e.Kind == "" {
		return fmt.Errorf("trajectory: kind 为空（必须登记；见 trajectory.Kinds）")
	}
	if !KnownKind(e.Kind) {
		return fmt.Errorf("trajectory: 未登记的 kind %q（必须先在 trajectory.Kinds 登记）", e.Kind)
	}
	if e.Kind == KindIntentSource && !ValidIntentSource(e.Content) {
		return fmt.Errorf("trajectory: intent_source 值 %q 不在值域 [%s, %s] 内",
			e.Content, IntentSourceASR, IntentSourceTextFallback)
	}
	return nil
}
