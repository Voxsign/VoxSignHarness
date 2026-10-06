// Package verify  nowindependent verification (verify   , M2 task  #6 /  recvuseexample 10). 
//
//     (   v2 §6):    done  after, verify [  ]heavynewread  file    status
// andperiod to ,   read       status--preventstop"     become ""  ermodify    ". 
// verify  unique inis Spec{Kind, Args, BaseDir}, close on store "    "    i.e.preventline. 
package verify

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// verifyclose getvalue(Result.Status). 
const (
	StatusPass         = "pass"         //      ed
	StatusFail         = "fail"         //     sendnow  statusandperiod   
	StatusUnverifiable = "unverifiable" // no   (    to / fileread out /  num  )
	StatusPartial      = "partial"      //  splitfull (e.g. grep  inbut overwritesafety period )
)

// Spec is  verify require. Kind decide use      path; Args semantic  Kind   seeunder; 
// BaseDir as topathresolve root(       obj  / file toroot). 
//
// Args   (frozen statusofout semantic now, toaftercompat   ): 
//   - test  : Args =    argv(Args[0]=  name, its as num). verify [  ]raise process , 
//     by[   outcode]asapprove(0=pass,   0=fail), stdout/stderr  as data. 
//   - diff  : Args = [ topath, period   ]. read path  filein ,   [  ]period   only  pass. 
//   - grep  : Args = [ form,  topath...](path empty=    BaseDir under rulefile). 
//       filein in  to formi.e. pass. 
//   - file  : Args = [ topath, period   ?]. pathstore i.e. pass; giveperiod   thenin    . 
type Spec struct {
	Kind    string   `json:"kind"`
	Args    []string `json:"args"`
	BaseDir string   `json:"base_dir"`
}

// Result isindependent verificationclose . Evidence as  trace    data seg(   out / filein   ), 
// Detail as    . 
type Result struct {
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
	Detail   string `json:"detail"`
}

// Verifier isnostatusindependent verification ; BaseDir as   Spec inoverwritetime default toroot. 
type Verifier struct {
	BaseDir string
}

// runTimeout is  verify     onlimit(independent verification  nolimit raise). 
const runTimeout = 60 * time.Second

// Run     independent verification. 
//
// [pseudocode logic layer]( writemodule; semantic/rulebody  VSL,  placeonlywritecontrol flow/branch/rejectpath/errorhandle)
//
// module  : by Spec.Kind       path,   heavy    /   readfile  , toperiod underclose . 
// close preventline: base numsignature  has"      status"in -- inonlyhas Spec, thusnofrom  . 
//
// control flow: 
//  0. base = Spec.BaseDir  empty ? Spec.BaseDir : v.BaseDir(emptythencurbeforeobj  "."). 
//  1. by Kind split : 
//     a. Kind == "test": 
//     - Args empty -> unverifiable(reject empty ). 
//     - raise process exec.CommandContext(ctx, Args[0], Args[1:]...), cwd=base, ctx  time=runTimeout. 
//     -    combined output(stdout+stderr). 
//     - rejectpath:    store (exec.ErrNotFound)-> unverifiable, Evidence=error. 
//     - branch: exit == 0 -> pass( data= outtail ); exit != 0 -> fail(    , i.e. on  calledbecome ). 
//     -  time -> fail( as  ed,    ). 
//     b. Kind == "diff": 
//     - len(Args) < 2 -> unverifiable( period   , no  to). 
//     - path = resolve (base, Args[0]); readfile. 
//     - rejectpath: file store /  read -> fail(  status= has file). 
//     - branch: in  strings.Contains(in , Args[1]) -> pass;  then fail( data=  in   ). 
//     c. Kind == "grep": 
//     - Args empty -> unverifiable(  form). 
//     - targets = Args[1:]  empty ? Args[1:] :   recv  base under rulefile. 
//     -  fileread, rune    ;  ini.e. pass( data=first  in file:line). 
//     - safety readfinishno in -> fail. read  file   ed,     . 
//     d. Kind == "file": 
//     - Args empty -> unverifiable( path). 
//     - path store andas rulefile ->  inin  disconnect;  then fail. 
//     - len(Args) >= 2 -> in     Args[1],  then fail. 
//     e. its  Kind -> unverifiable(  verifyclasstype,   ). 
//
// errorhandle:  has"  status  "   fail(touseuserkeep ,  pipe   become pass); 
//
//	onlyhas"link    all  raise "( num  /     to / rootobj   )onlygive unverifiable. 
//	 data   disconnectto maxEvidence charnode, preventback   . 
func (v *Verifier) Run(spec Spec) (Result, error) {
	base := spec.BaseDir
	if strings.TrimSpace(base) == "" {
		base = v.BaseDir
	}
	if strings.TrimSpace(base) == "" {
		base = "."
	}

	switch spec.Kind {
	case "test":
		return v.runTest(base, spec.Args), nil
	case "diff":
		return v.runDiff(base, spec.Args), nil
	case "grep":
		return v.runGrep(base, spec.Args), nil
	case "file":
		return v.runFile(base, spec.Args), nil
	default:
		return Result{
			Status:   StatusUnverifiable,
			Evidence: "",
			Detail:   fmt.Sprintf("unknown verify kind %q (only test|diff|grep|file)", spec.Kind),
		}, nil
	}
}

