package input

import (
	"testing"

	"voicesign-harness/contract"
)

// R11 (2026-10-09): English continuation/status words and explicit cancellation
// must route by rule — never to the UNKNOWN ask template or the ask-confirm loop.
func TestR11EnglishAndCancelRouting(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cont := []string{"Continue", "Keep going", "Go ahead", "Proceed", "Carry on", "继续", "接着弄"}
	for _, s := range cont {
		if it := c.ClassifyTask(s); it.Intent != contract.IntentContinue {
			t.Errorf("%q: 应判 CONTINUE，实际 %s", s, it.Intent)
		}
	}
	status := []string{"Are you done?", "What's the status?", "Is it finished?", "做完了吗", "那封回复了没"}
	for _, s := range status {
		if it := c.ClassifyTask(s); it.Intent != contract.IntentQuery {
			t.Errorf("%q: 应判 QUERY，实际 %s", s, it.Intent)
		}
	}
	cancel := []string{"取消", "算了", "算了不装了", "别发了", "别删了", "不用了", "stop", "cancel", "别管那个删除操作"}
	for _, s := range cancel {
		if it := c.ClassifyTask(s); it.Intent != contract.IntentCancel {
			t.Errorf("%q: 应判 CANCEL，实际 %s", s, it.Intent)
		}
	}
	// negation-with-ask stays for non-cancel negative commands (must not be CANCEL)
	keepAsk := []string{"请勿删除那个文件", "无需提交", "切勿修改配置"}
	for _, s := range keepAsk {
		if it := c.ClassifyTask(s); it.Intent != contract.IntentAsk || it.Conflict != contract.ConflictNegation {
			t.Errorf("%q: 应保持否定+回问 ASK，实际 intent=%s conflict=%q", s, it.Intent, it.Conflict)
		}
	}
	// question forms containing cancel words must not become CANCEL
	noCancel := []string{"要不要取消这个任务？", "还能不能继续"}
	for _, s := range noCancel {
		if it := c.ClassifyTask(s); it.Intent == contract.IntentCancel {
			t.Errorf("%q: 疑问句不应判 CANCEL，实际 %s", s, it.Intent)
		}
	}
}
