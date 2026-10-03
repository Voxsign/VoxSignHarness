// 否定仲裁的**评审补充**测试（缺口 G1 · 独立评审第二轮）。
//
// 评审 P1/P2 指出的两类漏洞，本文件各钉一组：
//
//	P1 该收未收：勿/请勿/切勿/无需/不再/免了，以及裸"别"后随的去/管/乱/忘
//	   —— 漏了这些，"请勿删除"仍会被判成可执行删除，G1 就没闭合。
//	P2 误伤（安全侧）：要不要/不要紧 里的"不要"不是否定
//	   —— 误判会让正常指令凭空弹出确认，把"帮我删除"变成"确认不执行吗"。
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestNegationCoversReviewP1Markers 评审 P1：补齐后的否定词必须都生效。
func TestNegationCoversReviewP1Markers(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []string{
		"请勿删除那个文件",
		"勿删",
		"无需提交",
		"不再部署到服务器",
		"切勿修改配置",
		"提交就免了",
		"别去删除那个文件",
		"别管那个删除操作",
	}
	for _, text := range cases {
		got := c.ClassifyTask(text)
		if got.Conflict != contract.ConflictNegation {
			t.Errorf("P1 未闭合: %q 未被识别为否定（conflict=%q intent=%s）",
				text, got.Conflict, got.Intent)
		}
		if got.Ask == "" {
			t.Errorf("%q: 否定句必须回问（Ask != '' 才保证绝不执行）", text)
		}
		if got.Intent == contract.IntentEdit && got.Params["action"] == "delete" {
			t.Errorf("%q 仍被判为可执行删除 —— G1 未闭合", text)
		}
	}
}

// TestNegationReviewP2FalsePositives 评审 P2：形似否定、实非否定。
func TestNegationReviewP2FalsePositives(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct{ text, why string }{
		{"要不要删除那个文件", "「不要」只是「要不要」的一部分"},
		{"不要紧，帮我删除它", "「不要紧」= 没关系，整句是请我删除"},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Conflict == contract.ConflictNegation {
			t.Errorf("P2 误伤: %q 被误判否定（%s）→ intent=%s",
				tc.text, tc.why, got.Intent)
		}
	}
	// 「不要紧，帮我删除它」应真的按删除处理（这是用户的真实请求）
	got := c.ClassifyTask("不要紧，帮我删除它")
	if got.Intent != contract.IntentEdit {
		t.Errorf("「不要紧，帮我删除它」意图 = %q，期望 EDIT（用户确实要求删除）", got.Intent)
	}

	// 单测 hasNegation 层面
	for _, text := range []string{"要不要删除", "不要紧"} {
		if _, _, hit := hasNegation(text); hit {
			t.Errorf("hasNegation(%q) 应为 false（形似否定）", text)
		}
	}
	for _, text := range []string{"请勿删除", "无需提交", "不再部署", "免了", "别去删除"} {
		if _, _, hit := hasNegation(text); !hit {
			t.Errorf("hasNegation(%q) 应为 true（P1 补词）", text)
		}
	}
}

// TestNegationMarkerPrecedence 长词优先：不要再 / 不需要 不应被 不要 抢先回显。
func TestNegationMarkerPrecedence(t *testing.T) {
	if mk, _, _ := hasNegation("不要再部署"); mk != "不要再" {
		t.Errorf("回显 = %q，期望「不要再」（长词优先，评审 P6）", mk)
	}
	if mk, _, _ := hasNegation("不需要提交"); mk != "不需要" {
		t.Errorf("回显 = %q，期望「不需要」", mk)
	}
}
