package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"voicesign-harness/contract"
	"voicesign-harness/search"
	"voicesign-harness/verify"
)

// Executor by   caps        .  forbid(space_check / risk  decide)  pipeline, 
// base       block, only "already    + already  cap"   and  back . 
type Executor struct {
	BaseDir string
	Timeout time.Duration
}

// defaultExecTimeout is refer  timetime   onlimit. 
const defaultExecTimeout = 30 * time.Second

func (e *Executor) timeout() time.Duration {
	if e.Timeout > 0 {
		return e.Timeout
	}
	return defaultExecTimeout
}

// str from args getchar  . 
func str(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// intVal reads an integer parameter (used for timeout_s on long-running jobs).
func intVal(args map[string]any, key string) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case float64:
			return int(n)
		case string:
			if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
				return i
			}
		}
	}
	return 0
}

// strSlice from args get []string(compat []any and   string). 
func strSlice(args map[string]any, key string) []string {
	switch v := args[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	}
	return nil
}

// Exec         . tool as  name; args as   num; c asto   . 
func (e *Executor) Exec(tool string, args map[string]any, c contract.ToolContract) (contract.Receipt, error) {
	recv := contract.Receipt{Tool: tool}
	if args == nil {
		args = map[string]any{}
	}
	start := time.Now()
	var ok bool
	var out string
	var execErr string

	switch tool {
	case "test", "run":
		argv := strSlice(args, "command")
		// distillation R6: long-running jobs (npm install -g) pass timeout_s to override
		// the default 30s exec cap.
		out, execErr, ok = e.runCmdIn(argv, str(args, "cwd"), intVal(args, "timeout_s"))
	case "git":
		argv := strSlice(args, "args")
		out, execErr, ok = e.runCmdIn(append([]string{"git"}, argv...), str(args, "cwd"))
	case "file":
		out, execErr, ok = e.execFile(args)
	case "search":
		out, execErr, ok = e.execSearch(args)
	case "verify":
		out, execErr, ok = e.execVerify(args)
	case "remote-desktop":
		// 2026-10-04      seg: langaudio  note  "  control  "   ->      
		//(list face/  /   use/read-only   name ).   =useuser    Mac(harness   ). 
		out, execErr, ok = e.execRemote(args)
	case "weather":
		// 2026-10-04      seg: langaudionote  "  day "->      . 
		// connect openday  API(wttr.in, no key), returnbackcurbeforeday ;   from lang get, default    . 
		out, execErr, ok = e.execWeather(args)
	default:
		recv.Blocked = fmt.Sprintf("executor: tool %q not implemented (only six built-in tools are executable)", tool)
		recv.DurationMs = time.Since(start).Milliseconds()
		return recv, nil
	}
	recv.OK = ok
	recv.Stdout = truncateOut(out)
	if execErr != "" {
		recv.Stderr = truncateOut(execErr)
	}
	recv.DurationMs = time.Since(start).Milliseconds()
	return recv, nil
}

const maxOut = 4000

func truncateOut(s string) string {
	if len(s) > maxOut {
		return s[:maxOut] + "...(truncated)"
	}
	return s
}

// runCmd raise process    , returnback (stdout and, stderr  , OK). 
func (e *Executor) runCmd(argv []string) (string, string, bool) {
	return e.runCmdIn(argv, "")
}

// runCmdIn is runCmd with an explicit working directory; empty dir falls back to BaseDir.
// distillation R5 (2026-10-08): build/test commands must run inside the cloned repo.
func (e *Executor) runCmdIn(argv []string, dir string, timeoutSec ...int) (string, string, bool) {
	if len(argv) == 0 {
		return "", "missing command argv", false
	}
	timeout := e.timeout()
	if len(timeoutSec) > 0 && timeoutSec[0] > 0 {
		timeout = time.Duration(timeoutSec[0]) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	if strings.TrimSpace(dir) == "" {
		cmd.Dir = e.BaseDir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return buf.String(), "timed out", false
	}
	if err != nil {
		return buf.String(), err.Error(), false
	}
	return buf.String(), "", true
}

// BackupMarker isclose ize  path out tgt before (M4-2  out  , pipeline/server    path). 
//
//  timeoutnow: file    write/append(EDIT/NOTE)become , and[objtgtfilechangebefore  store  in ], 
// and args  provide log_dir time, executor  firstpipe in   to <log_dir>/backups/, and back 
// stdout [endtail]    ( id + empty  +  topath): 
//
//	VHS_BACKUP_PATH: /abs/path/to/notes.md.20261002T143000.123Z.bak
//
// split   (and D split  server rollback   has getposthen `[^\s"]*backups[^\s"]*\.(bak|backup)`
// to , preventconnect   ): tgt andpathoftime  isempty -- kindposthenfrompathraiseptopenstart  ,  in is
// [   topath],   pipe `VHS_BACKUP_PATH:` before   path. 
//
//      : 
//   - pipeline: use   four-line receipt "  " (`  :    <basename>`),  againonlywrite"  obj "; 
//   - server rollback: use ParseBackupPath(stdout) getto[  ].bak path connect  , 
//      again"  backups obj get new"(   task/andsendunder    ); 
//   - no   scenario(read/exists/search/test/run etcread-only  , first   no in ,    log_dir)
//     -> stdout  outnow before , resolve  empty ,    back as"    /no  ". 
const BackupMarker = "VHS_BACKUP_PATH:"

// ParseBackupPath fromback  stdout resolve close ize  path; nothenreturnback "". 
//   resolve  path,    pipeline and server  write  posthen  connect   . 
func ParseBackupPath(stdout string) string {
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, BackupMarker) {
			return strings.TrimSpace(strings.TrimPrefix(line, BackupMarker))
		}
	}
	return ""
}

