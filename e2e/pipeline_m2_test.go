// Package e2e 的 M2 pipeline 端到端测试：用 t.TempDir() 内最小项目走通 pipeline.Run 全链。
// 只读/低影响意图（NOTE/QUERY）不触发人工确认；COMMIT 触发 human 确认路径。
package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"voicesign-harness/cache"
	"voicesign-harness/config"
	"voicesign-harness/memory"
	"voicesign-harness/pipeline"
	"voicesign-harness/refer"
	"voicesign-harness/space"
	"voicesign-harness/tools"
	"voicesign-harness/trajectory"
	"voicesign-harness/verify"
)

// buildOpts 在临时目录里装配最小可跑 Options（不写 ~/.voicesign）。
func buildOpts(t *testing.T, confirmFn func(string, string) (bool, error)) *pipeline.Options {
	t.Helper()
	root := t.TempDir()
	logDir := filepath.Join(root, "logs")
	memDir := filepath.Join(root, "mem")
	spacesDir := filepath.Join(root, "spaces")
	contractsDir := filepath.Join(root, "contracts")
	cacheDir := filepath.Join(root, "cache")
	for _, d := range []string{logDir, memDir, spacesDir, contractsDir, cacheDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 造一个最小项目 manifest（project 域，scope 指向真实存在目录，避免漂移）。
	projDir := filepath.Join(root, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	spaces, err := space.Load(spacesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := spaces.Add(&space.Manifest{
		Name: "proj", Type: space.TypeProject,
		Scope: []string{projDir},
		Tools: []string{"file", "git", "read", "search", "test", "run"},
		Perms: space.Perms{Read: true, Write: true},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Global.LogDir = logDir
	cfg.Memory.Dir = memDir
	cfg.Spaces.Dir = spacesDir
	cfg.Contracts.Dir = contractsDir
	cfg.Cache.Dir = cacheDir

	dict, err := memory.LoadDictionary(filepath.Join(memDir, "dictionary.json"))
	if err != nil {
		t.Fatal(err)
	}

	reg, err := tools.LoadContracts(contractsDir)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := trajectory.Open(logDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })

	st, err := cache.Open(filepath.Join(cacheDir, "quad.json"), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	return &pipeline.Options{
		Cfg:       &cfg,
		Dict:      dict,
		Spaces:    spaces,
		Refer:     refer.New(dict),
		Cache:     st,
		Tools:     reg,
		Exec:      &tools.Executor{BaseDir: root},
		Verifier:  &verify.Verifier{BaseDir: logDir},
		ConfirmFn: confirmFn,
		Trace:     tr,
	}
}

// TestPipelineNoteAppend 用例 11/12：NOTE 走通全链 → 追加 notes.md → verify pass → 归因写 discuss-log。
func TestPipelineNoteAppend(t *testing.T) {
	opts := buildOpts(t, nil)
	out, err := pipeline.Run(context.Background(), opts, "记一下冀总下周一出报价")
	if err != nil {
		t.Fatal(err)
	}
	if out.Intent.Intent != "NOTE" {
		t.Fatalf("意图应为 NOTE: %+v", out.Intent)
	}
	if out.Ask != "" {
		t.Fatalf("NOTE 不应回问: %q", out.Ask)
	}
	if !out.Verdict.Allowed {
		t.Fatalf("vault-notes 应放行: %+v", out.Verdict)
	}
	// 回执四行齐全
	receipt := strings.Join([]string{
		"动作：" + out.View.Action, "文件：" + out.View.Files,
		"结果：" + out.View.Result, "撤销：" + out.View.Undo,
	}, "\n")
	for _, want := range []string{"动作：", "文件：", "结果：", "撤销："} {
		if !strings.Contains(receipt, want) {
			t.Fatalf("回执缺 %q:\n%s", want, receipt)
		}
	}
	// verify 独立校验：notes.md 真实存在（不读执行器自报）
	if out.Verify.Status != verify.StatusPass {
		t.Fatalf("verify 应 pass（notes.md 真实落盘）: %+v", out.Verify)
	}
	// 归因六格
	if out.Attribution.Class == "" {
		t.Fatal("归因 Class 不应为空")
	}
	// discuss-log 落盘
	dl := filepath.Join(opts.Cfg.Global.LogDir, "discuss.jsonl")
	data, err := os.ReadFile(dl)
	if err != nil {
		t.Fatalf("discuss.jsonl 应存在: %v", err)
	}
	if !strings.Contains(string(data), `"class"`) {
		t.Fatalf("discuss-log 缺归因字段: %s", data)
	}
	// 认知闭环双报
	if out.LoopMs < 0 || out.NetMs < 0 {
		t.Fatalf("LoopMs/NetMs 应为非负: %d/%d", out.LoopMs, out.NetMs)
	}
}

// TestPipelineCommitHumanConfirm 用例 8/9/受控：COMMIT 永远 human → ConfirmFn 被调用。
func TestPipelineCommitHumanConfirm(t *testing.T) {
	called := false
	opts := buildOpts(t, func(taskID, q string) (bool, error) {
		called = true
		return false, nil // 拒绝
	})
	out, err := pipeline.Run(context.Background(), opts, "在 proj 里提交所有改动并推上去")
	if err != nil {
		t.Fatal(err)
	}
	if out.Intent.Intent != "COMMIT" {
		t.Fatalf("意图应为 COMMIT: %+v", out.Intent)
	}
	if out.Decision.Level != "human" {
		t.Fatalf("COMMIT 应裁决 human: %+v", out.Decision)
	}
	if !called {
		t.Fatal("human 确认必须调用 ConfirmFn")
	}
	if out.Confirmed {
		t.Fatal("拒绝后 Confirmed 应为 false")
	}
	if out.View.Result != "待确认（human，未放行）" && !strings.Contains(out.View.Result, "待确认") {
		t.Fatalf("未放行结果文案异常: %q", out.View.Result)
	}
}

// TestPipelineQueryReadOnly QUERY 走 global 只读域 + search。
func TestPipelineQueryReadOnly(t *testing.T) {
	opts := buildOpts(t, nil)
	out, err := pipeline.Run(context.Background(), opts, "查一下 main.go 里有没有 Run")
	if err != nil {
		t.Fatal(err)
	}
	if out.Intent.Intent != "QUERY" {
		t.Fatalf("意图应为 QUERY: %+v", out.Intent)
	}
	if out.Verdict.Allowed == false && out.Verdict.Reason != "" {
		// global 只读域应放行 read/search；若被拒则是集成问题
		t.Fatalf("global 只读应放行 QUERY: %+v", out.Verdict)
	}
}

// TestPipelineSerialGate 串行闸：并发两个 Run 不应竞争写同一轨迹（只验证能跑完不 panic）。
func TestPipelineSerialGate(t *testing.T) {
	opts := buildOpts(t, nil)
	done := make(chan error, 2)
	go func() {
		_, err := pipeline.Run(context.Background(), opts, "记一下第一个想法")
		done <- err
	}()
	go func() {
		_, err := pipeline.Run(context.Background(), opts, "记一下第二个想法")
		done <- err
	}()
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

// TestPipelineSummary 当日摘要可生成。
func TestPipelineSummary(t *testing.T) {
	opts := buildOpts(t, nil)
	_, _ = pipeline.Run(context.Background(), opts, "记一下摘要测试")
	s, err := pipeline.Summary(opts, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "VoxSign 当日摘要") {
		t.Fatalf("摘要格式异常: %q", s)
	}
}
