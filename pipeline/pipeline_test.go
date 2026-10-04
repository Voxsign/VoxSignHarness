package pipeline

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"voicesign-harness/cache"
	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/memory"
	"voicesign-harness/provider"
	"voicesign-harness/refer"
	"voicesign-harness/space"
	"voicesign-harness/tools"
	"voicesign-harness/trajectory"
	"voicesign-harness/verify"
)

func testOptions(t *testing.T, confirm func(string, string) (bool, error)) *Options {
	t.Helper()
	root := t.TempDir()
	logDir := filepath.Join(root, "logs")
	for _, d := range []string{logDir, filepath.Join(root, "spaces"), filepath.Join(root, "contracts"), filepath.Join(root, "cache")} {
		_ = os.MkdirAll(d, 0o755)
	}
	cfg := config.Default()
	cfg.Global.LogDir = logDir
	cfg.Spaces.Dir = filepath.Join(root, "spaces")
	cfg.Contracts.Dir = filepath.Join(root, "contracts")
	cfg.Cache.Dir = filepath.Join(root, "cache")

	dict, _ := memory.LoadDictionary("")
	spaces, _ := space.Load(cfg.Spaces.Dir)
	reg, _ := tools.LoadContracts(cfg.Contracts.Dir)
	tr, _ := trajectory.Open(logDir)
	t.Cleanup(func() { tr.Close() })
	st, _ := cache.Open(filepath.Join(cfg.Cache.Dir, "quad.json"), time.Hour)
	return &Options{
		Cfg:       &cfg,
		Dict:      dict,
		Spaces:    spaces,
		Refer:     refer.New(dict),
		Cache:     st,
		Tools:     reg,
		Exec:      &tools.Executor{BaseDir: root},
		Verifier:  &verify.Verifier{BaseDir: logDir},
		ConfirmFn: confirm,
		Trace:     tr,
	}
}

// TestPipelineWritesAttribution（SPEC 用例 12 / #43）：执行后归因写轨迹 + discuss-log。
func TestPipelineWritesAttribution(t *testing.T) {
	o := testOptions(t, nil)
	out, err := Run(context.Background(), o, "记一下归因测试")
	if err != nil {
		t.Fatal(err)
	}
	// 归因六格之一
	valid := map[string]bool{
		"input": true, "context": true, "contract": true,
		"model": true, "execution": true, "external": true,
	}
	if !valid[out.Attribution.Class] {
		t.Fatalf("归因 Class 必须∈六格, got %q", out.Attribution.Class)
	}
	if out.Attribution.Evidence == "" {
		t.Fatal("归因必须带证据")
	}
	// discuss.jsonl 落盘
	data, err := os.ReadFile(filepath.Join(o.Cfg.Global.LogDir, "discuss.jsonl"))
	if err != nil {
		t.Fatalf("discuss-log 应存在: %v", err)
	}
	if !strings.Contains(string(data), `"class"`) {
		t.Fatalf("discuss-log 缺归因: %s", data)
	}
	// 认知闭环双报
	if out.LoopMs < 0 || out.NetMs < 0 {
		t.Fatalf("LoopMs/NetMs 非负: %d/%d", out.LoopMs, out.NetMs)
	}
}

// TestPipelineOrdering（SPEC #5）：执行不得在 space_check/risk 之前；拦截即止。
func TestPipelineOrdering(t *testing.T) {
	// 2026-10-04 语音场景修复：写意图无点名域默认落 project（消除"跑一下测试/提交代码"→越界）；
	// 无项目根时以 FAILED 收据表达，而非 global 只读域 BOUNDARY_VIOLATION。
	called := false
	o := testOptions(t, func(string, string) (bool, error) { called = true; return true, nil })
	// 直接发起 COMMIT（无 proj 域）→ 默认 project 域，执行被放行；无项目根 → FAILED
	out, err := Run(context.Background(), o, "提交代码")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Verdict.Allowed {
		t.Fatal("写意图默认落 project 域，应被放行（不再对 global 越界）")
	}
	if !called {
		t.Fatal("COMMIT 为 human 级确认，应调用 ConfirmFn（voice 模式由 server 自动放行）")
	}
	if !strings.Contains(out.View.Result, "FAILED") {
		t.Fatalf("无项目根应报 FAILED（而非越界）: %q", out.View.Result)
	}
	if strings.Contains(out.View.Result, "BOUNDARY_VIOLATION") {
		t.Fatalf("不应再标越界: %q", out.View.Result)
	}
}

