// learn_detect.go -- LEARN-01 triggersend   nowbody(basely,   type,   num). 
//
//  disconnect  (safety  resolve ,  back ): 
//  1. only  class"  value  " event; kind == learn_derived **  **triggersend(prevent   backroute); 
//  2.   useuseralreadyconfirm(confirmed)-- confirm    is  ; 
//  3. raw == final timeno  in ; 
//  4. **overwrite  **: ifbaselyrule/word already pipe raw changebecome final,   "already  "->  triggersend; 
//  5. same  knowledge_key onlyproduce    ,  data numhasboundary(learnMaxBatch). 
//
// overwrite   usealreadyhas  (Engine.Correct  as  num; Dictionary.Apply read-onlyfast ), 
// because  Detector   innew status, also needneed   type. 
package asr

// detectLearnCandidates see note and learn.go  connect   . 
func detectLearnCandidates(engine *Personalized, dict *Dictionary, events []UsageEvent) []LearnCandidate {
	byKey := map[string]*LearnCandidate{}
	var order []string

	for _, e := range events {
		if e.Kind == eventLearnDerived {
			continue // E5: learn    artifact  again triggersend  
		}
		if !learnableKind(e.Kind) {
			continue
		}
		if !e.Confirmed {
			continue // L3:  hasuseuserconfirmthen has    
		}
		raw, final := normalizeText(e.Raw), normalizeText(e.Final)
		if raw == "" || final == "" || raw == final {
			continue
		}
		if localCoverage(engine, dict, raw, final) {
			continue // baselyalready  : heavy   only  ize voice
		}

		key := knowledgeKey(raw, final)
		c := byKey[key]
		if c == nil {
			c = &LearnCandidate{
				KnowledgeKey: key,
				Kind:         "dictionary",
				Trigger:      triggerName(e.Kind),
				Reason:       "本地规则/词典未覆盖这次纠正（" + e.Kind + "），值得发起一次 learn 调用",
			}
			byKey[key] = c
			order = append(order, key)
		}
		if len(c.Evidence) < learnMaxBatch {
			c.Evidence = append(c.Evidence, EvidenceRef{
				EventID:  e.ID,
				Kind:     e.Kind,
				Outcome:  e.Outcome,
				TraceRef: e.TraceRef,
				At:       e.At,
			})
		}
	}

	out := make([]LearnCandidate, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// triggerName pipeeventkindclass  to read triggersendrulename(write   , thenat  ). 
func triggerName(kind string) string {
	switch kind {
	case eventUserCorrection:
		return "E1_user_correction"
	case eventDictMiss:
		return "E2_dict_miss"
	case eventRuleOverride:
		return "E3_rule_override"
	case eventDiagnoseCause:
		return "E4_diagnose_cause"
	default:
		return "unknown"
	}
}

// localCoverage   basely  (rule   / word )is already pipe raw changebecome final. 
// read-only, no  use; engine/dict  allowas nil. 
func localCoverage(engine *Personalized, dict *Dictionary, raw, final string) bool {
	if engine != nil && engine.Correct(CorrectRequest{Raw: raw}).Text == final {
		return true
	}
	if dict != nil {
		if text, _ := dict.Apply(raw); text == final {
			return true
		}
	}
	return false
}
