package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseActionPlan_Happy(t *testing.T) {
	raw := `{"actions":[{"tool":"shell","args":{"cmd":"ls"}},{"tool":"get_time"}],"final":"完成"}`
	plan, err := ParseActionPlan(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Actions) != 2 {
		t.Fatalf("want 2 actions, got %d", len(plan.Actions))
	}
	if plan.Actions[0].Seq != 1 || plan.Actions[1].Seq != 2 {
		t.Fatalf("seq normalization failed: %+v", plan.Actions)
	}
	if plan.Final != "完成" {
		t.Fatalf("final mismatch: %q", plan.Final)
	}
}

func TestParseActionPlan_Fenced(t *testing.T) {
	raw := "```json\n{\"actions\":[{\"tool\":\"get_time\"}]}\n```"
	plan, err := ParseActionPlan(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Tool != "get_time" {
		t.Fatalf("bad plan: %+v", plan)
	}
}

func TestParseActionPlan_FinalOnly(t *testing.T) {
	plan, err := ParseActionPlan(`{"final":"直接回答"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Actions) != 0 || plan.Final != "直接回答" {
		t.Fatalf("bad plan: %+v", plan)
	}
}

func TestParseActionPlan_Empty(t *testing.T) {
	for _, raw := range []string{``, `{}`, `{"actions":[]}`} {
		if _, err := ParseActionPlan(raw); err == nil {
			t.Fatalf("want error for raw=%q", raw)
		}
	}
}

func TestParseActionPlan_MissingTool(t *testing.T) {
	_, err := ParseActionPlan(`{"actions":[{"args":{}}]}`)
	if err == nil || !strings.Contains(err.Error(), "missing tool") {
		t.Fatalf("want missing-tool error, got: %v", err)
	}
}

func TestParseActionPlan_InvalidJSON(t *testing.T) {
	_, err := ParseActionPlan(`not json at all`)
	if err == nil {
		t.Fatal("want error for invalid json")
	}
}

func TestIntentNeedsClarification(t *testing.T) {
	i := &Intent{Intent: IntentUnknown, Confidence: 0.2, Ask: "你是想让我做什么？请再说一遍"}
	if !i.NeedsClarification() {
		t.Fatal("ask 非空时应需要回问")
	}
	i.Ask = ""
	if i.NeedsClarification() {
		t.Fatal("ask 为空时不应回问")
	}
}

func TestIntentJSONRoundTrip(t *testing.T) {
	in := Intent{
		Intent:        IntentFileList,
		Slots:         map[string]string{"path": "~/Documents/Mansour"},
		Confidence:    0.8,
		CorrectedText: "打开 Mansour 的文件夹看看有什么",
		Corrections:   []Correction{{From: "美墅", To: "Mansour", Rule: "dict"}},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Intent
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Intent != IntentFileList || out.Slots["path"] != "~/Documents/Mansour" ||
		out.Confidence != 0.8 || len(out.Corrections) != 1 || out.Corrections[0].From != "美墅" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}
