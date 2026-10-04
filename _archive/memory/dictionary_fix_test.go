package memory

// dictionary_fix_test.go —— F7 修复回归：ASR 噪声「in 死 cope」→「In scope」。
// 2026-10-03 真实测试 R8：不纠错则整句 UNKNOWN 回问。

import (
	"strings"
	"testing"
)

func TestBuiltinCorrectionInScope(t *testing.T) {
	d := builtinDictionary()
	for _, variant := range []string{"in 死 cope", "因死 cope", "因斯科普"} {
		text := "把这个方案里的 " + variant + " 范围整成文档"
		got, corrs := d.Correct(text)
		if !strings.Contains(got, "In scope") {
			t.Errorf("F7 变体 %q 未纠正: %q", variant, got)
		}
		if len(corrs) == 0 {
			t.Errorf("F7 变体 %q 未产生纠错记录", variant)
		}
		for _, c := range corrs {
			if c.Rule != "dict" || c.To != "In scope" {
				t.Errorf("F7 纠错记录异常: %+v", c)
			}
		}
	}
}
