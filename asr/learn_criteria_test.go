//go:build vhs002

// learn_criteria_test.go -- LEARN-01:   **  ** diff"  value  "(first ,   type). 
//
// to  ASR-MODEL-02 §3/§4 and   §4.2   R1–R5: 
//
//	R1  back: learn_label=trigger  event,   be      data  referto
//	R2   : learn_label=no_trigger  event,     all   use
//	R3 hasboundary: same  knowledge_key onlyproduce    , and data num <= learnMaxBatch
//	R4   : Detector  read LearnLabel(pipe hastgt  empty,  out   change)
//	R5 read-only: Detector is  num( modify in,  writefile,  heavy )
//
//   : go test -tags vhs002 ./asr -run TestLEARN01
package asr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func loadUsageEvents(t *testing.T) []UsageEvent {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("corpus", "usage-events.jsonl"))
	if err != nil {
		t.Fatalf("[LEARN-01] 读使用记录失败: %v", err)
	}
	var out []UsageEvent
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e UsageEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("[LEARN-01] 使用记录非法行: %v", err)
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		t.Fatal("[LEARN-01] 使用记录为空")
	}
	return out
}

func evidenceIDs(c LearnCandidate) map[string]bool {
	ids := map[string]bool{}
	for _, e := range c.Evidence {
		ids[e.EventID] = true
	}
	return ids
}

func TestLEARN01DetectorIdentifiesLearnableEvents(t *testing.T) {
	events := loadUsageEvents(t)
	eng := NewEngine()
	got := DetectLearnCandidates(eng, nil, events)

	// basebaseinvariant:       data(L3)
	for i, c := range got {
		if len(c.Evidence) == 0 {
			t.Errorf("[LEARN-01] 候选 %d (%s) 没有任何证据 —— 违反 L3", i, c.KnowledgeKey)
		}
		if c.Reason == "" || c.Trigger == "" || c.KnowledgeKey == "" {
			t.Errorf("[LEARN-01] 候选字段不全: %+v", c)
		}
	}

	// R1  back
	triggerN, coveredTrigger := 0, 0
	for _, e := range events {
		if e.LearnLabel != "trigger" {
			continue
		}
		triggerN++
		found := false
		for _, c := range got {
			if evidenceIDs(c)[e.ID] {
				found = true
				break
			}
		}
		if found {
			coveredTrigger++
		} else {
			t.Errorf("[LEARN-01 R1] 该学的事件未被识别: %s (%s)", e.ID, e.Note)
		}
	}
	if triggerN == 0 {
		t.Fatal("[LEARN-01] 语料里没有 trigger 事件，判据形同虚设")
	}

	// R2   : no_trigger event  outnow      data 
	for _, e := range events {
		if e.LearnLabel != "no_trigger" {
			continue
		}
		for _, c := range got {
			if evidenceIDs(c)[e.ID] {
				t.Errorf("[LEARN-01 R2] 不该学的事件被学: %s (%s) → %s", e.ID, e.Note, c.KnowledgeKey)
			}
		}
	}

	// R3 hasboundary: same  knowledge_key only    ;  data <= learnMaxBatch
	seen := map[string]int{}
	for _, c := range got {
		seen[c.KnowledgeKey]++
		if len(c.Evidence) > learnMaxBatch {
			t.Errorf("[LEARN-01 R3] 证据无界: %s 有 %d 条（上限 %d）", c.KnowledgeKey, len(c.Evidence), learnMaxBatch)
		}
	}
	for k, n := range seen {
		if n != 1 {
			t.Errorf("[LEARN-01 R3] 同一 knowledge_key 出现 %d 条候选: %s", n, k)
		}
	}

	// R4   : Detector   readget LearnLabel --  emptytgt after out    
	blanked := make([]UsageEvent, len(events))
	copy(blanked, events)
	for i := range blanked {
		blanked[i].LearnLabel = ""
	}
	gotBlank := DetectLearnCandidates(eng, nil, blanked)
	if !reflect.DeepEqual(got, gotBlank) {
		t.Error("[LEARN-01 R4] 输出随 LearnLabel 变化 —— Detector 读了「期望标签」，不是自主判断")
	}

	// R5 read-only:  modify in,  heavy 
	before := make([]UsageEvent, len(events))
	copy(before, events)
	_ = DetectLearnCandidates(eng, nil, events)
	if !reflect.DeepEqual(events, before) {
		t.Error("[LEARN-01 R5] Detector 修改了输入事件")
	}
	if again := DetectLearnCandidates(eng, nil, events); !reflect.DeepEqual(got, again) {
		t.Error("[LEARN-01 R5] Detector 不可重放（两次输出不同）")
	}

	t.Logf("[LEARN-01] 使用记录 %d 条（trigger %d），产出候选 %d 条（召回 %d/%d）",
		len(events), triggerN, len(got), coveredTrigger, triggerN)
}
