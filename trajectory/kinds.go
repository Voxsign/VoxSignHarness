// kinds.go -- trace kind  **      **( data⑪ lybase). 
//
//  because(Lead 2026-10-03, by now     as data): 
//
//	if data use** store   kind name**,   "becauseas   to  tracebutemptyed" --
//	** data raise  disconnectlang,   disconnectlang   store   id**.       change  . 
//
// ⇒ fix is**close **: kind   has    ; writeouttime to     kind **    **( allow  ). 
package trajectory

import "fmt"

// KindIntentSource   "intent  "(valuedomain asr-intent | text-fallback). 
//
// ⚠️ **    KindIntent ofbeforewrite**( then"intent  "late-bind, readtracetime  tobecause ). 
const KindIntentSource = "intent_source"

// IntentSourceASR / IntentSourceTextFallback is KindIntentSource  **  valuedomain**. 
const (
	IntentSourceASR          = "asr-intent"
	IntentSourceTextFallback = "text-fallback"
)

// KindTaskMetrics   close izetaskrefertgt(#52  need   : loop/net/space/attr/ok). 
// by pipeline.writeTaskMetrics write, Summary by   . 
const KindTaskMetrics = "task_metrics"

// Kinds issafety    kind  **unique value**(newadd kind       ). 
var Kinds = map[string]bool{
	KindTaskMetrics:  true,
	KindInputRaw:     true,
	KindInputClean:   true,
	KindInputCorrec:  true,
	KindIntentSource: true, // base newadd
	KindIntent:       true,
	KindStart:        true,
	KindModel:        true,
	KindActions:      true,
	KindReceipts:     true,
	KindFinal:        true,
	KindError:        true,
	// P0-1: 13 stagemiddle  event  ( before pipeline writept use   kind but    ⇒ be Validate     ). 
	KindRefer:       true,
	KindSpaceCheck:  true,
	KindRisk:        true,
	KindConfirm:     true,
	KindVerify:      true,
	KindAttribution: true,
}

// KnownKind    kind is already  . 
func KnownKind(kind string) bool { return Kinds[kind] }

// ValidIntentSource   intent  is  valuedomainin( allow by base). 
func ValidIntentSource(v string) bool {
	return v == IntentSourceASR || v == IntentSourceTextFallback
}

// Validate verify  trace obj; **     kind ⇒   **( allow    /  ). 
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
