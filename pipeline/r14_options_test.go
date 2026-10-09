package pipeline

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/tools"
)

// R14 (2026-10-09): reproduce the BUILD_TEST test-step divergence at the Options
// layer — manual `go test <34 pkgs>` passes, but the harness run failed with
// "exit status 1" and empty stdout. Pins whether o.run("test", ...) differs from
// a plain shell invocation of the exact same argv.
func TestOptionsRunGoTest34(t *testing.T) {
	home, _ := os.UserHomeDir()
	repo := filepath.Join(home, ".voicesign", "harness", "workspace", "VoxSignHarness")
	if _, err := os.Stat(repo); err != nil {
		t.Skipf("workspace clone absent: %v", err)
	}
	cmd := exec.Command("go", "list", "./...")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var pkgs []string
	for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasSuffix(ln, "/asr") || strings.HasSuffix(ln, "/doccontract") {
			continue
		}
		pkgs = append(pkgs, ln)
	}
	argv := append([]string{"go", "test"}, pkgs...)
	t.Logf("pkgs=%d argv_len=%d", len(pkgs), len(argv))

	o := &Options{Exec: &tools.Executor{}, Tools: loadRegistry(t)}
	recv := o.run("test", map[string]any{"command": argv, "cwd": repo, "timeout_s": 180})
	t.Logf("ok=%v stdout=%q stderr=%q err=%q", recv.OK, trunc(recv.Stdout, 200), trunc(recv.Stderr, 200), trunc(recv.Err, 200))
	if !recv.OK {
		t.Fatalf("o.run test failed: err=%q stderr=%q", recv.Err, recv.Stderr)
	}
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func loadRegistry(t *testing.T) *tools.Registry {
	r, err := tools.LoadContracts("")
	if err != nil {
		t.Fatalf("load contracts: %v", err)
	}
	return r
}
