package pipeline

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// resolveEditPath resolves existing ancestors too, allowing safe new files.
func resolveEditPath(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if _, statErr := os.Lstat(p); statErr == nil {
		return "", err
	} // dangling symlink
	parent := filepath.Dir(p)
	if parent == p {
		return "", err
	}
	resolved, err = resolveEditPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(p)), nil
}

// withinRepo confines resolved paths to the repo, excluding .git.
func withinRepo(repo, p string) (string, bool) {
	if strings.TrimSpace(p) == "" {
		return "", false
	}
	root, err := resolveEditPath(repo)
	if err != nil {
		return "", false
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	p, err = resolveEditPath(p)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	if rel == ".git" || strings.HasPrefix(rel, ".git"+string(os.PathSeparator)) {
		return "", false
	}
	return p, true
}

// Protect the guard file and any existing Go file containing guard definitions.
func protectedGuardFile(p string) bool {
	if filepath.Base(p) == "selfimprove_guard.go" {
		return true
	}
	b, _ := os.ReadFile(p)
	file, _ := parser.ParseFile(token.NewFileSet(), p, b, 0)
	if file == nil {
		return false
	}
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		switch fn.Name.Name {
		case "dangerousCommand", "withinRepo", "frontEndExt", "cleanGateEnv", "protectedGuardFile", "resolveEditPath", "editSafetyError":
			return true
		}
	}
	return false
}

// frontEndExt blocks edits to front-end / iOS artifacts (the backend-only constraint).
func frontEndExt(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{".swift", ".storyboard", ".xib", ".html", ".htm",
		".jsx", ".tsx", ".vue", ".svelte", ".css", ".scss", ".png", ".jpg", ".jpeg",
		".gif", ".ico", ".xcassets", ".pbxproj"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// dangerousCommand rejects destructive / exfiltrating / privileged shell actions.
func dangerousCommand(argv []string) string {
	joined := strings.ToLower(strings.Join(argv, " "))
	// Defense in depth for direct guard writes, including nested sh -c commands.
	// Shell is open-ended: computed paths, aliases, scripts, and other interpreters
	// can evade this heuristic; this is not a filesystem sandbox.
	if strings.Contains(joined, "selfimprove_guard.go") &&
		regexp.MustCompile(`(^|[^a-z0-9_])(sed|tee|mv|dd|perl)([^a-z0-9_]|$)|>`).MatchString(joined) {
		return "command rejected by safety policy: write to selfimprove_guard.go"
	}
	for _, bad := range []string{
		"rm -rf /", "rm -rf /*", "rm -rf ~", "sudo ", "mkfs", "shutdown", "reboot",
		"halt", "init 0", "> /dev/sd", "dd if=", "chmod -r 777 /", "chmod -r 000 /",
		"git push", "git reset --hard", ":(){:|:&};:", "curl ", "wget ", "netcat",
		"scp ", "rsync ", "ssh ", "/etc/passwd", "/etc/shadow", "| sh", "| bash",
		"eval ", "exec >/dev/sd",
	} {
		if strings.Contains(joined, bad) {
			return "command rejected by safety policy: " + strings.TrimSpace(bad)
		}
	}
	// netcat as a standalone token (argv element) — substring "nc " would wrongly kill
	// legitimate greps such as `grep -rn "func "` (the word "func " contains "nc ").
	for _, tok := range argv {
		t := strings.ToLower(strings.TrimSpace(tok))
		if t == "nc" {
			return "command rejected by safety policy: nc (netcat)"
		}
		// Background/detached execution could mutate the repo after the action's
		// snapshot and escape change tracking (final-review blocker #4).
		if t == "&" || t == "nohup" || t == "setsid" || t == "disown" {
			return "command rejected by safety policy: background/detached execution (&, nohup, setsid, disown) is not allowed"
		}
		if hasBackgroundAmp(tok) {
			return "command rejected by safety policy: background operator '&' is not allowed"
		}
	}
	if regexp.MustCompile(`(^|[\s;|&()(])(nohup|setsid|disown)([\s]|$)`).MatchString(joined) {
		return "command rejected by safety policy: nohup/setsid/disown are not allowed"
	}
	return ""
}

// hasBackgroundAmp reports whether s launches a background job with a bare `&`.
// It deliberately ignores `&&`, fd redirects (`2>&1`, `>&2`) and `&>` so normal
// chaining and redirection still work.
func hasBackgroundAmp(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '&' {
			continue
		}
		prev, next := byte(' '), byte(' ')
		if i > 0 {
			prev = s[i-1]
		}
		if i+1 < len(s) {
			next = s[i+1]
		}
		if next == '&' || prev == '&' {
			continue // &&
		}
		if next == '>' || next == '=' {
			continue // &>, &=
		}
		// A digit right after '&' is a dup-fd target only when preceded by a
		// redirect operator, e.g. `2>&1`; `cmd &1` is a background job and is caught.
		if (next >= '0' && next <= '9') && (prev == '>' || prev == '<') {
			continue
		}
		if prev == '>' || prev == '<' || prev == '=' || (prev >= '0' && prev <= '9') {
			continue // 2>&1, >&, =&
		}
		return true
	}
	return false
}

// shellQuote renders s as a single-quoted shell literal, encoding any inner
// single quotes correctly.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellWrap runs an action argv through /bin/sh so shell builtins (cd),
// chaining (&&, ||, ;), pipes and redirects behave exactly as a coding agent
// expects — mirroring the Bash tool provided by Codex and Claude Code. The
// caller locks the working directory to the repo, so `cd` never fails with a
// confusing "executable not found". A command already expressed as
// ["sh","-c",script] is passed through unchanged. Always run dangerousCommand
// on the ORIGINAL argv (before quoting), otherwise the added quotes weaken the
// substring matching.
func shellWrap(argv []string) []string {
	if len(argv) >= 3 && argv[0] == "sh" && argv[1] == "-c" {
		return argv
	}
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = shellQuote(a)
	}
	return []string{"sh", "-c", strings.Join(parts, " ")}
}

// cleanGateEnv clears LLM keys so the repo's tests match CI (R14: a present
// STRATA_API_KEY makes server ASR-sim tests call the real Qwen model and time out).
func cleanGateEnv() map[string]string {
	return map[string]string{
		"STRATA_API_KEY":   "",
		"DEEPSEEK_API_KEY": "",
		"OPENAI_API_KEY":   "",
		"VHS_TOKEN":        "",
	}
}

// editSafetyError centralizes immutable write/replace policy.
func editSafetyError(p string) string {
	if protectedGuardFile(p) {
		return "禁止修改安全护栏: " + p
	}
	if frontEndExt(p) {
		return "禁止修改前端/iOS 文件（后端-only）: " + p
	}
	return ""
}
