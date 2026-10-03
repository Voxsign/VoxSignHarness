// G8 修复的单元测试：锁定评审指出的边界（S2 单字代词误伤、S3 引号可被抽取、
// S4 候选 id 路径一致、S7 空答案）。
//
// 这些断言的价值在于：它们是**评审提出、实现随后补上**的，
// 所以每一条都对应一个真实被指出的缺陷，而不是事后凑数的覆盖率。
package server

import (
	"strings"
	"testing"

	"voicesign-harness/input"
)

// TestResolveClarifiedSubstitutesAnaphora 基本替换。
func TestResolveClarifiedSubstitutesAnaphora(t *testing.T) {
	got := resolveClarified("把这个改一下", "main.go")
	if got != "把“main.go”改一下" {
		t.Fatalf("got %q", got)
	}
	// 长词优先：不应命中"这个"而留下"文件"
	got = resolveClarified("把这个文件改一下", "a.md")
	if got != "把“a.md”改一下" {
		t.Fatalf("长词优先失效: %q", got)
	}
}

// TestResolveClarifiedDoesNotMangleWords 评审 S2：裸单字代词不得误伤词的一部分。
func TestResolveClarifiedDoesNotMangleWords(t *testing.T) {
	cases := []struct{ text, want string }{
		{"把其它文件改一下", "把其它文件改一下"}, // "它" 在"其它"里
		{"由它去吧", "由它去吧"},         // "它" 在"由它"里
		{"把其他文件改一下", "把其他文件改一下"}, // 本来就不是"它"
	}
	for _, tc := range cases {
		if got := resolveClarified(tc.text, "main.go"); got != tc.want {
			t.Errorf("%q → %q，期望不被改写 %q", tc.text, got, tc.want)
		}
	}
	// 真正的代词仍应被替换
	if got := resolveClarified("打开它", "a.md"); got != "打开“a.md”" {
		t.Errorf("真正的代词应被替换，got %q", got)
	}
}

// TestResolveClarifiedExtractsObject 评审 S3：锁定"引号包裹后提取器能抽出显式对象"这一假设。
// 若哪天分类器的抽取逻辑改了（不再认中文弯引号），这条会红。
func TestResolveClarifiedExtractsObject(t *testing.T) {
	c := input.NewTaskClassifier(0.6, nil)
	clarified := resolveClarified("把这个改一下", "main.go")
	t.Logf("续跑文本 = %q", clarified)
	intent := c.ClassifyTask(clarified)
	if intent.Intent != "EDIT" {
		t.Fatalf("续跑文本应判 EDIT，实际 %q", intent.Intent)
	}
	if intent.Params["object"] != "main.go" {
		t.Fatalf("取出的 object = %q，期望 main.go —— 引号抽取假设不成立",
			intent.Params["object"])
	}
}

// TestStripOptionPrefix 评审 S4：候选 id 前缀剥离，两条路径一致。
func TestStripOptionPrefix(t *testing.T) {
	for in, want := range map[string]string{
		"dict:小林":    "小林",
		"rec:a.md":   "a.md",
		"  dict:x  ": "x",
		"plain text": "plain text",
		"DICT:upper": "DICT:upper", // 前缀大小写敏感，不做猜测
	} {
		if got := stripOptionPrefix(in); got != want {
			t.Errorf("stripOptionPrefix(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestResolveClarifiedEmptyAnswer 空答案不改写原文（由 resumeAsk 负责拒绝）。
func TestResolveClarifiedEmptyAnswer(t *testing.T) {
	if got := resolveClarified("把这个改一下", "   "); got != "把这个改一下" {
		t.Fatalf("空答案不应改写原文，got %q", got)
	}
}

// TestPronounIndexBoundary 直接锁 pronounIndex 的边界行为。
func TestPronounIndexBoundary(t *testing.T) {
	if i := pronounIndex("其它", "它"); i >= 0 {
		t.Error("“其它”里的“它”不应被当作代词")
	}
	if i := pronounIndex("打开它", "它"); i < 0 {
		t.Error("“打开它”里的“它”应被识别为代词")
	}
	// 多字词走精确匹配
	if i := pronounIndex("把那个文件改一下", "那个文件"); !strings.HasPrefix("把那个文件改一下"[i:], "那个文件") {
		t.Error("多字词匹配位置不正确")
	}
}
