package observe

import (
	"strings"
	"testing"
)

// 非 quiet：滚动事件覆写当前行；Finish 后清行回行首——外层 REPL 的 "> " 提示符
// 从干净行首打印，不被残留滚动字符污染。
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

	// Finish 必须以清行序列结尾 → 外层提示符干净起行
	got := buf.String()
	if !strings.HasSuffix(got, "\r\x1b[2K") {
		t.Fatalf("Finish 应清行回行首, 输出后缀=%q", got[len(got)-10:])
	}
	// 模拟外层 REPL 随后打印提示符，应落在干净行首（无残留滚动字符）
	buf.WriteString("> ")
	if !strings.Contains(buf.String(), "\r\x1b[2K> ") {
		t.Fatal("提示符应紧跟清行序列（不被滚动行污染）")
	}
}

// quiet（-q）：零执行期输出，OnEvent/Finish 全部静默（退回一次性四行回执）。
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