// backupName occurbecome class read +         filename(notes.md.20261002T143000.123456789Z.bak). 
//  name end(and D split  server rollback   has getposthen/ bot  to , preventconnect   ): 
//   -   by ".bak" closetail(server backupRe `.(?:bak|backup)` and newestBackup HasSuffix alldata   ); 
//   -  sectimetime  at ".bak" ofbefore,   same secinlinkcontinue  samenameoverwritebefore    . 
func backupName(rel string) string {
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	return fmt.Sprintf("%s.%s.bak", filepath.Base(rel), stamp)
}

// execFile    file    read/write/  /store ity; write/append beforepipe hasin   to <log_dir>/backups, 
// andpipe body  file topathby BackupMarker   out(close ize out  ). 
func (e *Executor) execFile(args map[string]any) (string, string, bool) {
	action := str(args, "action")
	rel := str(args, "path")
	if rel == "" {
		return "", "file action missing path", false
	}
	abs := rel
	if !filepath.IsAbs(rel) {
		abs = filepath.Join(e.BaseDir, rel)
	}
	switch action {
	case "read", "":
		data, err := os.ReadFile(abs)
		if err != nil {
			return "", err.Error(), false
		}
		return string(data), "", true
	case "exists":
		if _, err := os.Stat(abs); err != nil {
			return "", err.Error(), false
		}
		return "exists: " + rel, "", true
	case "write", "append":
		// EDIT/NOTE beforewrite  (four-line receipt"  "  data + server    rollback  in). 
		backupPath := ""
		if existing, rerr := os.ReadFile(abs); rerr == nil {
			if logDir := str(args, "log_dir"); logDir != "" {
				bdir := filepath.Join(logDir, "backups")
				_ = os.MkdirAll(bdir, 0o755)
				backupPath = filepath.Join(bdir, backupName(rel))
				_ = os.WriteFile(backupPath, existing, 0o644)
			}
		}
		content := str(args, "content")
		var flag int
		if action == "append" {
			flag = os.O_APPEND | os.O_CREATE | os.O_WRONLY
		} else {
			flag = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
		}
		f, err := os.OpenFile(abs, flag, 0o644)
		if err != nil {
			return "", err.Error(), false
		}
		defer f.Close()
		if _, err := f.WriteString(content); err != nil {
			return "", err.Error(), false
		}
		out := action + "d: " + rel
		if backupPath != "" {
			out += "\n" + BackupMarker + " " + backupPath
		}
		return out, "", true
	case "replace":
		// 2026-10-08 (distillation R3): conversational edit "把笔记里的A改成B" / "把X换成Y".
		// Reads the target, backs it up, replaces all occurrences of old with new, writes back.
		// Returns the backup path in the receipt for rollback.
		old, new := str(args, "old"), str(args, "new")
		if old == "" {
			return "", "replace missing old", false
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return "", err.Error(), false
		}
		updated := strings.ReplaceAll(string(data), old, new)
		if updated == string(data) {
			return "replace: no occurrence of target text in " + rel, "", true
		}
		backupPath := ""
		if logDir := str(args, "log_dir"); logDir != "" {
			bdir := filepath.Join(logDir, "backups")
			_ = os.MkdirAll(bdir, 0o755)
			backupPath = filepath.Join(bdir, backupName(rel))
			_ = os.WriteFile(backupPath, data, 0o644)
		}
		if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
			return "", err.Error(), false
		}
		out := "replaced in: " + rel
		if backupPath != "" {
			out += "\n" + BackupMarker + " " + backupPath
		}
		return out, "", true
	}
	return "", "unknown file action: " + action, false
}

