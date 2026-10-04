package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestDetectImplDispatch（2026-10-04 洞2 分发级验证）：实现类长任务必须由 Detect 分发
// 到 ORCHESTRATE(kind=implement)，而不是被 2b 的 commitTriggers 截成单动作 COMMIT。
func TestDetectImplDispatch(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	text := "把这份需求文档实现出来，生成可运行的 Go 后台服务并提交到仓库"
	got := c.ClassifyTask(text)
	if got.Intent != contract.IntentOrchestrate {
		t.Fatalf("期望 ORCHESTRATE，实得 %q（ask=%q）", got.Intent, got.Ask)
	}
	if got.Params == nil || got.Params["kind"] != "implement" {
		t.Fatalf("期望 kind=implement，实得 params=%v", got.Params)
	}
	t.Logf("PASS：%q → ORCHESTRATE(kind=implement)", text)
}

// TestDetectImplNotCommit：纯提交指令仍走 COMMIT，不被实现类抢走。
func TestDetectImplNotCommit(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("把这份文档提交到仓库")
	if got.Intent == contract.IntentOrchestrate {
		t.Fatalf("纯提交不应判实现类：%q", got.Intent)
	}
}
