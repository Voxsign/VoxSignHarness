package observe

import (
	"fmt"
	"io"
)

// CliRenderer is CLI       (   §7.3). 
//
//   -   quiet: OnEvent  curbefore  write      (\r +   ),   period state new; 
//     Finish()      andback first,    out  REPL   "> "  show . 
//   - quiet(-q/--quiet):    period out,  back  ityfour-line receipt(OnEvent/Finish safety   ). 
type CliRenderer struct {
	w     io.Writer
	quiet bool
}

func NewCliRenderer(w io.Writer, quiet bool) *CliRenderer {
	return &CliRenderer{w: w, quiet: quiet}
}

// OnEvent       event(  period   out). quiet timefinishsafety  . 
func (r *CliRenderer) OnEvent(ev LineEvent) {
	if r.quiet || r.w == nil {
		return
	}
	detail := ev.Detail
	if detail == "" {
		detail = ev.Status
	}
	// \r back first + \x1b[2K     ->  writeon     ,  tounder outnew . 
	io.WriteString(r.w, fmt.Sprintf("\r\x1b[2K› %s %s", ev.Stage, detail))
}

// Finish closeend  :    after     andback first,  out (REPL "> ") show  be  char   . 
func (r *CliRenderer) Finish() {
	if r.quiet || r.w == nil {
		return
	}
	io.WriteString(r.w, "\r\x1b[2K") //     + back first;  after four-line receipt/ show from   firstopenstart
}