// TestPipelineConfirmStrategy（用例 8/9）：auto 不打断；human 永远问。
func TestPipelineConfirmStrategy(t *testing.T) {
	// auto 路径：ConfirmFn 不被调用
	autoCalled := false
	o1 := testOptions(t, func(string, string) (bool, error) { autoCalled = true; return true, nil })
	if _, err := Run(context.Background(), o1, "记一下 auto 测试"); err != nil {
		t.Fatal(err)
	}
	if autoCalled {
		t.Fatal("auto 决策不应打断用户")
	}

	// human 路径：注册 proj 域后 COMMIT 必问
	projDir := t.TempDir()
	o2 := testOptions(t, nil)
	_ = o2.Spaces.Add(&space.Manifest{
		Name: "proj", Type: space.TypeProject, Scope: []string{projDir},
		Tools: []string{"git", "read"}, Perms: space.Perms{Read: true, Write: true},
	})
	humanCalled := false
	o2.ConfirmFn = func(string, string) (bool, error) { humanCalled = true; return false, nil }
	out, err := Run(context.Background(), o2, "在 proj 里提交所有改动")
	if err != nil {
		t.Fatal(err)
	}
	if !humanCalled {
		t.Fatal("human 决策必须调用 ConfirmFn")
	}
	if out.Decision.Level != "human" {
		t.Fatalf("COMMIT 应 human, got %q", out.Decision.Level)
	}
}

// TestPipelineVerifyHooked（用例 10）：NOTE 后 verify 独立校验 notes.md 真实存在。
func TestPipelineVerifyHooked(t *testing.T) {
	o := testOptions(t, nil)
	out, err := Run(context.Background(), o, "记一下 verify 钩子")
	if err != nil {
		t.Fatal(err)
	}
	if out.Verify.Status != verify.StatusPass {
		t.Fatalf("verify 应独立复核 pass, got %+v", out.Verify)
	}
}

// TestMechanicalImpactRealRefCount：temp 项目内有引用 vs 无引用 → RefCount 差异。
func TestMechanicalImpactRealRefCount(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "proj")
	_ = os.MkdirAll(projDir, 0o755)
	// 目标文件 + 两个引用它的文件
	_ = os.WriteFile(filepath.Join(projDir, "target.go"), []byte("package target\n"), 0o644)
	_ = os.WriteFile(filepath.Join(projDir, "a.go"), []byte(`import "proj/target"\n`), 0o644)
	_ = os.WriteFile(filepath.Join(projDir, "b_test.go"), []byte(`package a\nimport "proj/target"\n`), 0o644)

	logDir := filepath.Join(root, "logs")
	cfg := config.Default()
	cfg.Global.LogDir = logDir
	spaces, _ := space.Load(t.TempDir())
	_ = spaces.Add(&space.Manifest{
		Name: "proj", Type: space.TypeProject, Scope: []string{projDir},
		Tools: []string{"file"}, Perms: space.Perms{Read: true, Write: true},
	})
	o := &Options{Cfg: cfgVal(cfg), Spaces: spaces}

	// 有引用：target.go 被 a.go / b_test.go 引用 → RefCount≥2，HasTest=true
	it := contract.Intent{Intent: contract.IntentEdit, Space: "proj",
		Params: map[string]string{"object": "target"}}
	imp := o.mechanicalImpact(it)
	if imp.RefCount < 2 {
		t.Fatalf("有引用时 RefCount 应≥2, got %d", imp.RefCount)
	}
	if !imp.HasTest {
		t.Fatal("scope 内有 b_test.go，HasTest 应为 true")
	}

	// 无引用：不存在的符号 → RefCount=0
	it2 := contract.Intent{Intent: contract.IntentEdit, Space: "proj",
		Params: map[string]string{"object": "nonexistent_xyz"}}
	imp2 := o.mechanicalImpact(it2)
	if imp2.RefCount != 0 {
		t.Fatalf("无引用时 RefCount 应=0, got %d", imp2.RefCount)
	}
}

