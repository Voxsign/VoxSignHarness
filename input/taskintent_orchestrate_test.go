package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestDetectOrchestrate:    task diff +  be NOTE/COMMIT  classtriggersend  . 
func TestDetectOrchestrate(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct {
		name string
		text string
		want string
	}{
		{"用户原句", "把全部沟通记录和设计文档整理成《VoiceSign Harness 全景开发文档》并保存提交", contract.IntentOrchestrate},
		{"无书名号", "整理所有设计文档并提交", contract.IntentOrchestrate},
		{"仅整理无提交", "整理一下会议记录", contract.IntentNote}, // nokeepstore/  signal ->  isorchestrate
		{"普通记录", "记一下设计文档要点", contract.IntentNote},
		{"纯提交", "提交所有改动", contract.IntentCommit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.ClassifyTask(tc.text).Intent
			if got != tc.want {
				t.Fatalf("text=%q: got %q want %q", tc.text, got, tc.want)
			}
		})
	}
}

// TestDetectOrchestrateBookTitle:  nameidobjtgt   get. 
func TestDetectOrchestrateBookTitle(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	it := c.ClassifyTask("把沟通记录整理成《全景开发文档 v1》并保存提交")
	if it.Intent != contract.IntentOrchestrate {
		t.Fatalf("应命中 ORCHESTRATE, got %q", it.Intent)
	}
	if it.Params["target_doc"] != "全景开发文档 v1" {
		t.Fatalf("书名号抽取错误: %q", it.Params["target_doc"])
	}
	// orchestraterecvtail    -> baseline reversible. 
	if it.Risk == nil || it.Risk.Reversible {
		t.Fatal("ORCHESTRATE 基线应不可逆（收尾 git commit）")
	}
}
