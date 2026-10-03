// extractPath 的中文引号回归测试。
//
// 背景：G8 修复的独立评审（REVIEWER）质疑"用中文弯引号包裹答案能否被抽出对象"。
// 追下去发现比质疑更严重：extractPath 用 `text[i+1 : i+1+j]` 切引号内容，
// 而中文引号是多字节 UTF-8（“ 占 3 字节），于是
//
//	打开“报告.docx”  →  取出 "\x80\x9c报告.docx"
//
// 开引号的第 2、3 字节被留在了结果里。任何"用中文引号说出路径"的用户都会中招。
package input

import "testing"

func TestExtractPathChineseQuotesAreNotCorrupted(t *testing.T) {
	cases := []struct{ text, want string }{
		{"打开“报告.docx”", "报告.docx"},
		{"打开「报价单.pdf」", "报价单.pdf"},
		{`打开"report.md"`, "report.md"}, // ASCII 双引号（原本就正常）
		{"读一下'note.txt'", "note.txt"},  // ASCII 单引号
	}
	for _, tc := range cases {
		got, found := extractPath(tc.text)
		if !found {
			t.Errorf("%q: 应找到路径", tc.text)
			continue
		}
		if got != tc.want {
			t.Errorf("%q: 取出 %q，期望 %q（多字节引号切分错误）", tc.text, got, tc.want)
		}
	}
}

func TestExtractPathNonPathStillNotFound(t *testing.T) {
	if _, found := extractPath("把这个改一下"); found {
		t.Error("无路径的句子不应抽出路径")
	}
}

// 端到端：中文引号里的对象要能进 EDIT 的 object 槽位。
func TestClassifyEditExtractsChineseQuotedObject(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("把“main.go”改一下")
	if got.Intent != "EDIT" {
		t.Fatalf("意图 = %q，期望 EDIT", got.Intent)
	}
	if got.Params["object"] != "main.go" {
		t.Fatalf("object = %q，期望 main.go", got.Params["object"])
	}
}