// TestMechanicalImpactExcludesLogDir：log_dir 自身绝不能被算成引用。
func TestMechanicalImpactExcludesLogDir(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "logs")
	_ = os.MkdirAll(logDir, 0o755)
	// 故意把轨迹日志写进 logDir 且含目标字样
	_ = os.WriteFile(filepath.Join(logDir, "trajectory-20260101.jsonl"), []byte("{\"target\":\"leaked_sym\"}\n"), 0o644)

	// 空项目 scope（真实项目目录在别处）
	projDir := t.TempDir()
	spaces, _ := space.Load(t.TempDir())
	_ = spaces.Add(&space.Manifest{
		Name: "proj", Type: space.TypeProject, Scope: []string{projDir},
		Tools: []string{"file"}, Perms: space.Perms{Read: true},
	})
	cfg := config.Default()
	cfg.Global.LogDir = logDir
	o := &Options{Cfg: cfgVal(cfg), Spaces: spaces}
	it := contract.Intent{Intent: contract.IntentQuery, Space: "proj",
		Params: map[string]string{"object": "leaked_sym"}}
	imp := o.mechanicalImpact(it)
	if imp.RefCount != 0 {
		t.Fatalf("log_dir 泄露字样绝不能算引用, RefCount=%d", imp.RefCount)
	}
}

// TestSummaryAggregation（#52）：跑 2 个任务后摘要含任务数/通过率/均值/按域/按归因。
func TestSummaryAggregation(t *testing.T) {
	// 2026-10-04 修复恢复：task_metrics kind 登记 + Space 补齐（摘要按域/按归因/纯管线标注恢复）
	o := testOptions(t, nil)
	if _, err := Run(context.Background(), o, "记一下摘要任务A"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), o, "记一下摘要任务B"); err != nil {
		t.Fatal(err)
	}
	s, err := Summary(o, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"任务数：", "通过率：", "认知闭环：", "按意图", "按域", "按归因"} {
		if !strings.Contains(s, want) {
			t.Fatalf("摘要缺 %q:\n%s", want, s)
		}
	}
}

// TestCacheNeverAutoApprovesHuman（安全回归）：human 级 COMMIT 批准一次后，
// 第二次同 quad 仍必须走 ConfirmFn；ConfirmFn 返回 false 时绝不执行。
func TestCacheNeverAutoApprovesHuman(t *testing.T) {
	o := testOptions(t, nil)
	projDir := t.TempDir()
	_ = o.Spaces.Add(&space.Manifest{
		Name: "proj", Type: space.TypeProject, Scope: []string{projDir},
		Tools: []string{"git", "read", "file"}, Perms: space.Perms{Read: true, Write: true},
	})

	calls := 0
	o.ConfirmFn = func(taskID, q string) (bool, error) {
		calls++
		return true, nil // 第一次放行
	}

	// 第一次 COMMIT：human，走 ConfirmFn，放行。
	out1, err := Run(context.Background(), o, "在 proj 提交所有改动")
	if err != nil {
		t.Fatal(err)
	}
	if out1.Decision.Level != "human" {
		t.Fatalf("首次应 human, got %q", out1.Decision.Level)
	}
	if calls != 1 {
		t.Fatalf("首次应调 ConfirmFn 一次, calls=%d", calls)
	}
	if !out1.Confirmed {
		t.Fatal("首次应放行")
	}

	// 第二次同 quad：若 human 被错误缓存，ConfirmFn 不会再被调。
	// 改 ConfirmFn 返回 false——若被缓存绕过，仍会 approved=true 并执行（bug）。
	o.ConfirmFn = func(taskID, q string) (bool, error) {
		calls++
		return false, nil
	}
	out2, err := Run(context.Background(), o, "在 proj 提交所有改动")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("human 第二次仍必须走 ConfirmFn（不得被缓存跳过）, calls=%d", calls)
	}
	if out2.Confirmed {
		t.Fatal("ConfirmFn 返回 false 时不得执行——human 缓存绕过 bug 复现")
	}
	if len(out2.Receipts) != 0 {
		t.Fatal("未放行不得产生执行回执")
	}
}

