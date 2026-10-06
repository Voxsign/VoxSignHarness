package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestDetectImplDispatch(2026-10-04  2 splitsend   ):  nowclass task  by Detect splitsend
// to ORCHESTRATE(kind=implement), but isbe 2b   commitTriggers  become    COMMIT. 
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

// TestDetectImplNotCommit:    refer    COMMIT,  be nowclass  . 
func TestDetectImplNotCommit(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("把这份文档提交到仓库")
	if got.Intent == contract.IntentOrchestrate {
		t.Fatalf("纯提交不应判实现类：%q", got.Intent)
	}
}