// maxEvidence  disconnect data seg  . 
const maxEvidence = 2000

func truncateEvidence(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxEvidence {
		return s[:maxEvidence] + "...(truncated)"
	}
	return s
}

// resolve pipe topath  to base under, and  heavy containment prevent  (M3 #15): 
//  1. word  : Join+Clean after     absBase in(prevent ../   ); 
//  2.  idchainconnect : toclose   EvalSymlinks,   path     base    pathin
//     (prevent symlink referto base out; objtgt store timeget  alreadystore  first  ). 
//
//      -> error(calluse   as unverifiable, reject/  verify,      ). 
func resolve(base, p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	absBase = filepath.Clean(absBase)

	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(absBase, p)
	}
	clean := filepath.Clean(abs)
	//    : word  containment
	if !withinBase(clean, absBase) {
		return "", fmt.Errorf("path out of bounds: %q not within verify root %q", p, absBase)
	}

	//    :  idchainconnect containment
	realBase := evalSafe(absBase)
	real := evalSafe(clean)
	if !withinBase(real, realBase) {
		return "", fmt.Errorf("symlink escape: %q resolves outside verify root %q", p, realBase)
	}
	return clean, nil
}

// withinBase    p is etcat root or at root ofunder. 
func withinBase(p, root string) bool {
	if p == root {
		return true
	}
	return strings.HasPrefix(p, root+string(os.PathSeparator))
}

// evalSafe EvalSymlinks   path;   ( asobjtgt store )timereturnbackits  alreadystore  first. 
func evalSafe(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	// objtgt store : toon      store   first, againresolve its idchainconnect
	cur := p
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return cur
		}
		if real, err := filepath.EvalSymlinks(parent); err == nil {
			return real
		}
		cur = parent
	}
}

// runTest      , by   outcodeasapprove. 
func (v *Verifier) runTest(base string, args []string) Result {
	if len(args) == 0 {
		return Result{Status: StatusUnverifiable, Detail: "test verify missing command argv"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = base
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	evidence := truncateEvidence(buf.String())
	if ctx.Err() == context.DeadlineExceeded {
		return Result{Status: StatusFail, Evidence: evidence, Detail: fmt.Sprintf("verify command timed out (>%s), judged failed", runTimeout)}
	}
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") || os.IsNotExist(err) {
			return Result{Status: StatusUnverifiable, Evidence: evidence, Detail: "verify command does not exist, cannot re-check: " + err.Error()}
		}
		return Result{Status: StatusFail, Evidence: evidence, Detail: fmt.Sprintf("real exit code non-zero: %v", err)}
	}
	return Result{Status: StatusPass, Evidence: evidence, Detail: "verify command real exit code is 0"}
}