// TestSummaryNetExcludesNoLLM（M4-1 ④）：纯管线 NOTE 不计入 Net 均值。
func TestSummaryNetExcludesNoLLM(t *testing.T) {
	// 2026-10-04 修复恢复：task_metrics 行已写入轨迹（此前 kind 未登记被拒）
	o := testOptions(t, nil)
	if _, err := Run(context.Background(), o, "记一下纯管线A"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), o, "记一下纯管线B"); err != nil {
		t.Fatal(err)
	}
	s, err := Summary(o, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "纯管线任务") || !strings.Contains(s, "不计入 Net 均值") {
		t.Fatalf("应标注纯管线任务不计入 Net: %s", s)
	}
}

func cfgVal(c config.Config) *config.Config { return &c }

func TestPipelineReceiptFourLines(t *testing.T) {
	o := testOptions(t, nil)
	out, err := Run(context.Background(), o, "记一下四行回执")
	if err != nil {
		t.Fatal(err)
	}
	rendered := strings.Join([]string{
		"动作：" + out.View.Action, "文件：" + out.View.Files,
		"结果：" + out.View.Result, "撤销：" + out.View.Undo,
	}, "\n")
	for _, want := range []string{"动作：", "文件：", "结果：", "撤销："} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("回执缺 %q:\n%s", want, rendered)
		}
	}
}

// TestReceiptShowsBackupPath（M4-3 ②）：第二次 NOTE 追加后回执撤销行显示具体 .bak 文件名。
func TestReceiptShowsBackupPath(t *testing.T) {
	o := testOptions(t, nil)

	// 第一次：创建 notes.md（无旧内容，无备份）。
	if _, err := Run(context.Background(), o, "记一下 first"); err != nil {
		t.Fatal(err)
	}
	// 第二次：追加（旧内容存在 → 触发 VHS_BACKUP_PATH）。
	out, err := Run(context.Background(), o, "记一下 second")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.View.Undo, ".bak") {
		t.Fatalf("第二次 NOTE 撤销行应含 .bak 文件名, got %q", out.View.Undo)
	}
	if strings.Contains(out.View.Undo, "备份目录") {
		t.Fatalf("不应再写笼统的备份目录: %q", out.View.Undo)
	}
}

// TestAskOptionsStructured（M4-3 ① → Codex 2026-10-02 改版）：候选按意图动态生成，形状 [{id,label}]。
func TestAskOptionsStructured(t *testing.T) {
	// EDIT 歧义候选 = refer 目标文件（结构化 [{id,label}]）
	opts := optionsForIntent(&contract.Intent{Intent: contract.IntentEdit, Confidence: 0.85},
		[]refer.Option{{ID: "file-a.go", Label: "file-a.go"}, {ID: "file-b.go", Label: "file-b.go"}})
	if len(opts) != 2 {
		t.Fatalf("EDIT 候选应为 refer 文件数, got %d", len(opts))
	}
	for _, o := range opts {
		if o.ID == "" || o.Label == "" {
			t.Fatalf("候选 id/label 不得为空: %+v", o)
		}
	}
}

