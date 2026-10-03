// intent_model.go —— 模型兜底（**例外路径**）：只有本地低置信时才调用。
//
// 红线 #6：核心路径必须本地规则完成 —— 高置信**绝不**调模型。
// 需求 Δ7/4.9：超时/失败一律 fail-open，返回本地结果并标 degraded，不阻塞链路。
package asr

import (
	"context"
	"os"
	"strings"
	"time"
)

// 判据⑫：**同一个阈值常数不得表达两个不同语义**。
//
// ⚠️ 这两个语义**共用同一个值是有意的**（都是在"足够高置信"这条线上分岔）：
//   - ConfidenceThresholdAsk   ：是否回问（NeedDisambiguate = Confidence < 此值）
//   - ConfidenceThresholdModel ：是否调用模型（Confidence >= 此值 走规则路径）
//
// **值相同，但名字不同** ⇒ 将来若要单独调整任一条，改一处即可，且**影响面清晰**。
// 反例判据：源码里**不得再出现字面量 0.70**（两处必须用具名常量）。
const (
	ConfidenceThresholdAsk   = 0.70
	ConfidenceThresholdModel = 0.70
)

// IntentModel 是"意图兜底模型"的最小接口。
// 由 modelcenter.Registry 结构化实现（本包不依赖 modelcenter，保持单向/解耦）。
type IntentModel interface {
	ClassifyIntent(ctx context.Context, text string) (typ string, confidence float64, err error)
}

// DefaultIntentTimeout 是模型兜底超时（需求 Δ7：3s）。
const DefaultIntentTimeout = 3 * time.Second

// knownIntents 是合法的 9 类意图（控制语义另有 control 字段）。
var knownIntents = map[string]bool{
	"EDIT": true, "DEBUG": true, "QUERY": true, "TEST": true, "COMMIT": true,
	"DEPLOY": true, "NOTE": true, "ASK": true, "ORCHESTRATE": true,
}

// ClassifyIntentWith 本地优先；仅在低置信时走模型兜底，任何失败都降级。
func ClassifyIntentWith(ctx context.Context, text string, model IntentModel, timeout time.Duration) IntentResult {
	ir := ClassifyIntent(text)
	if ir.Confidence >= ConfidenceThresholdModel {
		return ir // 高置信：核心路径本地，绝不调模型（红线 #6）
	}
	if model == nil {
		// 没有可用兜底模型时不静默：若处于"超时注入"场景，如实标降级（链路无兜底）。
		if os.Getenv("VHS_ASR_FORCE_MODEL_TIMEOUT") == "1" {
			ir.Degraded = true
			ir.DegradedReason = "模型兜底不可用/超时（fail-open，返回本地结果）"
			ir.NeedDisambiguate = true
		}
		return ir
	}
	if timeout <= 0 {
		timeout = DefaultIntentTimeout
	}
	if os.Getenv("VHS_ASR_FORCE_MODEL_TIMEOUT") == "1" {
		timeout = 20 * time.Millisecond // 注入场景：让**真实**调用超时（不是跳过调用）
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	typ, conf, err := model.ClassifyIntent(cctx, text)
	if err != nil {
		ir.Degraded = true
		ir.DegradedReason = "模型兜底失败/超时（fail-open，返回本地结果）：" + err.Error()
		ir.NeedDisambiguate = true
		return ir
	}
	typ = strings.ToUpper(strings.TrimSpace(typ))
	if !knownIntents[typ] {
		ir.Degraded = true
		ir.DegradedReason = "模型返回非法意图类别，已忽略（fail-open）"
		ir.NeedDisambiguate = true
		return ir
	}
	ir.Type = typ
	if conf > 0 {
		ir.Confidence = conf
	}
	ir.NeedDisambiguate = ir.Confidence < ConfidenceThresholdAsk
	return ir
}
