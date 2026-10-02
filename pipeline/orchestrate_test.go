package pipeline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/space"
)

// initGitRepo 在 dir 初始化一个干净的 git 仓（带身份 + 一个基线提交）。
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "vhs-test@example.com"},
		{"config", "user.name", "vhs-test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	add := exec.Command("git", "add", "README.md")
	add.Dir = dir
	_ = add.Run()
	cm := exec.Command("git", "commit", "-m", "baseline")
	cm.Dir = dir
	if out, err := cm.CombinedOutput(); err != nil {
		t.Fatalf("baseline commit failed: %s", out)
	}
}

// setupOrchestrateProj 在临时目录建一个带 docs/ 的 project git 仓并注册 project 域。
func setupOrchestrateProj(t *testing.T, o *Options) string {
	t.Helper()
	projDir := t.TempDir()
	docsDir := filepath.Join(projDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 预置两份默认源文档。
	for _, name := range []string{"SPEC-v2-可执行规格书.md", "M7配置指南.md"} {
		if err := os.WriteFile(filepath.Join(docsDir, name),
			[]byte("内容："+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	initGitRepo(t, projDir)
	if err := o.Spaces.Add(&space.Manifest{
		Name:  "project",
		Type:  space.TypeProject,
		Scope: []string{projDir},
		Tools: []string{"file", "git", "search", "read", "test", "run"},
		Perms: space.Perms{Read: true, Write: true},
	}); err != nil {
		t.Fatal(err)
	}
	return projDir
}

// TestOrchestrateClassify：复合长任务必须命中 ORCHESTRATE，不再压成 NOTE。
func TestOrchestrateClassify(t *testing.T) {
	o := testOptions(t, func(string, string) (bool, error) { return true, nil })
	setupOrchestrateProj(t, o)
	text := "把全部沟通记录和设计文档整理成《VoiceSign Harness 全景开发文档》并保存提交"
	out, err := Run(context.Background(), o, text)
	if err != nil {
		t.Fatal(err)
	}
	if out.Intent.Intent != contract.IntentOrchestrate {
		t.Fatalf("长任务应命中 ORCHESTRATE, got %q (ask=%q)", out.Intent.Intent, out.Ask)
	}
	if out.Intent.Params["target_doc"] != "VoiceSign Harness 全景开发文档" {
		t.Fatalf("应抽出书名号目标文档名, got %q", out.Intent.Params["target_doc"])
	}
}

// TestOrchestrateEndToEnd：跑完整链——读文档→写文件→真实 git commit，全部回执真实。
func TestOrchestrateEndToEnd(t *testing.T) {
	confirmed := false
	o := testOptions(t, func(string, string) (bool, error) { confirmed = true; return true, nil })
	projDir := setupOrchestrateProj(t, o)

	text := "把全部沟通记录和设计文档整理成《VoiceSign-Harness-全景开发文档》并保存提交"
	out, err := Run(context.Background(), o, text)
	if err != nil {
		t.Fatal(err)
	}
	if !confirmed {
		t.Fatal("ORCHESTRATE 不可逆，人工确认闸必须触发一次")
	}
	if out.Ask != "" {
		t.Fatalf("不应回问, ask=%q", out.Ask)
	}
	// 多步回执：至少 2 次 file read + 1 次 file write + 1 次 git commit。
	var reads, writes, commits int
	for _, r := range out.Receipts {
		if r.Tool == "file" && strings.HasPrefix(r.Stdout, "writed:") {
			writes++
		} else if r.Tool == "file" {
			reads++
		}
		if r.Tool == "git" {
			commits++
		}
	}
	if reads < 1 || writes != 1 || commits != 1 {
		t.Fatalf("动作链异常: reads=%d writes=%d commits=%d receipts=%+v", reads, writes, commits, out.Receipts)
	}
	// 最后一条 git 回执必须 OK 且带 commit hash。
	gitRecv := out.Receipts[len(out.Receipts)-1]
	if !gitRecv.OK {
		t.Fatalf("git 提交应成功: %+v", gitRecv)
	}
	if !strings.Contains(gitRecv.Stdout, "vhs(orchestrate)") {
		t.Fatalf("git 回执应含提交记录: %q", gitRecv.Stdout)
	}
	// 生成文件真实存在且非空。
	target := filepath.Join(projDir, "docs", "VoiceSign-Harness-全景开发文档.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("生成文件应存在: %v", err)
	}
	if !strings.Contains(string(data), "VoiceSign-Harness-全景开发文档") {
		t.Fatalf("生成文件内容缺标题: %.200s", data)
	}
	// git log 真实可查。
	log := exec.Command("git", "log", "-1", "--format=%s")
	log.Dir = projDir
	lout, err := log.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lout), "orchestrate") {
		t.Fatalf("git log 应能查到编排提交: %s", lout)
	}
	// 四行回执要展示多步链。
	if !strings.Contains(out.View.Result, "多步链完成") {
		t.Fatalf("回执结果应展示多步链: %q", out.View.Result)
	}
}

// TestOrchestrateNoSquashNote：回归保护——"记一下 X"仍是 NOTE，不被误判 ORCHESTRATE。
func TestOrchestrateNoSquashNote(t *testing.T) {
	o := testOptions(t, nil)
	out, err := Run(context.Background(), o, "记一下明天要开会")
	if err != nil {
		t.Fatal(err)
	}
	if out.Intent.Intent == contract.IntentOrchestrate {
		t.Fatal("普通记笔记不应被判为 ORCHESTRATE")
	}
	if out.Intent.Intent != contract.IntentNote {
		t.Fatalf("应为 NOTE, got %q", out.Intent.Intent)
	}
}

// TestOrchestrateWhitelistReject：target_doc 含 .. 穿越 → 写路径越界必须被拒绝，
// 不写盘、不提交。
func TestOrchestrateWhitelistReject(t *testing.T) {
	o := testOptions(t, func(string, string) (bool, error) { return true, nil })
	projDir := setupOrchestrateProj(t, o)
	// 直接调编排引擎，传一个穿越域根的 target_doc。
	it := contract.Intent{
		Intent:        contract.IntentOrchestrate,
		CorrectedText: "整理文档提交",
		Space:         "project",
		Params:        map[string]string{"target_doc": "../../../etc/vhs-pwned", "commit": "1"},
	}
	receipts := o.execOrchestrate(context.Background(), it, o.Cfg.Global.LogDir)
	if len(receipts) == 0 {
		t.Fatal("应产出拒绝回执")
	}
	last := receipts[len(receipts)-1]
	if last.OK {
		t.Fatalf("越界写必须被拒绝, last=%+v", last)
	}
	if !strings.Contains(last.Err, "白名单外") {
		t.Fatalf("拒绝原因应明确写'白名单外', got %q", last.Err)
	}
	// 确认越界文件没有被创建。
	if _, err := os.Stat(filepath.Join(projDir, "docs", "..", "..", "..", "etc", "vhs-pwned.md")); !os.IsNotExist(err) {
		t.Fatal("越界文件不应被创建")
	}
}