// TestGitCommitInProjectRoot（M4-4）：temp git 项目注册 project 域，COMMIT → 项目根真实出现新提交。
func TestGitCommitInProjectRoot(t *testing.T) {
	o := testOptions(t, nil)
	projDir := t.TempDir()
	// init git 仓 + 一个初始提交
	mustRun := func(args ...string) string {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = projDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", args, out)
		}
		return string(out)
	}
	mustRun("git", "init", "-q")
	// ⚠️ 必须用 **--local**（2026-10-03 CI 事故）：
	// 产品代码 Run() 的 `git commit`（pipeline.go cm.Dir=root）**没有 -c 注入**，
	// 它依赖仓库的 user 配置 ⇒ 在**没有全局身份的 CI runner** 上会
	// `fatal: empty ident name` ⇒ TestGitCommitInProjectRoot 失败。
	// 而 `--local` **禁止上溯父仓库** ⇒ 既提供身份，又不会污染主仓库 .git/config。
	mustRun("git", "config", "--local", "user.email", "vhs@test")
	mustRun("git", "config", "--local", "user.name", "vhs")
	if err := os.WriteFile(filepath.Join(projDir, "init.txt"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun("git", "add", "-A")
	mustRun("git", "-c", "user.email=vhs@test", "-c", "user.name=vhs", "commit", "-q", "-m", "init")
	before := strings.TrimSpace(mustRun("git", "log", "-1", "--format=%H"))

	// 制造未提交改动
	if err := os.WriteFile(filepath.Join(projDir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_ = o.Spaces.Add(&space.Manifest{
		Name: "proj", Type: space.TypeProject, Scope: []string{projDir},
		Tools: []string{"git", "read", "file"}, Perms: space.Perms{Read: true, Write: true},
	})

	question := ""
	o.ConfirmFn = func(taskID, q string) (bool, error) {
		question = q
		return true, nil
	}
	out, err := Run(context.Background(), o, "在 proj 提交所有改动")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Confirmed {
		t.Fatal("应放行")
	}
	if !strings.Contains(question, "未提交改动") {
		t.Fatalf("确认问题应含未提交改动数: %q", question)
	}
	after := strings.TrimSpace(mustRun("git", "log", "-1", "--format=%H"))
	if after == "" || after == before {
		t.Fatalf("应产生新提交: before=%s after=%s", before, after)
	}
	// 历史未改写：init 提交仍在
	hist := mustRun("git", "log", "--oneline")
	if !strings.Contains(hist, "init") {
		t.Fatalf("历史应保留 init 提交: %s", hist)
	}
}

// TestCommitRefusedNoExec（M4-4）：拒绝 COMMIT → 无新提交。
func TestCommitRefusedNoExec(t *testing.T) {
	o := testOptions(t, nil)
	projDir := t.TempDir()
	mustRun := func(args ...string) string {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = projDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", args, out)
		}
		return string(out)
	}
	mustRun("git", "init", "-q")
	// ⚠️ 必须用 **--local**（2026-10-03 CI 事故）：
	// 产品代码 Run() 的 `git commit`（pipeline.go cm.Dir=root）**没有 -c 注入**，
	// 它依赖仓库的 user 配置 ⇒ 在**没有全局身份的 CI runner** 上会
	// `fatal: empty ident name` ⇒ TestGitCommitInProjectRoot 失败。
	// 而 `--local` **禁止上溯父仓库** ⇒ 既提供身份，又不会污染主仓库 .git/config。
	mustRun("git", "config", "--local", "user.email", "vhs@test")
	mustRun("git", "config", "--local", "user.name", "vhs")
	_ = os.WriteFile(filepath.Join(projDir, "a.txt"), []byte("a\n"), 0o644)
	mustRun("git", "add", "-A")
	mustRun("git", "-c", "user.email=vhs@test", "-c", "user.name=vhs", "commit", "-q", "-m", "init")
	before := strings.TrimSpace(mustRun("git", "log", "-1", "--format=%H"))

	_ = o.Spaces.Add(&space.Manifest{
		Name: "proj", Type: space.TypeProject, Scope: []string{projDir},
		Tools: []string{"git", "read", "file"}, Perms: space.Perms{Read: true, Write: true},
	})
	o.ConfirmFn = func(taskID, q string) (bool, error) { return false, nil }

	out, err := Run(context.Background(), o, "在 proj 提交所有改动")
	if err != nil {
		t.Fatal(err)
	}
	if out.Confirmed {
		t.Fatal("应拒绝")
	}
	after := strings.TrimSpace(mustRun("git", "log", "-1", "--format=%H"))
	if after != before {
		t.Fatalf("拒绝后不应有新提交: before=%s after=%s", before, after)
	}
}

// TestAskOptionsIncludeReferCandidates（M4-5）：意图候选 + refer 候选合并去重。
func TestAskOptionsIncludeReferCandidates(t *testing.T) {
	base := optionsForIntent(&contract.Intent{Intent: contract.IntentEdit, Confidence: 0.85}, nil)
	referOpts := []refer.Option{
		{ID: "dict:那个", Label: "词典：那个 → notes"},
		{ID: "rec:orders.go", Label: "最近实体：orders.go"},
		{ID: "edit", Label: "重复 id 应被去重"},
	}
	merged := mergeAskOptions(base, referOpts)
	if len(merged) != 3 {
		t.Fatalf("合并后应为 refer 候选去重数 3（base 为空——EDIT 无 refer 目标不塞固定项），got %d: %+v", len(merged), merged)
	}
	ids := map[string]bool{}
	for _, o := range merged {
		if ids[o.ID] {
			t.Fatalf("id 重复: %s", o.ID)
		}
		ids[o.ID] = true
	}
	// 固定意图候选已废除（Codex 2026-10-02：options 按意图动态生成）——只验证 refer 候选去重
	// 必含 refer 候选
	for _, want := range []string{"dict:那个", "rec:orders.go"} {
		if !ids[want] {
			t.Fatalf("应含 refer 候选 %s: %+v", want, merged)
		}
	}
	// 空 refer 候选 → 仅意图候选
	onlyIntent := mergeAskOptions(base, nil)
	if len(onlyIntent) != len(base) {
		t.Fatalf("无 refer 候选时应仅返回意图候选: %+v", onlyIntent)
	}
}

// TestQueryHighConfidenceSkipsReferAsk（M7，外部模型诊断方案 2026-10-02）：
// QUERY 高置信（>=0.8）且含"这个"口语代词 → 跳过 refer 指代消解 → 不 need_ask。
// 修复前：refer 命中"这个"、候选为空 → 写 Ask"你说的「这个」指的是哪个？"→ need_ask（答非所问）。
func TestQueryHighConfidenceSkipsReferAsk(t *testing.T) {
	o := testOptions(t, nil)
	out, err := Run(context.Background(), o, "查一下 这个方案怎么样")
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	if out.Ask != "" {
		t.Fatalf("QUERY 高置信不应 need_ask，got ask=%q（refer 指代消解应被门控跳过）", out.Ask)
	}
}

// TestShouldResolveReferGate（M7 门控纯函数单测，方案来源 Codex/gpt-6-luna 外部诊断；
// 2026-10-04 多轮指代接线新增 hasRecent 参数——有会话上下文时 QUERY 非裸指代也解析）。
func TestShouldResolveReferGate(t *testing.T) {
	cases := []struct {
		name      string
		intent    string
		conf      float64
		text      string
		hasRecent bool
		want      bool
	}{
		{"QUERY 高置信 0.9", contract.IntentQuery, 0.9, "查一下这个方案", false, false},
		{"QUERY 恰好 0.8", contract.IntentQuery, 0.8, "查一下这个方案", false, false},
		{"QUERY 低置信 0.79", contract.IntentQuery, 0.79, "这个", false, true},
		{"NOTE 无问句", contract.IntentNote, 0.85, "记一下 上次那个文件", false, true},
		{"NOTE 含问句（口语代词豁免）", contract.IntentNote, 0.85, "记一下 这个能用吗", false, false},
		{"EDIT", contract.IntentEdit, 0.9, "改一下 那个文件", false, true},
		{"COMMIT", contract.IntentCommit, 0.85, "把改动提交", false, true},
		{"UNKNOWN 无操作动词（陈述引用/元指令）", contract.IntentUnknown, 0.2, "随便看看", false, false},
		{"QUERY 裸指代（真歧义）", contract.IntentQuery, 0.9, "查一下这个", false, true},
		{"QUERY 有实体（不歧义）", contract.IntentQuery, 0.9, "查一下这个方案", false, false},
		{"QUERY 有实体+有上下文（多轮指代接线）", contract.IntentQuery, 0.9, "查一下这个方案", true, true},
		{"UNKNOWN 陈述引用（isNominalMention）", contract.IntentUnknown, 0.2, "我那个前端的问题又不过来", false, false},
		{"DEBUG 操作指代", contract.IntentDebug, 0.85, "修那个", false, true},
		{"QUERY+打开 操作指代", contract.IntentQuery, 0.85, "打开上次那个", false, true},
	}
	for _, c := range cases {
		it := contract.Intent{Intent: c.intent, Confidence: c.conf, CorrectedText: c.text}
		if got := shouldResolveRefer(&it, c.hasRecent); got != c.want {
			t.Errorf("shouldResolveRefer(%s conf=%v text=%q hasRecent=%v) = %v, want %v", c.name, c.conf, c.text, c.hasRecent, got, c.want)
		}
	}
}

// TestColloquialQuestionNoReferAsk（M7 复验补强）：22:04 用户原句——规则可能误判 NOTE，
// 但含问句特征（"效果怎么样"）时 refer 指代消解整体豁免，不因"这个"写 Ask。
func TestColloquialQuestionNoReferAsk(t *testing.T) {
	o := testOptions(t, nil)
	out, err := Run(context.Background(), o, "我现在测试一下，看看效果怎么样，如果这个效果好，我们就继续推进，就是重点是把这个能力建立起来")
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	if out.Ask != "" {
		t.Fatalf("含问句特征的口语陈述不应触发指代 Ask，got ask=%q", out.Ask)
	}
}

// TestCodexNineRegressions — Codex/gpt-6-luna 外部诊断（2026-10-02）9 项回归清单。
// 覆盖：陈述引用抑制 / 操作指代仍 Ask / 裸指代真歧义 / 元指令不 Ask / 既有正例不回归。
func TestCodexNineRegressions(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		wantAsk string // 子串断言；"" 表示断言 Ask 为空
	}{
		{"1-本实例不弹那个指哪个", "我现在想认真开始测，测完了之后能把这个哈你真的开始推进起来，我那个前端的问题又不过来", "指的是哪个"},
		{"2-陈述引用不Ask（断言不含'指的是哪个'）", "我那个前端的问题又不过来", "NOT:指的是哪个"},
		{"3-操作指代仍Ask", "把那个前端文件改一下", "指的是哪个"},
		{"4-把上次那个改成蓝色操作指代", "把上次那个改成蓝色", "那个"},
		{"5-EDIT真歧义候选无固定项", "把那个前端文件改一下", "指的是哪个"},
		{"6-QUERY裸指代真歧义", "查一下这个", "指的是哪个"},
		{"7-NOTE真歧义Ask（'记一下 这个'内容歧义）", "记一下 这个", "指的是哪个"},
		{"8-元指令控制组不Ask", "我想开始认真测一下，接下来把项目推进起来", ""},
		{"9a-修那个正例", "修那个", "指的是哪个"},
		{"9b-改那个文件正例", "改那个文件", "指的是哪个"},
		{"9c-打开上次那个正例", "打开上次那个", "指的是哪个"},
	}
	for _, c := range cases {
		o := testOptions(t, nil)
		out, err := Run(context.Background(), o, c.text)
		if err != nil {
			t.Fatalf("[%s] Run err: %v", c.name, err)
		}
		if c.wantAsk == "" {
			if out.Ask != "" {
				t.Errorf("[%s] 预期不 Ask，got ask=%q", c.name, out.Ask)
			}
		} else if strings.HasPrefix(c.wantAsk, "NOT:") {
			notWant := strings.TrimPrefix(c.wantAsk, "NOT:")
			if strings.Contains(out.Ask, notWant) {
				t.Errorf("[%s] 预期 ask 不含 %q，got ask=%q", c.name, notWant, out.Ask)
			}
		} else if !strings.Contains(out.Ask, c.wantAsk) {
			t.Errorf("[%s] 预期 ask 含 %q，got ask=%q", c.name, c.wantAsk, out.Ask)
		}
		// 固定候选不得出现（Codex：options 按意图生成，不塞"改文件/查代码/记想法/提交"）
		for _, oo := range out.Options {
			if oo.ID == "edit" || oo.ID == "query" || oo.ID == "note" || oo.ID == "commit" {
				t.Errorf("[%s] options 出现固定候选 id=%s（应为按意图动态生成）", c.name, oo.ID)
			}
		}
	}
}

// TestCodexOptionsForIntent — EDIT 歧义候选只含 refer 目标文件（Codex 第 5 项）。
func TestCodexOptionsForIntent(t *testing.T) {
	it := contract.Intent{Intent: contract.IntentEdit, Confidence: 0.85}
	opts := optionsForIntent(&it, []refer.Option{{ID: "file-a.go", Label: "file-a.go"}})
	if len(opts) != 1 || opts[0].ID != "file-a.go" {
		t.Fatalf("EDIT 候选应为 refer 文件，got %+v", opts)
	}
	if opts := optionsForIntent(&it, nil); len(opts) != 0 {
		t.Fatalf("EDIT 无 refer 候选时应为 nil，got %+v", opts)
	}
	q := contract.Intent{Intent: contract.IntentQuery, Confidence: 0.9}
	if opts := optionsForIntent(&q, nil); len(opts) != 0 {
		t.Fatalf("QUERY 无候选时应为 nil，got %+v", opts)
	}
}

// TestQueryLLMAnswerDegradedUnchanged（回归）：Providers=nil（testOptions 现状）→
// 诊断层零开销跳过，QUERY 走逐字降级文案（"今日预算可能已用尽或网络异常"）。
func TestQueryLLMAnswerDegradedUnchanged(t *testing.T) {
	o := testOptions(t, nil) // Providers 未设 → nil
	out, err := Run(context.Background(), o, "查一下 订单系统怎么样")
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, r := range out.Receipts {
		joined += r.Stdout
	}
	// **判据升版（由 Lead 发起，理由见 Issue #4 comment）**：
	// 原文断言"含'今日预算可能已用尽或网络异常'"—— 那是在**保护一个错误的归因**：
	// 真因可能是 HTTP 401 invalid_api_key，用户看了会去等明天/查网络。
	// 新文：**归因必须来自真实错误** —— 401 ⇒ 鉴权且**不得**出现"预算/网络"。
	if strings.Contains(joined, "今日预算可能已用尽或网络异常") {
		t.Errorf("降级文案仍套用旧的「预算/网络」归因：%s", joined)
	}
	for _, bad := range []string{"预算", "网络"} {
		if strings.Contains(joined, bad) {
			t.Errorf("降级文案出现 %q（归因错误，应指向真实错误）：%s", bad, joined)
		}
	}
}

// TestQueryLLMAnswerDiagRetrySucceeds（异常自愈接线）：fast 首次 500 → 诊断模型判 retry
// → 指数退避后重试 fast 成功，返回回答（而非降级文案）。诊断请求 failure 带 model=fast。
func TestQueryLLMAnswerDiagRetrySucceeds(t *testing.T) {
	logDir := t.TempDir()
	var fastCalls int32
	var diagRaw strings.Builder

	fastTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&fastCalls, 1)
		// openaiClient 内部对 5xx 重试 3 次：前 3 次都 500 才能让外层 callFast 真正返回错误。
		if n <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"boom"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"订单系统运行正常","finish_reason":"stop"}}],"usage":{}}`)
	}))
	defer fastTS.Close()

	diagTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		diagRaw.Write(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{\"category\":\"network\",\"root_cause\":\"瞬时500\",\"confidence\":0.8,\"recoverable\":true,\"suggestion\":\"退避重试\",\"action\":\"retry\"}"}}],"usage":{}}`)
	}))
	defer diagTS.Close()

	trueVal := true
	cfg := config.Config{
		Global: config.Global{LogDir: logDir, LLMTimeoutMs: 5000, FastResponseMs: 10000},
		Providers: []config.Provider{
			{Name: "fast", Kind: config.OpenAIKind, Endpoint: fastTS.URL, Model: "gpt-test", APIKey: "k", ResponseFormat: &trueVal, Params: map[string]any{"use_max_completion_tokens": true}},
			{Name: "diag", Kind: config.OpenAIKind, Endpoint: diagTS.URL, Model: "jev-diagnose", APIKey: "k", ResponseFormat: &trueVal, Params: map[string]any{"use_max_completion_tokens": true}},
		},
	}
	reg, err := provider.NewRegistry(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	o := &Options{Cfg: &cfg, Providers: reg}

	answer := o.queryLLMAnswer(context.Background(), "查订单", "search-hit-line")
	if !strings.Contains(answer, "订单系统运行正常") {
		t.Fatalf("diag=retry 退避重试成功应返回回答, got %q", answer)
	}
	// 前 3 次耗尽 openaiClient 内部重试 → 外层错误；退避后第 4 次成功。
	if atomic.LoadInt32(&fastCalls) != 4 {
		t.Fatalf("fast 应被调用 4 次（3 次内部 500 + 1 次退避后成功）, got %d", fastCalls)
	}
	// 诊断请求体应含 jev-diagnose 模型名；user content（二次 JSON 串）应带 failure.model=fast。
	var outer struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(diagRaw.String()), &outer); err != nil {
		t.Fatalf("诊断请求体应为 JSON: %v", err)
	}
	if outer.Model != "jev-diagnose" {
		t.Fatalf("诊断请求 model 应为 jev-diagnose, got %q", outer.Model)
	}
	var userContent string
	for _, m := range outer.Messages {
		if m.Role == "user" {
			userContent = m.Content
		}
	}
	if !strings.Contains(userContent, `"model":"fast"`) {
		t.Fatalf("诊断请求 failure 应带 model=fast, got %q", userContent)
	}
}
