package input

import "testing"

// R10 (2026-10-09): bare iOS continuation/progress words must route by rule to
// CONTINUE/QUERY and never fall to the LLM four-way classifier (which has no
// CONTINUE class and misreads them as EDIT).
func TestR10ContinuationAndProgressRouting(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct {
		text   string
		want   string
		reason string
	}{
		{"继续", "CONTINUE", "bare resume word"},
		{"接着", "CONTINUE", "bare resume word"},
		{"接着弄", "CONTINUE", "bare resume word"},
		{"继续推进", "CONTINUE", "urge resume"},
		{"怎么还没好", "CONTINUE", "progress urge"},
		{"还没好", "CONTINUE", "progress urge"},
		{"做完了吗", "QUERY", "status ask"},
		{"完成了吗", "QUERY", "status ask"},
		{"那封回复了没", "QUERY", "email follow-up ask"},
		{"回了没", "QUERY", "email follow-up ask"},
	}
	for _, tc := range cases {
		it := c.ClassifyTask(tc.text)
		if it.Intent != tc.want {
			t.Errorf("%q (%s): got %s, want %s (conf %.2f)",
				tc.text, tc.reason, it.Intent, tc.want, it.Confidence)
		}
	}
}
