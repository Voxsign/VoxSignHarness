// 本文件是 builder 的回归保护，不属于 stubsmith 的判据桩；判据桩见 e2e/arch_test.go。
//
// 背景：架构缺口 A1（决策 #7 自举入口）——「注册一个命令，用来压缩图片」曾被判 UNKNOWN，
// 根因是把"注册工具意图"实现成了对固定短语的词表匹配（加一个工具/注册工具/新增工具…）。
// 修复把判据换成结构性条件（注册动词 + 量化虚词 + 能力名词，见 taskintent.go
// registerToolRequest），本文件锁住该修复的**两个方向**，防止回退：
//
//  1. 正向：同一注册意图的不同措辞必须都判 REGISTER_TOOL（用户不迁就系统词表）；
//  2. 反向：含「注册/工具/命令」字样但语义不是注册的句子，不得被误判 ——
//     尤其是「查一下注册表」必须仍是 QUERY、「把提交按钮改成中文」必须仍是 EDIT。
//
// 本文件只覆盖本次修复的正/反例，不承担完整判据体系（那是 stubsmith 的职责）。
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestRegisterToolStructuralHits 锁定：注册动词支配能力名词的不同措辞都判 REGISTER_TOOL。
func TestRegisterToolStructuralHits(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []string{
		// 任务书验收三句。
		"加一个工具把 md 转 pdf",
		"给我加个工具，能把 markdown 转成 pdf",
		"注册一个命令，用来压缩图片",
		// 同一结构特征的其它措辞（证明判据是结构而不是固定词表）。
		"新增一个技能",
		"添加个插件",
		"创建一个脚本",
		"新建一个命令",
		"加个新工具",
	}
	for _, text := range cases {
		if got := c.ClassifyTask(text); got.Intent != contract.IntentRegisterTool {
			t.Errorf("注册意图未命中: %q 被判 %s（期望 REGISTER_TOOL）", text, got.Intent)
		}
	}
}

// TestRegisterToolNoFalsePositiveAmongLookalikes 锁定反向：
// 句子里出现「注册/工具/命令」不等于注册意图，正常 EDIT/QUERY/TEST 不得被抢走。
func TestRegisterToolNoFalsePositiveAmongLookalikes(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)

	// 必须保持精确类别的关键反例（判据桩 A1 的验收 2 直接依赖这两条）。
	exact := []struct{ text, want string }{
		{"查一下注册表", contract.IntentQuery},      // "注册"是名词的一部分，不是动词
		{"查一下已注册的工具", contract.IntentQuery},   // 动词与名词之间夹着"的" = 描述，不是注册
		{"把提交按钮改成中文", contract.IntentEdit},    // 含"提交"但不是 COMMIT/REGISTER_TOOL
		{"把命令改成中文", contract.IntentEdit},      // 含"命令"但动作是 EDIT
		{"把那个工具的说明改成中文", contract.IntentEdit}, // 含"工具"但动作是 EDIT
		{"跑一下工具链的测试", contract.IntentTest},    // 含"工具链"但动作是 TEST
		{"删掉那个工具", contract.IntentEdit},       // 含"工具"但动作是删除仲裁
	}
	for _, tc := range exact {
		if got := c.ClassifyTask(tc.text); got.Intent != tc.want {
			t.Errorf("误判: %q 被判 %s（期望 %s）", tc.text, got.Intent, tc.want)
		}
	}

	// 只要求"不是 REGISTER_TOOL"的单动作句（类别由其它既有判据决定，本文件不越权断言）。
	notRegister := []string{
		"添加一个注释",    // "注释"不是能力名词（旧词表也不该命中）
		"增加一个测试",    // 动作是 TEST
		"加个说明文档",    // 加的是文档，不是可注册能力
		"给这个报告加个图表", // 加的是内容
		"参加一个工具培训",  // "加"只是"参加"的一部分
	}
	for _, text := range notRegister {
		if got := c.ClassifyTask(text); got.Intent == contract.IntentRegisterTool {
			t.Errorf("误判: %q 被判 REGISTER_TOOL，但它不是注册指令", text)
		}
	}
}
