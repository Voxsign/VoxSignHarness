package input

// realreq_fix_regression_test.go —— 2026-10-03 真实用户需求（National Grid SA · OT-ODP
// 技术提案）实跑发现 F1–F7 的回归测试。语料与逐条结果见
// docs/测试报告-真实用户需求-NationalGridSA-OT-ODP-2026-10-03.md。
// 判据：修复后的意图必须稳定；安全行为（否定/多动作摊开）不得回归。

import (
	"strings"
	"testing"

	"voicesign-harness/contract"
)

// noSpaceClassifier 与生产 `vhs task` 同构：无空间提示（沙箱空域），
// 依赖 builtin 空间名词守卫（F3 修复）而非 SpaceHint 别名。
func noSpaceClassifier() *TaskClassifier {
	return NewTaskClassifier(0.6, nil)
}

// TestF1FactualQuestionNotDeploy 真实测试 R2：事实疑问句不因「发布」误判 DEPLOY。
func TestF1FactualQuestionNotDeploy(t *testing.T) {
	c := noSpaceClassifier()
	cases := []struct {
		text string
		want string
	}{
		{"这个方案里 DMZ 发布是不是单向的", contract.IntentQuery},   // R2：原是 DEPLOY
		{"OT-ODP 的六层目标架构分别是哪六层", contract.IntentQuery}, // R1：原是 UNKNOWN
		{"S1 振荡场景的降级模式 OT 边界是什么", contract.IntentQuery}, // R9：原是 UNKNOWN
		{"我上次问的 OT-ODP 定位和范围是什么", contract.IntentQuery}, // R15：保持 QUERY
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.want {
			t.Errorf("F1 %q → %s, want %s", tc.text, got.Intent, tc.want)
		}
		if got.Ask != "" {
			t.Errorf("F1 %q 不应回问, Ask=%q", tc.text, got.Ask)
		}
	}
}

// TestF1LongNarrativeOrchestrate 真实测试 R10：长口语真实需求（建平台+收数据+发布+AI建议）
// 不再被「发布」误判 DEPLOY，落入实现类编排。
func TestF1LongNarrativeOrchestrate(t *testing.T) {
	c := noSpaceClassifier()
	text := "我们要为国家电网南非公司建一个 OT 运营数据平台，把 SCADA EMS WAMS 这些源系统的数据统一收上来做受治理的数据产品，通过 DMZ 单向发布给企业，AI 只给建议动作要人工批准，帮我规划一下这件事"
	got := c.ClassifyTask(text)
	if got.Intent != contract.IntentOrchestrate {
		t.Fatalf("R10 → %s, want ORCHESTRATE", got.Intent)
	}
	if got.Params == nil || got.Params["kind"] != "implement" {
		t.Errorf("R10 params 缺 kind=implement: %v", got.Params)
	}
}

// TestF2InfoRoute 真实测试「翻译一下：采集安全六支柱是什么」→ INFO（接通配置 INFO 路由）。
func TestF2InfoRoute(t *testing.T) {
	c := noSpaceClassifier()
	for _, text := range []string{
		"翻译一下：采集安全六支柱是什么",
		"总结一下这段方案",
		"把这段话做个摘要",
	} {
		got := c.ClassifyTask(text)
		if got.Intent != contract.IntentInfo {
			t.Errorf("F2 %q → %s, want INFO", text, got.Intent)
		}
	}
	// INFO 不得因槽位闸回问（LLM 直答路径）。
	if got := c.ClassifyTask("翻译一下：采集安全六支柱是什么"); got.Ask != "" {
		t.Errorf("F2 INFO 不应回问, Ask=%q", got.Ask)
	}
}