// execSearch    search  (read-only  ). 
func (e *Executor) execSearch(args map[string]any) (string, string, bool) {
	pattern := str(args, "pattern")
	if pattern == "" {
		return "", "search missing pattern", false
	}
	opts := search.Options{Roots: []string{e.BaseDir}}
	var (
		hits []search.Hit
		err  error
	)
	if str(args, "kind") == "symbol" {
		hits, err = search.FindSymbol(pattern, opts)
	} else {
		hits, err = search.FindText(pattern, opts)
	}
	if err != nil {
		return "", err.Error(), false
	}
	var b strings.Builder
	for _, h := range hits {
		fmt.Fprintf(&b, "%s:%d [%s] %s\n", h.File, h.Line, h.Kind, h.LineText)
	}
	return b.String(), "", true
}

// execVerify    verify  (  read-only  ,   read  ). 
func (e *Executor) execVerify(args map[string]any) (string, string, bool) {
	spec := verify.Spec{Kind: str(args, "kind"), Args: strSlice(args, "args"), BaseDir: e.BaseDir}
	v := &verify.Verifier{BaseDir: e.BaseDir}
	res, err := v.Run(spec)
	if err != nil {
		return "", err.Error(), false
	}
	ok := res.Status == verify.StatusPass
	detail := res.Status + ": " + res.Detail
	if res.Evidence != "" {
		detail += "\n" + res.Evidence
	}
	return detail, "", ok
}

// ----------   control  (     seg: langaudionote    ->      ) ----------

// remoteReadOnlyCmds read-only   name (  controlonly allowno  use   class  ; 
// writeclass    has git/file    and forbid,     open). 
var remoteReadOnlyCmds = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true,
	"pwd": true, "whoami": true, "date": true, "ps": true,
	"df": true, "du": true, "echo": true, "uptime": true,
}

// execRemote   "  control  "  : by base   list facefile,  get  , 
// listout   use, or   name read-only  .  out  close (<=15  ),   keepstoreto
// log_dir/screenshots/ and  path(iOS endby baseback  show). 
func (e *Executor) execRemote(args map[string]any) (string, string, bool) {
	text := str(args, "text")
	logDir := str(args, "log_dir")
	var b strings.Builder

	home, err := os.UserHomeDir()
	if err != nil {
		return "", "cannot locate user home dir: " + err.Error(), false
	}

	// ①  name read-only  :  base outnow "   ls /    cat /     df" etc -> resolve     . 
	if cmd := extractReadOnlyCmd(text); cmd != "" {
		argv := strings.Fields(cmd)
		out, errStr, ok := e.runCmd(argv)
		if ok {
			b.WriteString("Command \"" + cmd + "\"" + " output:\n" + out)
			return b.String(), "", true
		}
		b.WriteString("Command \"" + cmd + "\"" + " failed: " + errStr + "\n")
	}

	// ②  facefilelisttable("   faceonhas  "). 
	desktop := filepath.Join(home, "Desktop")
	if entries, err := os.ReadDir(desktop); err == nil {
		var names []string
		for _, ent := range entries {
			if len(names) >= 15 {
				names = append(names, "... and "+fmt.Sprintf("%d", len(entries))+" more")
				break
			}
			suffix := ""
			if ent.IsDir() {
				suffix = "/"
			}
			names = append(names, ent.Name()+suffix)
		}
		b.WriteString("Desktop has " + fmt.Sprintf("%d", len(entries)) + " items: " + strings.Join(names, ", ") + "\n")
	} else {
		b.WriteString("(cannot read desktop dir: " + err.Error() + ")\n")
	}

	// ③     (screencapture need   restrict limit;      ,  show limit). 
	// in  lang  : "   "middle " "char, "   /  /  / face/  "safetyrecv. 
	if strings.Contains(text, "截个图") || strings.Contains(text, "截图") ||
		strings.Contains(text, "截屏") || strings.Contains(text, "桌面") ||
		strings.Contains(text, "看看") {
		shotDir := filepath.Join(logDir, "screenshots")
		if err := os.MkdirAll(shotDir, 0o755); err == nil {
			png := filepath.Join(shotDir, "screen-"+time.Now().Format("20060102-150405")+".png")
			if err := exec.Command("screencapture", "-x", png).Run(); err != nil {
				b.WriteString("Screenshot failed (may lack screen-recording permission or screen unavailable): " + err.Error() + "\n")
			} else if fi, err := os.Stat(png); err != nil || fi.Size() == 0 {
				b.WriteString("No screenshot produced (file empty or unreadable)\n")
			} else {
				//   back :  out to URL(/screenshots/<file>), iOS end  base  connect  ; 
				//  again out Mac basely topath(  no   ). 
				b.WriteString("Screenshot saved: /screenshots/" + filepath.Base(png) + " (" + fmt.Sprintf("%d", fi.Size()/1024) + " KB)\n")
			}
		}
	}

	// ④   in  use(" open/  /has   use"). 
	if strings.Contains(text, "应用") || strings.Contains(text, "运行") || strings.Contains(text, "程序") {
		if out, errStr, ok := e.runCmd([]string{"ps", "-axo", "comm"}); ok {
			lines := strings.Split(out, "\n")
			seen := map[string]bool{}
			var apps []string
			for _, ln := range lines {
				name := strings.TrimSpace(ln)
				if name == "" || strings.HasPrefix(name, "comm") {
					continue
				}
				base := filepath.Base(name)
				if !seen[base] && !strings.HasPrefix(base, "/") {
					seen[base] = true
					apps = append(apps, base)
				}
				if len(apps) >= 15 {
					break
				}
			}
			if len(apps) > 0 {
				b.WriteString("Currently running main apps: " + strings.Join(apps, ", ") + "\n")
			}
		} else {
			b.WriteString("Process list failed: " + errStr + "\n")
		}
	}

	if b.Len() == 0 {
		b.WriteString("Remote control is ready. Try saying \"show what is on the desktop\", \"take a screenshot\", \"run ls\", or \"which apps are running\".")
	} else if !strings.Contains(text, "截图") && !strings.Contains(text, "截个图") &&
		!strings.Contains(text, "截屏") && !strings.Contains(text, "桌面") &&
		!strings.Contains(text, "看看") && !strings.Contains(text, "应用") &&
		!strings.Contains(text, "运行") && !strings.Contains(text, "程序") &&
		extractReadOnlyCmd(text) == "" {
		//      (" controlafter     "): alreadyuse facelisttable    ,     . 
		b.WriteString("(You can control this machine. Say \"remote control: take a screenshot\" or \"run ls -la /tmp\" to execute)\n")
	}
	return b.String(), "", true
}

