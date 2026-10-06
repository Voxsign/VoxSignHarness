// extractPath  in  idback   . 
//
//  scenario: G8 fix      (REVIEWER)  "usein   id      be outto ". 
//  under sendnow   change heavy: extractPath use `text[i+1 : i+1+j]`   idin , 
// butin  idis charnode UTF-8(“   3 charnode), atis
//
//	 open“  .docx”  ->  getout "\x80\x9c  .docx"
//
// open id   2, 3 charnodebe  close  .   "usein  id outpath" useuserall in . 
package input

import "testing"

func TestExtractPathChineseQuotesAreNotCorrupted(t *testing.T) {
	cases := []struct{ text, want string }{
		{"打开“报告.docx”", "报告.docx"},
		{"打开「报价单.pdf」", "报价单.pdf"},
		{`打开"report.md"`, "report.md"}, // ASCII   id(origbasethenpos )
		{"读一下'note.txt'", "note.txt"},  // ASCII   id
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

// endtoend: in  id  to need   EDIT   object   . 
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
