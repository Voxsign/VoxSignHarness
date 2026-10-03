package input

import (
	"testing"

	"voicesign-harness/contract"
)

// 线 C 修订卡回归：技能调用识别（正向）+ 不误判（反向）。
func TestSkillInvocation(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct {
		name string
		text string
		want string // 期望 Intent；"" 表示不应命中技能类
	}{
		// —— 正向：技能调用 ——
		{"调用技能做校准", "调用校验与对齐技能，对 VoiceSign ASR 做一次实证校准，输出校准报告并提交", contract.IntentOrchestrate},
		{"use validate-align", "使用 validate-align 技能对验收标准做对齐检查", contract.IntentOrchestrate},
		{"技能名+动作域", "按校验与对齐技能，对 ASR 做校准", contract.IntentOrchestrate},
		{"调用技能校准", "调用技能，对 X 做一次校准，输出报告", contract.IntentOrchestrate},
		{"技能检索", "使用技能对材料做一次检索总结", contract.IntentOrchestrate},
		// —— 反向：不得误判 ——
		{"问句排除", "怎么调用校验技能做校准？", ""},
		{"实现类含技能名词", "实现一个使用技能的推荐系统", ""}, // 无动作域词 → 不走技能
		{"实现类", "实现个性化 ASR 后台服务", ""},               // 无"技能" → 不走技能
		{"普通文档整理", "把沟通记录整理成文档并保存提交", ""},
		{"普通询问", "这个技能是干什么的", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ti := c.ClassifyTask(tc.text)
			got := ti.Intent
			if tc.want == "" {
				if got == contract.IntentOrchestrate {
					// 允许非技能类的其他意图，但不得是技能调用
					if ti.Params != nil && ti.Params["kind"] == "skill" {
						t.Fatalf("误判为技能调用: text=%q intent=%v params=%v", tc.text, got, ti.Params)
					}
				}
				return
			}
			if got != tc.want {
				t.Fatalf("意图不匹配: text=%q got=%v want=%v params=%v", tc.text, got, tc.want, ti.Params)
			}
			if ti.Params == nil || ti.Params["kind"] != "skill" {
				t.Fatalf("技能调用应带 kind=skill: params=%v", ti.Params)
			}
		})
	}
}