// extractReadOnlyCmd from lang base get name read-only  ("   ls -la" / "   cat xxx"). 
// onlyconnectaccept remoteReadOnlyCmds  name in   name; its      (preventnotein). 
func extractReadOnlyCmd(text string) string {
	prefixes := []string{"运行 ", "执行 ", "帮我跑 ", "跑一下 ", "用 ", "命令 "}
	for _, p := range prefixes {
		if idx := strings.Index(text, p); idx >= 0 {
			rest := strings.TrimSpace(text[idx+len(p):])
			if fields := strings.Fields(rest); len(fields) > 0 {
				if remoteReadOnlyCmds[fields[0]] {
					return strings.Join(fields, " ")
				}
			}
		}
	}
	return ""
}

// weatherCities  see  name(in ),  lang ini.e.by   day ;   in    . 
var weatherCities = []string{
	"北京", "上海", "广州", "深圳", "杭州", "成都", "重庆", "武汉", "西安", "南京",
	"天津", "苏州", "长沙", "青岛", "大连", "厦门", "福州", "合肥", "郑州", "济南",
	"昆明", "贵阳", "乌鲁木齐", "兰州", "哈尔滨", "长春", "沈阳", "南昌", "南宁", "海口",
	"利雅得", "迪拜", "多哈", "吉达", "麦加", "麦地那",
	"Riyadh", "Dubai", "Doha", "Jeddah", "London", "New York", "Tokyo", "Singapore",
}

// extractCityForWeather from lang get  name("  under   day "->  ); 
//  in see  tablethenreturnback,  thenempty (wttr.in     ). 
func extractCityForWeather(text string) string {
	for _, c := range weatherCities {
		if strings.Contains(text, c) {
			return c
		}
	}
	return ""
}

// execWeather   "  day "  :   calluse openday  API(wttr.in, noneed key). 
//  out  day (day now /  /  /  );   returnbackorigbecauseback (     etc),    . 
func (e *Executor) execWeather(args map[string]any) (string, string, bool) {
	text := str(args, "text")
	if text == "" {
		text = str(args, "pattern")
	}
	city := extractCityForWeather(text)
	queryURL := "https://wttr.in/?format=%C+%t+%h+%w"
	if city != "" {
		queryURL = fmt.Sprintf("https://wttr.in/%s?format=%%C+%%t+%%h+%%w", url.QueryEscape(city))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return "", "failed to build weather request: " + err.Error(), false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "weather service unreachable (network issue): " + err.Error(), false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	raw := strings.TrimSpace(string(body))
	if resp.StatusCode != 200 || raw == "" {
		return "", fmt.Sprintf("weather service returned error (HTTP %d)", resp.StatusCode), false
	}
	// wttr.in format  out e.g. "Patchy rain nearby +22°C 63% 9km/h"
	where := city
	if where == "" {
		where = "current city"
	}
	out := "Weather now (" + where + "): " + raw
	return out, "", true
}
