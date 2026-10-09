package tools

import (
	"voicesign-harness/contract"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func homeDir(t *testing.T) string { h, _ := os.UserHomeDir(); return h }
func osStat(p string) (os.FileInfo, error) { return os.Stat(p) }
func execCommandOutput(t *testing.T, name string, args ...string) string {
	return execCommandOutputIn(t, "", name, args...)
}
func execCommandOutputIn(t *testing.T, dir, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return string(b)
}
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// R14 (2026-10-09): repro for "BUILD_TEST test step failed exit status 1 with
// empty stdout although the same `go test <pkgs>` passes by hand". Exercises the
// real Executor against the cloned repo to pin the divergence (PATH/argv/timeout).
func TestExecGoTest33(t *testing.T) {
	repo := filepath.Join(homeDir(t), ".voicesign", "harness", "workspace", "VoxSignHarness")
	if _, err := osStat(repo); err != nil {
		t.Skipf("workspace clone absent: %v", err)
	}
	// same package filter as execBuildTest (HasSuffix /asr, /doccontract)
	list := execCommandOutput(t, "go", "list", "./...")
	var pkgs []string
	for _, ln := range strings.Split(strings.TrimSpace(list), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasSuffix(ln, "/asr") || strings.HasSuffix(ln, "/doccontract") {
			continue
		}
		pkgs = append(pkgs, ln)
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages listed")
	}
	e := &Executor{BaseDir: repo, Timeout: 180 * time.Second}
	recv, err := e.Exec("test", map[string]any{"command": append([]string{"go", "test"}, pkgs...), "cwd": repo}, contract.ToolContract{})
	if err != nil {
		t.Fatalf("exec err: %v", err)
	}
	t.Logf("pkgs=%d ok=%v stdout=%q stderr=%q", len(pkgs), recv.OK, truncate(recv.Stdout, 300), truncate(recv.Stderr, 300))
	if !recv.OK {
		t.Fatalf("expected go test to pass like manual run; stderr=%q", recv.Stderr)
	}
}