// TestF3SpaceNounShadowsThought 真实测试 vhs task 3 条失败（无空间提示时）：
// 「想法库」是实体名，不是"记想法"触发。
func TestF3SpaceNounShadowsThought(t *testing.T) {
	c := noSpaceClassifier() // 关键：无 SpaceHint，验证 builtin 守卫
	cases := []struct {
		text string
		want string
	}{
		{"把想法库那几个旧标签改成一个新标签", contract.IntentEdit},
		{"上次那个想法库里面记的报价客户是哪家", contract.IntentQuery},
		{"把想法库的内容发到外部 markdown 文件", contract.IntentDeploy},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.want {
			t.Errorf("F3 %q → %s, want %s", tc.text, got.Intent, tc.want)
		}
	}
	// 真想法句不回归：记一个想法 → NOTE；发个想法 → NOTE。
	for _, text := range []string{"记一个想法：AI 输出只能给建议", "发个想法"} {
		if got := c.ClassifyTask(text); got.Intent != contract.IntentNote {
			t.Errorf("F3 回归 %q → %s, want NOTE", text, got.Intent)
		}
	}
}

// TestF5MultiActionThoughtNotSwallowed 真实测试 R13：想法句含多动作连接 → 摊开回问，
// 不静默吞掉「提交」。
func TestF5MultiActionThoughtNotSwallowed(t *testing.T) {
	c := noSpaceClassifier()
	got := c.ClassifyTask("把采集安全六支柱记下来然后再提交一个想法")
	if got.Intent != contract.IntentAsk {
		t.Fatalf("R13 → %s, want ASK（多动作摊开）", got.Intent)
	}
	if got.Conflict != contract.ConflictMultiAction {
		t.Errorf("R13 Conflict=%s, want multi_action", got.Conflict)
	}
	if !strings.Contains(got.Ask, "两件以上") {
		t.Errorf("R13 Ask 文案异常: %q", got.Ask)
	}
}

// TestF6DeadlineAsNote 真实测试 R14：带时间锚点的截止/待办 → NOTE 且时间进 params。
func TestF6DeadlineAsNote(t *testing.T) {
	c := noSpaceClassifier()
	got := c.ClassifyTask("下周三之前完成方案评审")
	if got.Intent != contract.IntentNote {
		t.Fatalf("R14 → %s, want NOTE（待办）", got.Intent)
	}
	if got.Params == nil || got.Params["time_hint"] == "" {
		t.Errorf("R14 时间锚点未进 params: %v", got.Params)
	}
}

// TestR8CorrectedEdit 真实测试 R8（纠错后）：In scope 范围整成文档 → EDIT。
// 纠错本身见 memory/dictionary_fix_test.go（F7）。
func TestR8CorrectedEdit(t *testing.T) {
	c := noSpaceClassifier()
	got := c.ClassifyTask("把这个方案里的 In scope 范围整成文档")
	if got.Intent != contract.IntentEdit {
		t.Errorf("R8(纠错后) → %s, want EDIT", got.Intent)
	}
}

// TestR5OrchestrateDocSave 真实测试 R5：「整理成…文档放到 docs」→ 编排（三连信号）。
func TestR5OrchestrateDocSave(t *testing.T) {
	c := noSpaceClassifier()
	got := c.ClassifyTask("把技术方案里 In scope 的范围整理成一份 markdown 文档放到 docs 目录")
	if got.Intent != contract.IntentOrchestrate {
		t.Errorf("R5 → %s, want ORCHESTRATE", got.Intent)
	}
}

// TestSafetyNoRegression 安全行为回归：否定/元指令/状态问句不被新触发词破坏。
func TestSafetyNoRegression(t *testing.T) {
	c := noSpaceClassifier()
	cases := []struct {
		text string
		want string
	}{
		{"不要删除外部系统数据贡献表", contract.IntentAsk}, // R11 否定拦截
		{"开始测试方案里 Discover 到 Scale 的五个交付阶段", contract.IntentTest}, // R12 元指令后的真测试
		{"把这次的文档改动提交一下", contract.IntentCommit}, // R7 保持 COMMIT
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.want {
			t.Errorf("安全回归 %q → %s, want %s", tc.text, got.Intent, tc.want)
		}
	}
}
