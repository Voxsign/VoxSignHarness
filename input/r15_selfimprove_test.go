package input

import "testing"

// R15/v0.6.0: an open-ended "research a reference then improve your own backend"
// long task must classify to SELF_IMPROVE (and therefore reach the ReAct loop),
// never be shredded into SEQUENCE.
func TestDetectSelfImprove(t *testing.T) {
	hit := []string{
		"做一个长期任务：研究这台电脑上的 Claude Code 和 Codex 是怎么规划和执行任务的，学习它们的长任务执行方式，然后改进你自己的后端。步骤：先看 ~/.codex 和 ~/.claude 的配置，提炼改进点，在 pipeline 代码里实现一个，编译测试通过",
		"蒸馏 claude code 的执行范式来改进你的代码",
		"研究 codex 怎么干活，然后优化你自己的 pipeline",
		"learn from claude code and improve your own backend",
		"你自己改自己：参考 codex 的长任务方式升级你的后端",
	}
	for _, s := range hit {
		if _, ok := detectSelfImprove(s); !ok {
			t.Errorf("expected SELF_IMPROVE detection: %q", s)
		}
	}
	miss := []string{
		"先下载那个代码再编译测试",          // BUILD_TEST / SEQUENCE, not self-improve
		"帮我安装 codex 到后台",            // INSTALL
		"北京天气怎么样",                // QUERY
		"研究一下这个方案，给我讲讲",           // research without self-reference -> chat/query
		"把笔记里的 A 改成 B",             // EDIT
	}
	for _, s := range miss {
		if _, ok := detectSelfImprove(s); ok {
			t.Errorf("should NOT detect SELF_IMPROVE: %q", s)
		}
	}
}

// End-to-end classification: the user's real R15 prompt must win over the
// multi-task splitter.
func TestClassifyTaskSelfImproveBeatsSequence(t *testing.T) {
	c := NewTaskClassifier(0, nil)
	text := "做一个长期任务：研究 Claude Code 和 Codex 怎么执行任务，学习后改进你自己的后端。步骤：先看配置，提炼3个改进点，在 pipeline 里实现一个，编译测试，最后汇报"
	got := c.ClassifyTask(text)
	if got.Intent != "SELF_IMPROVE" {
		t.Fatalf("intent=%q want SELF_IMPROVE", got.Intent)
	}
}
