package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
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

// TestPipelineWritesAttribution(SPEC useexample 12 / #43):   afterattributionwritetrace + discuss-log. 
func TestPipelineWritesAttribution(t *testing.T) {
	o := testOptions(t, nil)
	out, err := Run(context.Background(), o, "记一下归因测试")
	if err != nil {
		t.Fatal(err)
	}
	// attribution  of 
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
	// discuss.jsonl   
	data, err := os.ReadFile(filepath.Join(o.Cfg.Global.LogDir, "discuss.jsonl"))
	if err != nil {
		t.Fatalf("discuss-log 应存在: %v", err)
	}
	if !strings.Contains(string(data), `"class"`) {
		t.Fatalf("discuss-log 缺归因: %s", data)
	}
	//       
	if out.LoopMs < 0 || out.NetMs < 0 {
		t.Fatalf("LoopMs/NetMs 非负: %d/%d", out.LoopMs, out.NetMs)
	}
}

// TestPipelineOrdering(SPEC #5):       space_check/risk ofbefore; blocki.e.stop. 
func TestPipelineOrdering(t *testing.T) {
	// 2026-10-04 langaudio scenariofix : write intentnoptnamedomaindefault  project(  "  under  /   code"->out-of-scope); 
	// no objroottimeby FAILED recvdatatable , but  global read-onlydomain BOUNDARY_VIOLATION. 
	called := false
	o := testOptions(t, func(string, string) (bool, error) { called = true; return true, nil })
	//  connectsendraise COMMIT(no proj domain)-> default project domain,   be  ; no objroot -> FAILED
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

// TestPipelineConfirmStrategy(useexample 8/9): auto   disconnect; human    . 
func TestPipelineConfirmStrategy(t *testing.T) {
	// auto path: ConfirmFn  becalluse
	autoCalled := false
	o1 := testOptions(t, func(string, string) (bool, error) { autoCalled = true; return true, nil })
	if _, err := Run(context.Background(), o1, "记一下 auto 测试"); err != nil {
		t.Fatal(err)
	}
	if autoCalled {
		t.Fatal("auto 决策不应打断用户")
	}

	// human path: note  proj domainafter COMMIT   
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

// TestPipelineVerifyHooked(useexample 10): NOTE after verify independent verification notes.md   store . 
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

// TestMechanicalImpactRealRefCount: temp  objinhas use vs no use -> RefCount diffdiff. 
func TestMechanicalImpactRealRefCount(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "proj")
	_ = os.MkdirAll(projDir, 0o755)
	// objtgtfile +    use  file
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

	// has use: target.go be a.go / b_test.go  use -> RefCount>=2, HasTest=true
	it := contract.Intent{Intent: contract.IntentEdit, Space: "proj",
		Params: map[string]string{"object": "target"}}
	imp := o.mechanicalImpact(it)
	if imp.RefCount < 2 {
		t.Fatalf("有引用时 RefCount 应≥2, got %d", imp.RefCount)
	}
	if !imp.HasTest {
		t.Fatal("scope 内有 b_test.go，HasTest 应为 true")
	}

	// no use:  store   id -> RefCount=0
	it2 := contract.Intent{Intent: contract.IntentEdit, Space: "proj",
		Params: map[string]string{"object": "nonexistent_xyz"}}
	imp2 := o.mechanicalImpact(it2)
	if imp2.RefCount != 0 {
		t.Fatalf("无引用时 RefCount 应=0, got %d", imp2.RefCount)
	}
}

// TestMechanicalImpactExcludesLogDir: log_dir      be become use. 
func TestMechanicalImpactExcludesLogDir(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "logs")
	_ = os.MkdirAll(logDir, 0o755)
	// thus pipetraceday write  logDir and objtgtcharkind
	_ = os.WriteFile(filepath.Join(logDir, "trajectory-20260101.jsonl"), []byte("{\"target\":\"leaked_sym\"}\n"), 0o644)

	// empty obj scope(   objobj  diffplace)
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

// TestSummaryAggregation(#52):   2  taskafter need tasknum/ edrate/ value/bydomain/byattribution. 
func TestSummaryAggregation(t *testing.T) {
	// 2026-10-04 fix   : task_metrics kind    + Space patch ( needbydomain/byattribution/ managelinetgtnote  )
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

// TestCacheNeverAutoApprovesHuman(safesafetyback ): human   COMMIT approveapprove  after, 
//    same quad      ConfirmFn; ConfirmFn returnback false time    . 
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
		return true, nil //      
	}

	//     COMMIT: human,   ConfirmFn,   . 
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

	//    same quad: if human beerrorcache, ConfirmFn   againbecall. 
	// modify ConfirmFn returnback false--ifbecache ed,    approved=true and  (bug). 
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

// TestSummaryNetExcludesNoLLM(M4-1 ④):  manageline NOTE   in Net  value. 
func TestSummaryNetExcludesNoLLM(t *testing.T) {
	// 2026-10-04 fix   : task_metrics  alreadywritetrace( before kind    bereject)
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

// TestReceiptShowsBackupPath(M4-3 ②):     NOTE   afterback     show body .bak filename. 
func TestReceiptShowsBackupPath(t *testing.T) {
	o := testOptions(t, nil)

	//    :    notes.md(no in , no  ). 
	if _, err := Run(context.Background(), o, "记一下 first"); err != nil {
		t.Fatal(err)
	}
	//    :   ( in store  -> triggersend VHS_BACKUP_PATH). 
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

// TestAskOptionsStructured(M4-3 ① -> Codex 2026-10-02 modify ):   byintent stateoccurbecome,  status [{id,label}]. 
func TestAskOptionsStructured(t *testing.T) {
	// EDIT      = refer objtgtfile(close ize [{id,label}])
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

// TestGitCommitInProjectRoot(M4-4): temp git  objnote  project domain, COMMIT ->  objroot  outnownew  . 
func TestGitCommitInProjectRoot(t *testing.T) {
	o := testOptions(t, nil)
	projDir := t.TempDir()
	// init git   +   initstart  
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
	// ⚠️   use **--local**(2026-10-03 CI  thus): 
	// produce  code Run()   `git commit`(pipeline.go cm.Dir=root)** has -c notein**, 
	//  dependency    user    ⇒  ** hasglobal    CI runner** on 
	// `fatal: empty ident name` ⇒ TestGitCommitInProjectRoot   . 
	// but `--local` **forbidstopon    ** ⇒   provide  , again        .git/config. 
	mustRun("git", "config", "--local", "user.email", "vhs@test")
	mustRun("git", "config", "--local", "user.name", "vhs")
	if err := os.WriteFile(filepath.Join(projDir, "init.txt"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun("git", "add", "-A")
	mustRun("git", "-c", "user.email=vhs@test", "-c", "user.name=vhs", "commit", "-q", "-m", "init")
	before := strings.TrimSpace(mustRun("git", "log", "-1", "--format=%H"))

	// restrict uncommitted changes
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
	if !strings.Contains(question, "uncommitted changes") {
		t.Fatalf("确认问题应含未提交改动数: %q", question)
	}
	after := strings.TrimSpace(mustRun("git", "log", "-1", "--format=%H"))
	if after == "" || after == before {
		t.Fatalf("应产生新提交: before=%s after=%s", before, after)
	}
	//    modifywrite: init     
	hist := mustRun("git", "log", "--oneline")
	if !strings.Contains(hist, "init") {
		t.Fatalf("历史应保留 init 提交: %s", hist)
	}
}

// TestCommitRefusedNoExec(M4-4): reject COMMIT -> nonew  . 
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
	// ⚠️   use **--local**(2026-10-03 CI  thus): 
	// produce  code Run()   `git commit`(pipeline.go cm.Dir=root)** has -c notein**, 
	//  dependency    user    ⇒  ** hasglobal    CI runner** on 
	// `fatal: empty ident name` ⇒ TestGitCommitInProjectRoot   . 
	// but `--local` **forbidstopon    ** ⇒   provide  , again        .git/config. 
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

// TestAskOptionsIncludeReferCandidates(M4-5): intent   + refer    and heavy. 
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
	//   intent  already  (Codex 2026-10-02: options byintent stateoccurbecome)--only   refer    heavy
	//    refer   
	for _, want := range []string{"dict:那个", "rec:orders.go"} {
		if !ids[want] {
			t.Fatalf("应含 refer 候选 %s: %+v", want, merged)
		}
	}
	// empty refer    -> onlyintent  
	onlyIntent := mergeAskOptions(base, nil)
	if len(onlyIntent) != len(base) {
		t.Fatalf("无 refer 候选时应仅返回意图候选: %+v", onlyIntent)
	}
}

// TestQueryHighConfidenceSkipsReferAsk(M7, out  type disconnect   2026-10-02): 
// QUERY    (>=0.8)and "  " lang word ->  ed refer coreference resolution ->   need_ask. 
// fix before: refer  in"  ",   asempty -> write Ask"   "  "refer is   "-> need_ask(    ). 
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

// TestShouldResolveReferGate(M7     num  ,      Codex/gpt-6-luna out  disconnect; 
// 2026-10-04   coreferenceconnectlinenewadd hasRecent  num--has  onunder time QUERY   coreferencealsoresolve ). 
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

// TestColloquialQuestionNoReferAsk(M7   patch ): 22:04 useuserorigsent--rule     NOTE, 
// but  sent  ("    kind")time refer coreference resolution body  ,  because"  "write Ask. 
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

// TestCodexNineRegressions — Codex/gpt-6-luna out  disconnect(2026-10-02)9  back list. 
// overwrite:    use restrict /   coreference  Ask /  coreference    /  refer   Ask /  hasposexample back . 
func TestCodexNineRegressions(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		wantAsk string //   disconnectlang; "" tableshowdisconnectlang Ask asempty
	}{
		{"1-本实例不弹那个指哪个", "我现在想认真开始测，测完了之后能把这个哈你真的开始推进起来，我那个前端的问题又不过来", "Which did you mean"},
		{"2-陈述引用不Ask（断言不含'Which did you mean'）", "我那个前端的问题又不过来", "NOT:Which did you mean"},
		{"3-操作指代仍Ask", "把那个前端文件改一下", "Which did you mean"},
		{"4-把上次那个改成蓝色操作指代", "把上次那个改成蓝色", "那个"},
		{"5-EDIT真歧义候选无固定项", "把那个前端文件改一下", "Which did you mean"},
		{"6-QUERY裸指代真歧义", "查一下这个", "Which did you mean"},
		{"7-NOTE真歧义Ask（'记一下 这个'内容歧义）", "记一下 这个", "Which did you mean"},
		{"8-元指令控制组不Ask", "我想开始认真测一下，接下来把项目推进起来", ""},
		{"9a-修那个正例", "修那个", "Which did you mean"},
		{"9b-改那个文件正例", "改那个文件", "Which did you mean"},
		{"9c-打开上次那个正例", "打开上次那个", "Which did you mean"},
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
		//       outnow(Codex: options byintentoccurbecome,   "modifyfile/  code/   /  ")
		for _, oo := range out.Options {
			if oo.ID == "edit" || oo.ID == "query" || oo.ID == "note" || oo.ID == "commit" {
				t.Errorf("[%s] options 出现固定候选 id=%s（应为按意图动态生成）", c.name, oo.ID)
			}
		}
	}
}

// TestCodexOptionsForIntent — EDIT     only  refer objtgtfile(Codex   5  ). 
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

// TestQueryLLMAnswerDegradedUnchanged(back ): Providers=nil(testOptions nowstatus)->
//  disconnect  open  ed, QUERY   char    (" day    alreadyuse or  error"). 
func TestQueryLLMAnswerDegradedUnchanged(t *testing.T) {
	o := testOptions(t, nil) // Providers    -> nil
	out, err := Run(context.Background(), o, "查一下 订单系统怎么样")
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, r := range out.Receipts {
		joined += r.Stdout
	}
	// ** data  (by Lead sendraise,  bysee Issue #4 comment)**: 
	// orig disconnectlang" ' day    alreadyuse or  error'"--  is **protect  error attribution**: 
	//  because  is HTTP 401 invalid_api_key, useuser   etc day/   . 
	// new : **attribution      error** -- 401 ⇒   and**  **outnow"  /  ". 
	if strings.Contains(joined, "今日预算可能已用尽或网络异常") {
		t.Errorf("降级文案仍套用旧的「预算/网络」归因：%s", joined)
	}
	for _, bad := range []string{"预算", "网络"} {
		if strings.Contains(joined, bad) {
			t.Errorf("降级文案出现 %q（归因错误，应指向真实错误）：%s", bad, joined)
		}
	}
}

// TestQueryLLMAnswerDiagRetrySucceeds(error  connectline): fast first  500 ->  disconnect type  retry
// -> refernum  afterheavy  fast become , returnbackanswer(but     ).  disconnect require failure   model=fast. 
func TestQueryLLMAnswerDiagRetrySucceeds(t *testing.T) {
	logDir := t.TempDir()
	var fastCalls int32
	var diagRaw strings.Builder

	fastTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&fastCalls, 1)
		// openaiClient in to 5xx heavy  3  : before 3  all 500 only  out  callFast  posreturnbackerror. 
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
	// before 3     openaiClient in heavy  -> out error;   after  4  become . 
	if atomic.LoadInt32(&fastCalls) != 4 {
		t.Fatalf("fast 应被调用 4 次（3 次内部 500 + 1 次退避后成功）, got %d", fastCalls)
	}
	//  disconnect requirebody   jev-diagnose  typename; user content(   JSON  )   failure.model=fast. 
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

// TestRequestIDPassthroughAndTrajectory(P0-4b): 
//
//	① Options.RequestID empty -> Run  occurbecome(= nowstatus as, toaftercompat); 
//	② refer  RequestID -> out.RequestID etcat invalue, andwrite    trace Entry   request_id allis . 
func TestRequestIDPassthroughAndTrajectory(t *testing.T) {
	// ① empty ->  occurbecome
	o1 := testOptions(t, nil)
	out1, err := Run(context.Background(), o1, "记一下 rid 自生成")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out1.RequestID, "req-") {
		t.Fatalf("空 RequestID 应自生成 req-*，实际 %q", out1.RequestID)
	}

	// ② refer  ->    + trace Entry   
	o2 := testOptions(t, nil)
	o2.RequestID = "client-req-xyz"
	out2, err := Run(context.Background(), o2, "记一下 rid 指定")
	if err != nil {
		t.Fatal(err)
	}
	if out2.RequestID != "client-req-xyz" {
		t.Fatalf("指定 RequestID 应透传为 out.RequestID，实际 %q", out2.RequestID)
	}

	entries, _ := filepath.Glob(filepath.Join(o2.Cfg.Global.LogDir, "trajectory-*.jsonl"))
	if len(entries) == 0 {
		t.Fatal("未找到轨迹文件")
	}
	data, err := os.ReadFile(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	var bad []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("轨迹行不可解析: %v: %s", err, line)
		}
		if rid, _ := e["request_id"].(string); rid != "client-req-xyz" {
			bad = append(bad, fmt.Sprintf("kind=%v rid=%q", e["kind"], rid))
		}
	}
	if len(bad) > 0 {
		t.Fatalf("轨迹里 %d 条 Entry 的 request_id != client-req-xyz: %v", len(bad), bad)
	}
}

// TestSerialGateSharedAcrossClones    C0 serial gate taskoccur : 
// EnableSerialGate after,      and    same pipe *sync.Mutex(server  task o:=*tmpl). 
// revtoto :   EnableSerialGate time   mu  as nil(i.e. before     bug  state). 
func TestSerialGateSharedAcrossClones(t *testing.T) {
	tmpl := testOptions(t, nil)
	tmpl.EnableSerialGate()
	if tmpl.mu == nil {
		t.Fatal("EnableSerialGate 后模板 mu 应非 nil")
	}
	c1 := *tmpl
	c2 := *tmpl
	if c1.mu != tmpl.mu || c2.mu != tmpl.mu {
		t.Fatal("克隆必须与模板共享同一把串行闸指针（跨任务串行生效）")
	}

	// to :  initstartize  Options,    after mu  as nil(Run will    new  ->  serial)
	fresh := testOptions(t, nil)
	if fresh.mu != nil {
		t.Fatal("新 Options 未经 EnableSerialGate，mu 应为 nil")
	}
	cf := *fresh
	if fresh.mu != nil || cf.mu != nil {
		t.Fatal("对照克隆 mu 应保持 nil")
	}
}
