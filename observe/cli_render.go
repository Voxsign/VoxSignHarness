package observe

import (
	"fmt"
	"io"
)

// CliRenderer 是 CLI 滚动行渲染器（设计 §7.3）。
//
//   - 非 quiet：OnEvent 在当前行覆写打印一条进度（\r + 清行），执行期动态刷新；
//     Finish() 清掉滚动行并回行首，不破坏外层 REPL 的 "> " 提示符。
//   - quiet（-q/--quiet）：零执行期输出，退回一次性四行回执（OnEvent/Finish 全部静默）。
type CliRenderer struct {
	w     io.Writer
	quiet bool
}

func NewCliRenderer(w io.Writer, quiet bool) *CliRenderer {
	return &CliRenderer{w: w, quiet: quiet}
}

// OnEvent 消费一条进度事件（执行期滚动输出）。quiet 时完全静默。
func (r *CliRenderer) OnEvent(ev LineEvent) {
	if r.quiet || r.w == nil {
		return
	}
	detail := ev.Detail
	if detail == "" {
		detail = ev.Status
	}
	// \r 回行首 + \x1b[2K 清整行 → 覆写上一条滚动行，不向下滚出新行。
	io.WriteString(r.w, fmt.Sprintf("\r\x1b[2K› %s %s", ev.Stage, detail))
}

// Finish 结束滚动：清掉最后一条滚动行并回行首，使外层（REPL "> "）提示符不被残留字符污染。
func (r *CliRenderer) Finish() {
	if r.quiet || r.w == nil {
		return
	}
	io.WriteString(r.w, "\r\x1b[2K") // 清整行 + 回行首；随后的四行回执/提示符从干净行首开始
}
