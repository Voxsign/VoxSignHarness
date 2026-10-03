package provider

// truncate_fix_test.go —— F8 修复回归：截断按 rune 切，不得切裂多字节字符（非法 UTF-8）。
// 2026-10-03 真实测试 R15 输出流出现非法字节导致下游解析崩溃。

import "testing"

func TestTruncateRespRuneSafe(t *testing.T) {
	// 「六层架构」= 12 字节 4 个 rune；n=5 若按字节切会切裂最后一个中文字符。
	s := "六层架构目标"
	got := truncateResp(s, 5)
	for _, r := range got {
		if r == '\uFFFD' {
			t.Fatalf("F8 truncateResp 产生替换字符: %q", got)
		}
	}
	if want := "六层架构目...(truncated)"; got != want { // n=5 按 rune 保留 5 个字符
		t.Errorf("truncateResp = %q, want %q", got, want)
	}
	// 短文本不截断。
	if got := truncateResp(s, 20); got != s {
		t.Errorf("truncateResp 短文本被误截: %q", got)
	}
}
