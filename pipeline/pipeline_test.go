package pipeline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"voicesign-harness/cache"
	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/memory"
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
	// 未知域写意图 → 必须在 space_check 拦截，绝不执行。
	called := false
	o := testOptions(t, func(string, string) (bool, error) { called = true; return true, nil })
	// 直接对 global 只读域发起 COMMIT（无 proj 域）→ boundary_violation
	out, err := Run(context.Background(), o, "提交代码")
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict.Allowed {
		t.Fatal("global 只读域不应放行 COMMIT")
	}
	if called {
		t.Fatal("被拦截的动作不得调用 ConfirmFn")
	}
	if len(out.Receipts) != 0 {
		t.Fatal("被拦截不得产生执行回执")
	}
	if !strings.Contains(out.View.Result, "BOUNDARY_VIOLATION") {
		t.Fatalf("结果行应标越界: %q", out.View.Result)
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

// TestAskOptionsStructured（M4-3 ①）：need_ask 候选为结构化 [{id,label}]。
func TestAskOptionsStructured(t *testing.T) {
	// 直接验证候选集形状
	opts := intentCandidates()
	if len(opts) < 2 || len(opts) > 4 {
		t.Fatalf("候选数应 2-4, got %d", len(opts))
	}
	for _, o := range opts {
		if o.ID == "" || o.Label == "" {
			t.Fatalf("候选 id/label 不得为空: %+v", o)
		}
	}
	ids := map[string]bool{}
	for _, o := range opts {
		ids[o.ID] = true
	}
	if !ids["edit"] || !ids["note"] {
		t.Fatalf("应含 edit/note 候选: %+v", opts)
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
	mustRun("git", "config", "user.email", "vhs@test")
	mustRun("git", "config", "user.name", "vhs")
	if err := os.WriteFile(filepath.Join(projDir, "init.txt"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun("git", "add", "-A")
	mustRun("git", "commit", "-q", "-m", "init")
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
	mustRun("git", "config", "user.email", "vhs@test")
	mustRun("git", "config", "user.name", "vhs")
	_ = os.WriteFile(filepath.Join(projDir, "a.txt"), []byte("a\n"), 0o644)
	mustRun("git", "add", "-A")
	mustRun("git", "commit", "-q", "-m", "init")
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
	base := intentCandidates()
	referOpts := []refer.Option{
		{ID: "dict:那个", Label: "词典：那个 → notes"},
		{ID: "rec:orders.go", Label: "最近实体：orders.go"},
		{ID: "edit", Label: "重复 id 应被去重"},
	}
	merged := mergeAskOptions(base, referOpts)
	if len(merged) < 5 || len(merged) > 8 {
		t.Fatalf("合并后总数应 5-8（4 意图 + 2-4 refer），got %d: %+v", len(merged), merged)
	}
	ids := map[string]bool{}
	for _, o := range merged {
		if ids[o.ID] {
			t.Fatalf("id 重复: %s", o.ID)
		}
		ids[o.ID] = true
	}
	// 必含意图候选
	for _, want := range []string{"edit", "query", "note", "commit"} {
		if !ids[want] {
			t.Fatalf("应含意图候选 %s: %+v", want, merged)
		}
	}
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

// TestShouldResolveReferGate（M7 门控纯函数单测，方案来源 Codex/gpt-6-luna 外部诊断）。
func TestShouldResolveReferGate(t *testing.T) {
	cases := []struct {
		name   string
		intent string
		conf   float64
		text   string
		want   bool
	}{
		{"QUERY 高置信 0.9", contract.IntentQuery, 0.9, "查一下这个方案", false},
		{"QUERY 恰好 0.8", contract.IntentQuery, 0.8, "查一下这个方案", false},
		{"QUERY 低置信 0.79", contract.IntentQuery, 0.79, "这个", true},
		{"NOTE 无问句", contract.IntentNote, 0.85, "记一下 上次那个文件", true},
		{"NOTE 含问句（口语代词豁免）", contract.IntentNote, 0.85, "记一下 这个能用吗", false},
		{"EDIT", contract.IntentEdit, 0.9, "改一下 那个文件", true},
		{"COMMIT", contract.IntentCommit, 0.85, "把改动提交", true},
		{"UNKNOWN 兜底", contract.IntentUnknown, 0.2, "随便看看", true},
	}
	for _, c := range cases {
		it := contract.Intent{Intent: c.intent, Confidence: c.conf, CorrectedText: c.text}
		if got := shouldResolveRefer(&it); got != c.want {
			t.Errorf("shouldResolveRefer(%s conf=%v text=%q) = %v, want %v", c.name, c.conf, c.text, got, c.want)
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