// runDiff read  filein toperiod   . 
func (v *Verifier) runDiff(base string, args []string) Result {
	if len(args) < 2 {
		return Result{Status: StatusUnverifiable, Detail: "diff verify needs [path, expected substring]"}
	}
	p, err := resolve(base, args[0])
	if err != nil {
		return Result{Status: StatusUnverifiable, Detail: "diff path invalid: " + err.Error()}
	}
	data, rerr := os.ReadFile(p)
	if rerr != nil {
		return Result{Status: StatusFail, Evidence: truncateEvidence(rerr.Error()), Detail: "real file unreadable (missing or unreadable), judged failed: " + args[0]}
	}
	content := string(data)
	if strings.Contains(content, args[1]) {
		return Result{Status: StatusPass, Evidence: truncateEvidence(content), Detail: fmt.Sprintf("file %s real content contains expected substring", args[0])}
	}
	return Result{Status: StatusFail, Evidence: truncateEvidence(content), Detail: fmt.Sprintf("file %s real content does NOT contain expected substring %q", args[0], args[1])}
}

// runGrep    file     form. 
func (v *Verifier) runGrep(base string, args []string) Result {
	if len(args) == 0 {
		return Result{Status: StatusUnverifiable, Detail: "grep verify missing search pattern"}
	}
	pattern := args[0]
	var targets []string
	if len(args) > 1 {
		for _, rel := range args[1:] {
			p, err := resolve(base, rel)
			if err != nil {
				continue
			}
			targets = append(targets, p)
		}
	} else {
		absBase, _ := filepath.Abs(base)
		realBase := evalSafe(absBase)
		// Walk default    idchainconnect;  place  formverify: symlink referto base out ->  ed(  = ed, 
		//  instop   grep, preventstop     symlink     verifyclose ). 
		_ = filepath.Walk(absBase, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				if real, e := filepath.EvalSymlinks(path); e == nil && !withinBase(real, realBase) {
					if fi.IsDir() {
						return filepath.SkipDir
					}
					return nil // symlink file  :  ed read
				}
			}
			if fi.Mode().IsRegular() {
				targets = append(targets, path)
			}
			return nil
		})
	}
	for _, t := range targets {
		data, err := os.ReadFile(t)
		if err != nil {
			continue
		}
		if idx := strings.Index(string(data), pattern); idx >= 0 {
			line := 1 + strings.Count(string(data)[:idx], "\n")
			rel, _ := filepath.Rel(base, t)
			return Result{Status: StatusPass, Evidence: fmt.Sprintf("%s:%d: %s", rel, line, pattern), Detail: "pattern found in real filesystem content"}
		}
	}
	return Result{Status: StatusFail, Detail: fmt.Sprintf("%d real files searched, pattern %q not found", len(targets), pattern)}
}

// runFile verifyfilestore (  in disconnectlang). 
func (v *Verifier) runFile(base string, args []string) Result {
	if len(args) == 0 {
		return Result{Status: StatusUnverifiable, Detail: "file verify missing path"}
	}
	p, err := resolve(base, args[0])
	if err != nil {
		return Result{Status: StatusUnverifiable, Detail: "file path invalid: " + err.Error()}
	}
	data, rerr := os.ReadFile(p)
	if rerr != nil {
		return Result{Status: StatusFail, Detail: "real file missing or unreadable: " + args[0]}
	}
	if len(args) >= 2 && args[1] != "" {
		if strings.Contains(string(data), args[1]) {
			return Result{Status: StatusPass, Evidence: truncateEvidence(string(data)), Detail: "file exists and contains expected substring"}
		}
		return Result{Status: StatusFail, Evidence: truncateEvidence(string(data)), Detail: fmt.Sprintf("file exists but content lacks expected substring %q", args[1])}
	}
	return Result{Status: StatusPass, Detail: "file really exists: " + args[0]}
}
