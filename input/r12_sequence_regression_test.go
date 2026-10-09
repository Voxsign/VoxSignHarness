package input

import (
	"strings"
	"testing"

	"voicesign-harness/contract"
)

// R12 (2026-10-09): ordered multi-task chains and interruption-then routing.
func TestR12MultiTaskRouting(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	seq := []string{
		"先装 codex 再处理邮件",
		"先下载代码，然后编译测试",
		"先看 SFDA 邮件，接着写回复",
		"同时处理邮件和安装 codex",
		"帮我看看邮件，顺便翻译那个 PDF",
		"先提交 A 再提交 B",
	}
	for _, s := range seq {
		it := c.ClassifyTask(s)
		if it.Intent != contract.IntentSequence {
			t.Errorf("%q: 应判 SEQUENCE，实际 %s", s, it.Intent)
			continue
		}
		if n := len(strings.Split(it.Params["steps"], "\n")); n < 2 {
			t.Errorf("%q: SEQUENCE steps < 2（%d）", s, n)
		}
	}
	// single-action "先 X" without a follow-on stays the plain intent
	if it := c.ClassifyTask("先看 SFDA 邮件"); it.Intent != contract.IntentEmail {
		t.Errorf("单独「先看 SFDA 邮件」应保持 EMAIL，实际 %s", it.Intent)
	}
	// plan talk without actionable steps never becomes a chain
	if it := c.ClassifyTask("先这样再那样"); it.Intent == contract.IntentSequence {
		t.Errorf("「先这样再那样」不应判 SEQUENCE")
	}
	// document orchestration is one job, not a chain
	orch := "把全部沟通记录和设计文档整理成《VoiceSign Harness 全景开发文档》并保存提交"
	if it := c.ClassifyTask(orch); it.Intent != contract.IntentOrchestrate {
		t.Errorf("文档编排句应判 ORCHESTRATE，实际 %s", it.Intent)
	}
}

func TestR12InterruptionThen(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, s := range []string{"先别管安装，先看邮件", "别管那个删除操作，先看看 SFDA 邮件"} {
		it := c.ClassifyTask(s)
		if it.Intent != contract.IntentCancel {
			t.Errorf("%q: 应判 CANCEL，实际 %s", s, it.Intent)
		}
		if it.Params["then"] == "" {
			t.Errorf("%q: CANCEL 必须带 then 后续动作", s)
		}
	}
	// plain cancel carries no then
	if it := c.ClassifyTask("取消"); it.Params["then"] != "" {
		t.Errorf("纯「取消」不应带 then，实际 %q", it.Params["then"])
	}
}
