package pipeline

// truncate_fix_test.go —— F8 修复回归（同 provider）：轨迹/回执截断按 rune 切。

import "testing"

func TestTruncateStrRuneSafe(t *testing.T) {
	s := "六层架构目标"
	got := truncateStr(s, 5)
	for _, r := range got {
		if r == '\uFFFD' {
			t.Fatalf("F8 truncateStr 产生替换字符: %q", got)
		}
	}
	if want := "六层架构目…"; got != want { // n=5 按 rune 保留 5 个字符
		t.Errorf("truncateStr = %q, want %q", got, want)
	}
	if got := truncateStr(s, 20); got != s {
		t.Errorf("truncateStr 短文本被误截: %q", got)
	}
}
