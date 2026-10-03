// learn_test.go —— 学习触发器（LEARN-01）的默认门禁单测（不带 tag）。
//
// 判据本体在 learn_criteria_test.go（vhs002，跑语料）；这里测单点规则，
// 让默认 `go test ./...` 就能守住"什么该学、什么不该学"。
package asr

import (
	"path/filepath"
	"reflect"
	"testing"
)

func ev(id, kind, raw, final string, confirmed bool) UsageEvent {
	return UsageEvent{ID: id, Kind: kind, Raw: raw, Final: final, Confirmed: confirmed, Outcome: "accepted"}
}

// TestLearnDetectFindsUncoveredCorrection：本地不覆盖的已确认纠正 → 值得学。
func TestLearnDetectFindsUncoveredCorrection(t *testing.T) {
	got := DetectLearnCandidates(NewEngine(), nil, []UsageEvent{
		ev("e1", eventUserCorrection, "把deept接上", "把DeepSeek接上", true),
	})
	if len(got) != 1 {
		t.Fatalf("期望 1 条候选，实际 %d: %+v", len(got), got)
	}
	if got[0].KnowledgeKey != knowledgeKey("把deept接上", "把DeepSeek接上") {
		t.Errorf("knowledge_key 不符: %q", got[0].KnowledgeKey)
	}
	if len(got[0].Evidence) != 1 || got[0].Evidence[0].EventID != "e1" {
		t.Errorf("证据不精确: %+v", got[0].Evidence)
	}
	if got[0].Trigger != "E1_user_correction" || got[0].Reason == "" {
		t.Errorf("审计字段缺失: %+v", got[0])
	}
}

// TestLearnDetectCoveredCorrectionNotTriggered：本地规则已覆盖 → 不学。
func TestLearnDetectCoveredCorrectionNotTriggered(t *testing.T) {
	cases := [][2]string{
		{"嗯那个呃记一下这个想法", "记一下这个想法"},     // 填充词规则
		{"帮我把这个文件题交一下", "帮我把这个文件提交一下"}, // 近音词表
		{"报价页那个别字", "报价页那个错别字"},        // 截断还原
	}
	for _, c := range cases {
		got := DetectLearnCandidates(NewEngine(), nil, []UsageEvent{
			ev("e", eventUserCorrection, c[0], c[1], true),
		})
		if len(got) != 0 {
			t.Errorf("本地已覆盖却仍触发学习: %q→%q → %+v", c[0], c[1], got)
		}
	}
}

// TestLearnDetectLearnDerivedNeverTriggers：防自训练回路（E5）。
func TestLearnDetectLearnDerivedNeverTriggers(t *testing.T) {
	got := DetectLearnCandidates(NewEngine(), nil, []UsageEvent{
		ev("e", eventLearnDerived, "把哈牛斯接上", "把harness接上", true),
	})
	if len(got) != 0 {
		t.Fatalf("learn 自产物触发了学习（回路）: %+v", got)
	}
}

// TestLearnDetectRequiresConfirmation：未确认不学（L3）。
func TestLearnDetectRequiresConfirmation(t *testing.T) {
	got := DetectLearnCandidates(NewEngine(), nil, []UsageEvent{
		ev("e", eventUserCorrection, "把哈牛斯接上", "把harness接上", false),
	})
	if len(got) != 0 {
		t.Fatalf("未确认的编辑触发了学习: %+v", got)
	}
}

// TestLearnDetectSkipsNoiseAndNavigation：噪声/导航/无差异不学。
func TestLearnDetectSkipsNoiseAndNavigation(t *testing.T) {
	got := DetectLearnCandidates(NewEngine(), nil, []UsageEvent{
		ev("e1", "noise", "呃呃呃", "呃呃呃", true),
		ev("e2", "navigation", "打开设置", "打开设置", true),
		ev("e3", eventUserCorrection, "开始测试", "开始测试", true),
	})
	if len(got) != 0 {
		t.Fatalf("不该学的事件被学: %+v", got)
	}
}

// TestLearnDetectDedupAndBounded：同 key 重复事件 → 1 条候选，证据有界。
func TestLearnDetectDedupAndBounded(t *testing.T) {
	var events []UsageEvent
	for i := 0; i < 20; i++ {
		events = append(events, ev("e"+string(rune('a'+i%26)), eventDictMiss, "哈牛斯", "harness", true))
	}
	got := DetectLearnCandidates(NewEngine(), nil, events)
	if len(got) != 1 {
		t.Fatalf("同 key 应合并为 1 条候选，实际 %d", len(got))
	}
	if len(got[0].Evidence) != learnMaxBatch {
		t.Fatalf("证据应有界为 %d，实际 %d", learnMaxBatch, len(got[0].Evidence))
	}
}

// TestLearnDetectDictionaryCoverage：词典已覆盖 → 不学。
func TestLearnDetectDictionaryCoverage(t *testing.T) {
	d, err := NewDictionary(filepath.Join(t.TempDir(), "d.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Add(DictionaryEntry{RawSpeech: "哈牛斯", Target: "harness"}); err != nil {
		t.Fatal(err)
	}
	if got := DetectLearnCandidates(NewEngine(), d, []UsageEvent{
		ev("e", eventUserCorrection, "哈牛斯", "harness", true),
	}); len(got) != 0 {
		t.Fatalf("词典已覆盖却仍触发学习: %+v", got)
	}
}

// TestLearnDetectPureAndDeterministic：纯函数——不改输入、可重放。
func TestLearnDetectPureAndDeterministic(t *testing.T) {
	events := []UsageEvent{
		ev("e1", eventUserCorrection, "把deept接上", "把DeepSeek接上", true),
		ev("e2", eventUserCorrection, "嗯那个呃记一下", "记一下", true),
		ev("e3", eventDictMiss, "哈牛斯", "harness", true),
	}
	before := make([]UsageEvent, len(events))
	copy(before, events)
	a := DetectLearnCandidates(NewEngine(), nil, events)
	if !reflect.DeepEqual(events, before) {
		t.Fatal("Detector 修改了输入")
	}
	b := DetectLearnCandidates(NewEngine(), nil, events)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("Detector 不可重放")
	}
}
