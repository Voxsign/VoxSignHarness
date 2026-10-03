package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"voicesign-harness/contract"
	"voicesign-harness/search"
	"voicesign-harness/verify"
)

// Executor 按契约 caps 机械执行六工具。门禁（space_check / risk 裁决）在 pipeline，
// 本执行器不做策略拦截，只做「已知工具 + 已知 cap」的执行与失败回执。
type Executor struct {
	BaseDir string
	Timeout time.Duration
}

// defaultExecTimeout 是未指定超时时的执行上限。
const defaultExecTimeout = 30 * time.Second

func (e *Executor) timeout() time.Duration {
	if e.Timeout > 0 {
		return e.Timeout
	}
	return defaultExecTimeout
}

// str 从 args 取字符串。
func str(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// strSlice 从 args 取 []string（兼容 []any 与单个 string）。
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

// Exec 执行一个工具动作。tool 为契约名；args 为动作参数；c 为对应契约。
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
		out, execErr, ok = e.runCmd(argv)
	case "git":
		argv := strSlice(args, "args")
		out, execErr, ok = e.runCmd(append([]string{"git"}, argv...))
	case "file":
		out, execErr, ok = e.execFile(args)
	case "search":
		out, execErr, ok = e.execSearch(args)
	case "verify":
		out, execErr, ok = e.execVerify(args)
	default:
		recv.Blocked = fmt.Sprintf("执行器未实现工具 %q（仅内置六工具可执行）", tool)
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
		return s[:maxOut] + "…(截断)"
	}
	return s
}

// runCmd 起子进程执行命令，返回 (stdout合并, stderr信息, OK)。
func (e *Executor) runCmd(argv []string) (string, string, bool) {
	if len(argv) == 0 {
		return "", "缺命令 argv", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = e.BaseDir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return buf.String(), "超时", false
	}
	if err != nil {
		return buf.String(), err.Error(), false
	}
	return buf.String(), "", true
}

// BackupMarker 是结构化备份路径输出的标记前缀（M4-2 输出契约，pipeline/server 消费口径）。
//
// 何时出现：file 工具 write/append（EDIT/NOTE）成功，且【目标文件改动前真实存在旧内容】、
// 且 args 提供了 log_dir 时，executor 会先把旧内容备份到 <log_dir>/backups/，并在回执
// stdout 【末尾】追加一行（冒号 + 空格 + 绝对路径）：
//
//	VHS_BACKUP_PATH: /abs/path/to/notes.md.20261002T143000.123Z.bak
//
// 分隔约定（与 D 分片 server rollback 的既有抽取正则 `[^\s"]*backups[^\s"]*\.(bak|backup)`
// 对齐，防接口漂移）：标记与路径之间必须是空白——这样正则从路径起点开始匹配，抽中的是
// 【干净绝对路径】，不会把 `VHS_BACKUP_PATH:` 前缀吞进路径。
//
// 消费方约定：
//   - pipeline：用它渲染四行回执的"撤销"行（`撤销：备份 <basename>`），不再只写"备份目录"；
//   - server rollback：用 ParseBackupPath(stdout) 取到【精确】.bak 路径直接恢复，
//     不再"扫 backups 目录取最新"（避免多任务/并发下拿错备份）；
//   - 无备份场景（read/exists/search/test/run 等只读动作、首次创建无旧内容、未传 log_dir）
//     → stdout 不出现该前缀，解析得空串，撤销行回退为"不可撤销/无备份"。
const BackupMarker = "VHS_BACKUP_PATH:"

// ParseBackupPath 从回执 stdout 解析结构化备份路径；无则返回 ""。
// 统一解析口径，避免 pipeline 与 server 各写一套正则导致接口漂移。
func ParseBackupPath(stdout string) string {
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, BackupMarker) {
			return strings.TrimSpace(strings.TrimPrefix(line, BackupMarker))
		}
	}
	return ""
}

// backupName 生成人类可读 + 机器可定位的备份文件名（notes.md.20261002T143000.123456789Z.bak）。
// 命名约束（与 D 分片 server rollback 的既有抽取正则/兜底扫描对齐，防接口漂移）：
//   - 必须以 ".bak" 结尾（server backupRe `.(?:bak|backup)` 与 newestBackup HasSuffix 都据此匹配）；
//   - 纳秒时间戳置于 ".bak" 之前，避免同一秒内连续编辑同名覆盖前一份备份。
func backupName(rel string) string {
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	return fmt.Sprintf("%s.%s.bak", filepath.Base(rel), stamp)
}

// execFile 执行 file 工具的读/写/追加/存在性；write/append 前把既有内容备份到 <log_dir>/backups，
// 并把具体备份文件绝对路径以 BackupMarker 行带出（结构化输出契约）。
func (e *Executor) execFile(args map[string]any) (string, string, bool) {
	action := str(args, "action")
	rel := str(args, "path")
	if rel == "" {
		return "", "file 动作缺 path", false
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
		// EDIT/NOTE 前写备份（四行回执"撤销"行依据 + server 精确 rollback 输入）。
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
		// 父目录自动创建（2026-10-03 真跑发现：harness-output/ 等新目录 write 失败
		// "no such file or directory"——execOrchestrate 骨架分支首次建目录被拒）。
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return "", "创建父目录失败: " + err.Error(), false
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
	}
	return "", "未知 file 动作: " + action, false
}

// execSearch 委托 search 包（只读扫描）。
func (e *Executor) execSearch(args map[string]any) (string, string, bool) {
	pattern := str(args, "pattern")
	if pattern == "" {
		return "", "search 缺 pattern", false
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

// execVerify 委托 verify 包（独立只读复核，绝不读自报）。
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
