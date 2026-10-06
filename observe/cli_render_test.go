package observe

import (
	"strings"
	"testing"
)

//   quiet:   event writecurbefore ; Finish after  back first--out  REPL   "> "  show 
// from   first  ,  be    char   . 
func TestCliRenderer_KeepsPrompt(t *testing.T) {
	var buf strings.Builder
	r := NewCliRenderer(&buf, false)

	r.OnEvent(LineEvent{Stage: "intent", Detail: "意图=note"})
	r.OnEvent(LineEvent{Stage: "exec", Detail: "写入中"})
	before := buf.String()
	if !strings.Contains(before, "\r\x1b[2K› exec 写入中") {
		t.Fatalf("滚动行应覆写打印 stage/detail, got %q", before)
	}
	r.Finish()

	// Finish   by   listclosetail -> out  show   raise 
	got := buf.String()
	if !strings.HasSuffix(got, "\r\x1b[2K") {
		t.Fatalf("Finish 应清行回行首, 输出后缀=%q", got[len(got)-10:])
	}
	//   out  REPL  after   show ,       first(no    char )
	buf.WriteString("> ")
	if !strings.Contains(buf.String(), "\r\x1b[2K> ") {
		t.Fatal("提示符应紧跟清行序列（不被滚动行污染）")
	}
}

// quiet(-q):    period out, OnEvent/Finish safety   ( back  ityfour-line receipt). 
func TestCliRenderer_Quiet(t *testing.T) {
	var buf strings.Builder
	r := NewCliRenderer(&buf, true)

	r.OnEvent(LineEvent{Stage: "intent", Detail: "x"})
	r.OnEvent(LineEvent{Stage: "exec", Detail: "y"})
	r.Finish()

	if buf.Len() != 0 {
		t.Fatalf("-q 应零执行期输出, got %q", buf.String())
	}
}
