// G8 fix      :     referout  boundary(S2  char word  , S3  id be get, 
// S4    id path  , S7 empty  ). 
//
//   disconnectlang  value at:   is**   out,  now afterpatchon** , 
//  by   allto     bereferout   , but is after num overwriterate. 
package server

import (
	"strings"
	"testing"

	"voicesign-harness/input"
)

// TestResolveClarifiedSubstitutesAnaphora basebase  . 
func TestResolveClarifiedSubstitutesAnaphora(t *testing.T) {
	got := resolveClarified("把这个改一下", "main.go")
	if got != "把“main.go”改一下" {
		t.Fatalf("got %q", got)
	}
	//  word first:    in"  "but under"file"
	got = resolveClarified("把这个文件改一下", "a.md")
	if got != "把“a.md”改一下" {
		t.Fatalf("长词优先失效: %q", got)
	}
}

// TestResolveClarifiedDoesNotMangleWords    S2:   char word    word   split. 
func TestResolveClarifiedDoesNotMangleWords(t *testing.T) {
	cases := []struct{ text, want string }{
		{"把其它文件改一下", "把其它文件改一下"}, // " "  "its " 
		{"由它去吧", "由它去吧"},         // " "  "by " 
		{"把其他文件改一下", "把其他文件改一下"}, // base then is" "
	}
	for _, tc := range cases {
		if got := resolveClarified(tc.text, "main.go"); got != tc.want {
			t.Errorf("%q → %q，期望不被改写 %q", tc.text, got, tc.want)
		}
	}
	//  pos  word  be  
	if got := resolveClarified("打开它", "a.md"); got != "打开“a.md”" {
		t.Errorf("真正的代词应被替换，got %q", got)
	}
}

// TestResolveClarifiedExtractsObject    S3:   " id  after get   out formto "    . 
// if dayclassify   get  modify( again in   id),     . 
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

// TestStripOptionPrefix    S4:    id before   ,   path  . 
func TestStripOptionPrefix(t *testing.T) {
	for in, want := range map[string]string{
		"dict:小林":    "小林",
		"rec:a.md":   "a.md",
		"  dict:x  ": "x",
		"plain text": "plain text",
		"DICT:upper": "DICT:upper", // before   write  ,     
	} {
		if got := stripOptionPrefix(in); got != want {
			t.Errorf("stripOptionPrefix(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestResolveClarifiedEmptyAnswer empty   modifywriteorig (by resumeAsk responsiblereject). 
func TestResolveClarifiedEmptyAnswer(t *testing.T) {
	if got := resolveClarified("把这个改一下", "   "); got != "把这个改一下" {
		t.Fatalf("空答案不应改写原文，got %q", got)
	}
}

// TestPronounIndexBoundary  connect  pronounIndex   boundary as. 
func TestPronounIndexBoundary(t *testing.T) {
	if i := pronounIndex("其它", "它"); i >= 0 {
		t.Error("“其它”里的“它”不应被当作代词")
	}
	if i := pronounIndex("打开它", "它"); i < 0 {
		t.Error("“打开它”里的“它”应被识别为代词")
	}
	//  charword     
	if i := pronounIndex("把那个文件改一下", "那个文件"); !strings.HasPrefix("把那个文件改一下"[i:], "那个文件") {
		t.Error("多字词匹配位置不正确")
	}
}
