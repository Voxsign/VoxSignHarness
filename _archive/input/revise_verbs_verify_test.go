package input

import "testing"

// 洞2 验证：修订类表述必须命中 implement（带实现域信号），防"修订文档"误判。
func TestReviseVerbsHitImplement(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"根据验收反馈修订 ASR 后台服务并补齐缺失端点", true},
		{"把缺失的 /v1/blacklist 端点补齐", true},
		{"修订这份文档", false}, // 无实现域信号 → 不误判
		{"把实现计划修订一下并继续实现提交", true},
		{"帮我完善一下这个代码", true},
		{"把这份需求说明书实现出来并提交", true}, // 原路径不回退
		{"根据反馈修复编译问题并跑通", true},
	}
	for _, c := range cases {
		_, _, ok := detectImplementOrchestrate(c.text)
		if ok != c.want {
			t.Errorf("text=%q 期望 implement=%v 实得 %v", c.text, c.want, ok)
		}
	}
}
