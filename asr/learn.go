// learn.go --   calluse(`learn`) **baselytriggersend **: from   use    disconnect"  value  ". 
//
// to  ASR-MODEL-02 / `LEARN-01`:   **  ** diffvalue     seg, but isetc    . 
// basefile**  type**: onlyusebaselyrule + alreadyhas  (  /word is alreadyoverwrite   pos)  disconnect, 
// because  by  model in   andbot  type  ofbeforefirst ly. 
//
//  boundary(and    ): 
//   - DetectLearnCandidates is**  num**: read-only in,  write  file(R5); 
//   -  **  readget** UsageEvent.LearnLabel( islang   "period ",  is in)--R4 by  keep ; 
//   - triggersend  :    knowledge_key     (same key event and,  data numhasboundary); 
//   - prevent   backroute: kind == learn_derived  event**  **triggersend(E5). 
//
//  only**produceout  **;  pos "calluse type getrule + writeback"  LEARN-02 ofafter(etc typeand  ). 
package asr

import "strings"

// eventkindclass(triggersend     name ). 
const (
	eventUserCorrection = "user_correction"
	eventDictMiss       = "dict_miss"
	eventRuleOverride   = "rule_override"
	eventDiagnoseCause  = "diagnose_cause"
	eventLearnDerived   = "learn_derived" // forbidstopagaintriggersend(preventbackroute)
)

// learnMaxBatch issame  knowledge_key   keep  ** data num**(   hasboundary). 
// event by    , but    nolimitadd ,  dataalso  noboundary  . 
const learnMaxBatch = 8

// UsageEvent is     use   (append-only event  usage-events.jsonl). 
type UsageEvent struct {
	ID         string `json:"id"`
	At         string `json:"at,omitempty"`
	Kind       string `json:"kind"`
	Raw        string `json:"raw"`
	Final      string `json:"final,omitempty"`
	Confirmed  bool   `json:"confirmed"`
	Outcome    string `json:"outcome,omitempty"`
	TraceRef   string `json:"trace_ref,omitempty"`
	LearnLabel string `json:"learn_label,omitempty"` // **period tgt **: onlylang /   use, Detector   readget
	Provenance string `json:"provenance,omitempty"`
	Note       string `json:"note,omitempty"`
}

// EvidenceRef is  writeback data(L3: no   writeback). 
type EvidenceRef struct {
	EventID  string `json:"event_id"`
	Kind     string `json:"kind,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
	TraceRef string `json:"trace_ref,omitempty"`
	At       string `json:"at,omitempty"`
}

// LearnCandidate is  "value  "   . 
type LearnCandidate struct {
	KnowledgeKey string        `json:"knowledge_key"`
	Kind         string        `json:"kind"`    // dictionary | pattern
	Trigger      string        `json:"trigger"` //  in triggersendrulename
	Reason       string        `json:"reason"`
	Evidence     []EvidenceRef `json:"evidence"`
}

// DetectLearnCandidates    use  , returnback"value sendraise   learn calluse"   . 
//
// P1 status: **connect  + classtypefirstat now**( data LEARN-01 first ). 
//  nowsee learn_detect.go; base numonly  numverifyafter  . 
func DetectLearnCandidates(engine *Personalized, dict *Dictionary, events []UsageEvent) []LearnCandidate {
	return detectLearnCandidates(engine, dict, events)
}

// knowledgeKey is   unique : same place pos(same raw->final)    tosame   , 
// atisheavy eventonly and data,   heavy sendraise  . 
func knowledgeKey(raw, final string) string {
	return raw + "=>" + final
}

// learnableKind   eventkindclassis  at"  value  " classdiff(   voice/  / artifact). 
func learnableKind(kind string) bool {
	switch kind {
	case eventUserCorrection, eventDictMiss, eventRuleOverride, eventDiagnoseCause:
		return true
	default:
		return false
	}
}

// normalizeText   firsttailempty (event base   empty ). 
func normalizeText(s string) string { return strings.TrimSpace(s) }
