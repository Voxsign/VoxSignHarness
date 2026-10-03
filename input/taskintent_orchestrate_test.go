package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestDetectOrchestrate：复合长任务识别 + 不被 NOTE/COMMIT 单类触发压扁。
func TestDetectOrchestrate(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct {
		name string
		text string
		want string
	}{
		{"用户原句", "把全部沟通记录和设计文档整理成《VoiceSign Harness 全景开发文档》并保存提交", contract.IntentOrchestrate},
		{"无书名号", "整理所有设计文档并提交", contract.IntentOrchestrate},
		{"仅整理无提交", "整理一下会议记录", contract.IntentNote}, // 无保存/提交信号 → 不是编排
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

// TestDetectOrchestrateBookTitle：书名号目标文档抽取。
func TestDetectOrchestrateBookTitle(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	it := c.ClassifyTask("把沟通记录整理成《全景开发文档 v1》并保存提交")
	if it.Intent != contract.IntentOrchestrate {
		t.Fatalf("应命中 ORCHESTRATE, got %q", it.Intent)
	}
	if it.Params["target_doc"] != "全景开发文档 v1" {
		t.Fatalf("书名号抽取错误: %q", it.Params["target_doc"])
	}
	// 编排收尾含提交 → 基线不可逆。
	if it.Risk == nil || it.Risk.Reversible {
		t.Fatal("ORCHESTRATE 基线应不可逆（收尾 git commit）")
	}
}

// TestDetectOrchestrateCreateDoc（缺陷修复回归）：「生成《X》文档并保存提交」类任务
// 必须判 ORCHESTRATE，不得因标题含"测试"被 TEST 触发词抢先误判。
// 复现：2026-10-04 实测「生成《测试-打断语义》文档并保存提交」被判 TEST → space_check 拒绝 → 秒级终态。
func TestDetectOrchestrateCreateDoc(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct {
		name string
		text string
		want string
	}{
		{"复现用例·标题含测试", "生成《测试-打断语义》文档并保存提交", contract.IntentOrchestrate},
		{"生成式·无提交", "生成《iOS客户端-后台能力设计》文档", contract.IntentOrchestrate},
		{"写成·带书名号", "把设计要点写成《后端设计 v2》文档", contract.IntentOrchestrate},
		{"产出·文档语境", "产出《周报》文档并提交", contract.IntentOrchestrate},
		{"纯测试不回归", "跑一下测试", contract.IntentTest},
		{"测试指定包不回归", "执行测试这个函数", contract.IntentTest},
		{"普通编辑不误伤", "把《文档》标题改成中文", contract.IntentEdit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.ClassifyTask(tc.text).Intent
			if got != tc.want {
				t.Fatalf("text=%q: got %q want %q", tc.text, got, tc.want)
			}
		})
	}
	// 书名号目标必须抽准（生成式路径 2 的 target_doc）。
	it := c.ClassifyTask("生成《测试-打断语义》文档并保存提交")
	if it.Params["target_doc"] != "测试-打断语义" {
		t.Fatalf("生成式路径书名号抽取错误: %q", it.Params["target_doc"])
	}
}
