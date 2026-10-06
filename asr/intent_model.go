// intent_model.go --  type bot(**exampleoutpath**): onlyhasbaselylow-confidencetimeonlycalluse. 
//
//  line #6:   path  baselyruledone --    **  **call type. 
// needrequire Δ7/4.9:  time/     fail-open, returnbackbaselyclose andtgt degraded,    chainroute. 
package asr

import (
	"context"
	"os"
	"strings"
	"time"
)

//  data⑫: **same   value num  table    samesemantic**. 
//
// ⚠️    semantic** usesame  valueishas  **(allis "     "  lineonsplit ): 
//   - ConfidenceThresholdAsk   : is clarification(NeedDisambiguate = Confidence <  value)
//   - ConfidenceThresholdModel : is calluse type(Confidence >=  value  rulepath)
//
// **value same, butnamechar same** ⇒ will ifneed  call    , modify placei.e. , and**  face  **. 
// revexample data:  code **  againoutnowcharface  0.70**( place  use name  ). 
const (
	ConfidenceThresholdAsk   = 0.70
	ConfidenceThresholdModel = 0.70
)

// IntentModel is"intent bot type"   connect . 
// by modelcenter.Registry close ize now(this package dependency modelcenter, keepkeep to/resolve ). 
type IntentModel interface {
	ClassifyIntent(ctx context.Context, text string) (typ string, confidence float64, err error)
}

// DefaultIntentTimeout is type bot time(needrequire Δ7: 3s). 
const DefaultIntentTimeout = 3 * time.Second

// knownIntents is    9 classintent(controlsemantic has control charseg). 
var knownIntents = map[string]bool{
	"EDIT": true, "DEBUG": true, "QUERY": true, "TEST": true, "COMMIT": true,
	"DEPLOY": true, "NOTE": true, "ASK": true, "ORCHESTRATE": true,
}

// ClassifyIntentWith basely first; only low-confidencetime  type bot,     all  . 
func ClassifyIntentWith(ctx context.Context, text string, model IntentModel, timeout time.Duration) IntentResult {
	ir := ClassifyIntent(text)
	if ir.Confidence >= ConfidenceThresholdModel {
		return ir //    :   pathbasely,   call type( line #6)
	}
	if model == nil {
		//  has use bot typetime   : ifplaceat" timenotein" scenario, e.g. tgt  (chainrouteno bot). 
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
		timeout = 20 * time.Millisecond // notein scenario:  **  **calluse time( is edcalluse)
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
