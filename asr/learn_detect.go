// learn_detect.go —— LEARN-01 触发器的实现体（本地、零模型、纯函数）。
//
// 判断逻辑（全部可解释、可回放）：
//  1. 只认四类"可能值得学"的事件；kind == learn_derived **永不**触发（防自训练回路）；
//  2. 必须用户已确认（confirmed）——未确认的编辑不是知识；
//  3. raw == final 时无可学内容；
//  4. **覆盖检查**：若本地规则/词典已能把 raw 变成 final，说明"已经会了"→ 不触发；
//  5. 同一 knowledge_key 只产一条候选，证据条数有界（learnMaxBatch）。
//
// 覆盖检查复用已有能力（Engine.Correct 恒为纯函数；Dictionary.Apply 只读快照），
// 因此 Detector 不引入新的状态，也不需要任何模型。
package asr

// detectLearnCandidates 见包注释与 learn.go 的接口说明。
func detectLearnCandidates(engine *Personalized, dict *Dictionary, events []UsageEvent) []LearnCandidate {
	byKey := map[string]*LearnCandidate{}
	var order []string

	for _, e := range events {
		if e.Kind == eventLearnDerived {
			continue // E5：learn 自己的产物不得再次触发学习
		}
		if !learnableKind(e.Kind) {
			continue
		}
		if !e.Confirmed {
			continue // L3：没有用户确认就没有知识来源
		}
		raw, final := normalizeText(e.Raw), normalizeText(e.Final)
		if raw == "" || final == "" || raw == final {
			continue
		}
		if localCoverage(engine, dict, raw, final) {
			continue // 本地已经会了：重复学习只会固化噪声
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

// triggerName 把事件种类映射到可读的触发规则名（写进候选，便于审计）。
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

// localCoverage 报告本地能力（规则引擎 / 词典）是否已能把 raw 变成 final。
// 只读、无副作用；engine/dict 允许为 nil。
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
