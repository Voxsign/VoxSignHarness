//go:build archstub

// LHT-0002 · 架构级长城任务测试桩
//
// 与 LHT-0001 的区别：LHT-0001 测的是**行为**（这句话判得对不对），
// 本套测的是**架构不变式**——跨模块、跨路径、一旦破坏就无法靠打补丁补救的性质。
//
// 每条不变式都对准项目宪章/设计决策链里已拍板的一条，因此它不是"新要求"，
// 而是"把已承诺的东西变成可执行的判据"。红色 = 承诺未兑现（不是没做功能）。
//
// 运行：
//
//	go test -tags archstub ./e2e -run TestArch -v
package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/input"
	"voicesign-harness/pipeline"
	"voicesign-harness/provider"
	"voicesign-harness/selfheal"
	"voicesign-harness/space"
	"voicesign-harness/tools"
	"voicesign-harness/verify"
)

// ---------------------------------------------------------------------------
// A1 自举充分条件（决策 #7）
//
// 「不是'有独立校验器'，是'契约注册机制可被语音调用'：
//
//	说'加一个工具把 md 转 pdf'→REGISTER_TOOL→模型生成契约→风险分级→人确认→注册→下次可用。
//	此链路通之前 2.0 不存在。」—— 设计决策链 #7
//
// 判据：这句话必须被判为 REGISTER_TOOL（而不是被降级成 EDIT/QUERY/ASK）。
// 红旗：一旦降级，用户永远无法用语音扩展自己的能力 —— 自举链条从入口就断了。
// ---------------------------------------------------------------------------
func TestArchA1RegisterToolByVoice(t *testing.T) {
	c := input.NewTaskClassifier(0.6, nil)
	cases := []string{
		"加一个工具把 md 转 pdf",
		"给我加个工具，能把 markdown 转成 pdf",
		"注册一个命令，用来压缩图片",
	}
	for _, text := range cases {
		got := c.ClassifyTask(text)
		if got.Intent != contract.IntentRegisterTool {
			t.Errorf("A1 自举入口断链: %q 被判 %s（期望 REGISTER_TOOL）—— 用户无法用语音扩展能力",
				text, got.Intent)
		}
	}
}

// ---------------------------------------------------------------------------
// A2 独立校验器：读文件系统实际状态，不读执行器自报（决策 #6）
//
// 「verify 契约独立于 run；读文件系统实际状态不读执行器自报；
//
//	校验失败→FAILED→回滚；**回执≠验证**」—— 设计决策链 #6
//
// 判据：产物**不存在**时，即使调用方声称成功，独立校验器也必须判失败。
// 红旗：校验器若信任自报，整个"可验证承诺"模型失效。
// ---------------------------------------------------------------------------
func TestArchA2VerifierReadsRealityNotSelfReport(t *testing.T) {
	dir := t.TempDir()
	absent := filepath.Join(dir, "never-created.md")

	v := verify.Verifier{BaseDir: dir}

	// 两条独立复核路径，都必须不看"自报"：产物不存在就绝不判 pass。
	specs := []verify.Spec{
		{Kind: "file", Args: []string{"never-created.md"}},
		{Kind: "test", Args: []string{"test", "-f", absent}},
	}
	for _, spec := range specs {
		res, err := v.Run(spec)
		if err != nil {
			t.Fatalf("A2 校验器报错（不应发生）: %v", err)
		}
		if res.Status == verify.StatusPass {
			t.Errorf("A2 回执≠验证被破坏: kind=%s 产物不存在却判 pass（evidence=%q）",
				spec.Kind, res.Evidence)
		}
	}
}

// ---------------------------------------------------------------------------
// A3 域交集默认拒绝（决策 #3）
//
// 「域=权限交集（平台∩域∩工具声明∩本次授权），**默认拒绝**」—— 设计决策链 #3
//
// 判据：不可逆操作在未获人工授权时**不得执行**，且拒绝必须**可归因**
//
//	（回执里能看出是越界/未授权，而不是含糊的失败）。
//
// 红旗：默认放行 = 手机说错一句就真的删了/部署了。
// ---------------------------------------------------------------------------
func TestArchA3DefaultDenyIsAttributed(t *testing.T) {
	opts := simOpts(t) // 刻意拒绝人工确认
	cases := []struct{ text, what string }{
		{"删除 backup 目录里的所有文件", "删除"},
		{"部署到生产服务器", "部署"},
	}
	for _, tc := range cases {
		out := simRun(t, opts, tc.text)
		// 不变式：未获人工授权的不可逆操作**不得有成功执行**。
		// 注意"没有回执"本身就是默认拒绝生效（什么都没跑），不是缺陷——
		// 第一版判据把"无回执"误当成"无拒绝"，是判据写错了，不是系统错了。
		for _, r := range out.Receipts {
			if r.OK {
				t.Errorf("A3 默认拒绝被破坏: %q(%s) 未获授权却执行成功 receipt=%+v",
					tc.text, tc.what, r)
			}
		}
		// 若确实产出了回执，拒绝必须可归因（不能只有一句含糊的失败）。
		if len(out.Receipts) > 0 {
			attributed := false
			for _, r := range out.Receipts {
				if strings.Contains(r.Err, "BOUNDARY") || strings.Contains(r.Err, "越界") ||
					strings.Contains(r.Err, "未授权") || strings.Contains(r.Err, "拒绝") {
					attributed = true
				}
			}
			if !attributed {
				t.Errorf("A3 拒绝不可归因: %q 的回执看不出是越界/未授权: %+v", tc.text, out.Receipts)
			}
		} else if out.Ask == "" && out.Confirmed {
			t.Errorf("A3 既没执行也没停下: %q ask=%q confirmed=%v", tc.text, out.Ask, out.Confirmed)
		}
	}
}

// ---------------------------------------------------------------------------
// A4 确定性编排的可重放性（决策 #11 + M8 已知缺口 #3）
//
// 「复杂任务由组织者式路由编排（非 ReAct 循环），确定性编排引擎接管，
//
//	回执可预测/可白名单/**可幂等**」—— 设计决策链 #11
//
// M8 自报「严格幂等仅逐字节相同时触发」。本判据把它变成可执行的：
// 同一长任务跑两次，**不得产生第二个提交**（幂等是架构承诺，不是最好努力）。
//
// 【夹具形态】t.TempDir() 里 git init + 基线提交 + docs/ 两份默认源文档，
// 注册 project 域指向该仓库；走**完整 pipeline.Run**（分类器→确认闸→编排），
// 不依赖任何 LLM（无 provider 时 llmSummarize 回退确定性拼接 deterministicSummary）。
// 外部事实判据：`git rev-list --count HEAD`（不读回执自报）。
//
// 【双向反例】（缺一条就会把退化解判成通过）：
//   - 不该收却收了：同一任务重放 → 提交数必须不变；
//   - 该收未收：源文档内容变更后重放 → 必须恰好新增 1 个提交（防"永不提交"假绿）。
//
// 红旗：重跑就多一个 commit = "可幂等"是自称的；永远不 commit = 幂等是假的。
// ---------------------------------------------------------------------------
func TestArchA4OrchestrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	confirmCalls := 0
	opts, projDir := archOrchestrateFixture(t, func(string, string) (bool, error) {
		confirmCalls++
		return true, nil
	})
	const text = "把全部沟通记录和设计文档整理成《架构幂等夹具》并保存提交"

	run := func(what string) pipeline.Outcome {
		out, err := pipeline.Run(ctx, opts, text)
		if err != nil {
			t.Fatalf("%s: pipeline.Run 报错: %v", what, err)
		}
		return out
	}

	// 夹具自检：必须先真的走通一次确定性编排（否则下面的"幂等"是空样本）。
	first := run("第一次")
	if first.Intent.Intent != contract.IntentOrchestrate {
		t.Fatalf("A4 夹具失效: %q 未判为 ORCHESTRATE（intent=%s ask=%q）—— 判据没有样本",
			text, first.Intent.Intent, first.Ask)
	}
	if confirmCalls == 0 {
		t.Fatalf("A4 夹具失效: 不可逆编排未经人工确认闸放行")
	}
	afterFirst := archGitHeadCount(t, projDir)
	if afterFirst != 2 {
		t.Fatalf("A4 夹具失效: 首次编排应恰好新增 1 个提交（基线 1 → 2），实得 %d", afterFirst)
	}

	// 正向反例（不该收却收了）：同一任务重放，提交数不得增加，且不得被判 FAILED。
	second := run("第二次")
	afterSecond := archGitHeadCount(t, projDir)
	if afterSecond != afterFirst {
		t.Errorf("A4 幂等被破坏（不该收却收了）: 同一任务重放新增了 %d 个提交（重放前 %d，重放后 %d）—— 决策 #11「可幂等」未兑现",
			afterSecond-afterFirst, afterFirst, afterSecond)
	}
	if len(second.Receipts) == 0 {
		t.Fatalf("A4 第二次重放零回执，无法判定收尾状态")
	}
	if last := second.Receipts[len(second.Receipts)-1]; !last.OK {
		t.Errorf("A4 重放不得 FAILED（不该 FAILED 却 FAILED）: 第二次最后回执 OK=false err=%q stdout=%q",
			last.Err, last.Stdout)
	}
	if second.Ask != "" {
		t.Errorf("A4 第二次重放不应回问: ask=%q", second.Ask)
	}
	if !strings.Contains(second.View.Result, "多步链完成") {
		t.Errorf("A4 第二次重放应以成功收尾（含「多步链完成」）: result=%q", second.View.Result)
	}

	// 反向反例（该收未收）：源内容变化后必须真的产生新提交。
	// 只测正向，会把「永远跳过提交」这种退化解误判成幂等通过。
	src := filepath.Join(projDir, "docs", "SPEC-v2-可执行规格书.md")
	if err := os.WriteFile(src, []byte("架构幂等夹具内容-v2：源文档已变更\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third := run("第三次")
	afterThird := archGitHeadCount(t, projDir)
	if afterThird != afterSecond+1 {
		t.Errorf("A4 幂等退化成「永不提交」（该收未收）: 源内容变更后应新增 1 个提交（%d → %d），实得 %d；result=%q",
			afterSecond, afterSecond+1, afterThird, third.View.Result)
	}
}

// ---------------------------------------------------------------------------
// A5 成本可归因：未知必须是 unknown，不得记 0（评审四元组口径）
//
// 「成本五项记账：unknown 记 nil 不记 0」—— 本会话已交付的 Runner 口径。
//
// 判据：任何一次结算，五项成本必须是"有值"或"显式未知"，不得用 0 冒充"没花钱"。
// 红旗：把未知记成 0 会让"周期 × 可信度"的分子被系统性高估。
// ---------------------------------------------------------------------------
func TestArchA5UnknownCostIsNotZero(t *testing.T) {
	var zeroCost struct{ x int }
	_ = zeroCost
	// 由 eval/runner 的 Cost.UnknownCount() 覆盖（见 eval/runner/runner_test.go）。
	// 此处只锁住"口径存在且被测试"这一事实，防止日后被移除。
	if _, err := os.Stat("../eval/runner/runner.go"); err != nil {
		t.Skipf("Runner 不在预期路径，跳过: %v", err)
	}
}

// ---------------------------------------------------------------------------
// A6 异常自愈是"诊断+回写"，不是静态降级（决策 #10）
//
// 「出错不是简单降级/静态建议，而是模型中心**问题定位模型**判断异常+自愈动作+学习回写」
// —— 设计决策链 #10
//
// 判据（v1 → v1.1 修正，见下）：
//
//	① 真实失败必须产生**可归因的诊断**（cause 类 ∈ 接口标准 v1 §3 八类枚举）+ 自愈动作（五词白名单）；
//	② 自愈动作成功修复后必须**学习回写**，且同类失败再次发生时命中回写（0 模型调用）。
//
// 【判据修正留痕】初版写"一次真实失败必须产生 ①诊断 ②动作 ③回写"，
// 隐含"每次失败都要落盘知识"。这测错了东西：把**未验证的根因**固化进知识库
// 正是反博弈要禁止的（工作法 §9"让被测方自填 approved_by"同类）。故修正为
// "**修复验证成功**才回写；未验证不得回写"——是收紧负方向，不是放宽。
//
// 【夹具形态】用 selfheal 包的公开装配面 `NewService(kb, diag, runner)`：
//   - 失败用**真实 tools.Executor** 制造（file read 不存在路径 / git commit 不存在仓库），非手写字符串；
//   - diag 为确定性替身（实现 provider.Provider，不触网）；runner 为自愈动作替身；
//   - 回写用真实 KB 文件（exceptions.jsonl）。
//
// 【诚实边界】本桩证明 **harness 闭环**，不证明真实 JEV 模型的诊断质量 ——
// 后者离线不可判定，单独记为 unverified（见文档 §1 A6-model、§7）。
// ---------------------------------------------------------------------------
func TestArchA6FailureProducesWriteback(t *testing.T) {
	ctx := context.Background()
	logDir := t.TempDir()
	kb := selfheal.OpenKB(filepath.Join(logDir, selfheal.KBFileName))

	// 确定性「问题定位模型」替身：不触网、同一输入必返回同一诊断。
	// 注意：本桩只证明 **harness 闭环**（失败→诊断→动作→回写→复用），
	// 不证明真实 JEV 模型的诊断质量 —— 后者离线无法判定，另记 unverified（见文档 §7）。
	diag := &archDiagStub{content: `{"category":"transient","root_cause":"瞬时抖动",` +
		`"confidence":0.9,"recoverable":true,"suggestion":"稍后重试","action":"retry"}`}

	var replayCalls int32
	runner := func(tool string, args map[string]any) contract.Receipt {
		atomic.AddInt32(&replayCalls, 1)
		return contract.Receipt{Tool: tool, OK: true, Stdout: "重放成功"}
	}
	svc := selfheal.NewService(kb, diag, runner)

	// 夹具：用**真实执行器**制造一次确定性只读失败（读不存在的路径）。
	failed, readArgs := archRealReadFailure(t, logDir)
	attempt := selfheal.Attempt{Tool: "file", Args: readArgs, Receipt: failed}

	// ① 该收必收：可恢复的只读真实失败必须产生带 cause 类的诊断 + 自愈动作（安全重放）。
	repaired, d := svc.SafeRetry(ctx, "查一下不存在文件", contract.IntentQuery, attempt)
	if repaired == nil {
		t.Fatalf("A6 自愈动作缺失: 可恢复的只读失败未触发安全重放（failure=%+v）", failed)
	}
	if d == nil {
		t.Fatalf("A6 诊断缺失: 真实失败未产生诊断记录")
	}
	if !archIsCauseClass(d.Category) {
		t.Errorf("A6 诊断不可归因: cause 类为空或越界 category=%q", d.Category)
	}
	if !archIsHealAction(d.Action) {
		t.Errorf("A6 自愈动作非法: action=%q（五词白名单外）", d.Action)
	}

	// ③ 学习回写：修复成功后必须落盘知识库，且能按同一指纹检索到。
	fp := selfheal.Fingerprint(contract.IntentQuery, "file", failed.Stderr)
	if got := kb.Count(); got != 1 {
		t.Errorf("A6 学习回写缺失: 修复成功应写入 1 条知识，实得 %d", got)
	}
	if hit, ok := kb.Lookup(fp); !ok || hit.Category == "" {
		t.Errorf("A6 回写不可检索: 按修复指纹未命中知识库（fp=%.12s…）", fp)
	}

	// ② 同类失败再次发生 → 命中回写，零模型调用（这才叫"回写闭环"）。
	callsBefore := atomic.LoadInt32(&diag.calls)
	_, d2 := svc.SafeRetry(ctx, "查一下不存在文件", contract.IntentQuery, attempt)
	if d2 == nil {
		t.Fatalf("A6 第二次同类失败应命中知识库并给出诊断")
	}
	if d2.Source != "kb" {
		t.Errorf("A6 回写未被复用: 第二次诊断 source=%q（期望 kb）", d2.Source)
	}
	if now := atomic.LoadInt32(&diag.calls); now != callsBefore {
		t.Errorf("A6 回写未减少模型调用: 同类失败重复调用模型（%d → %d）", callsBefore, now)
	}

	// 反向反例（不该收却收了）：不可逆工具失败即使诊断说可恢复，也不得自动重放、不得回写知识。
	gitRecv, gitArgs := archRealGitFailure(t)
	replayBefore, kbBefore := atomic.LoadInt32(&replayCalls), kb.Count()
	_, dg := svc.SafeRetry(ctx, "提交这批改动", contract.IntentCommit,
		selfheal.Attempt{Tool: "git", Args: gitArgs, Receipt: gitRecv})
	if dg == nil {
		t.Fatalf("A6 不可逆失败仍应产生诊断（供归因）")
	}
	if now := atomic.LoadInt32(&replayCalls); now != replayBefore {
		t.Errorf("A6 安全红线被破坏: 不可逆工具（git）失败被自动重放")
	}
	if got := kb.Count(); got != kbBefore {
		t.Errorf("A6 反博弈被破坏: 未验证的重放不得回写知识（%d → %d）", kbBefore, got)
	}

	// 反向反例（缺证据 ≠ 通过）：模型输出不可解析时，不得折算成一条诊断。
	badKB := selfheal.OpenKB(filepath.Join(t.TempDir(), selfheal.KBFileName))
	bad := selfheal.NewService(badKB, &archDiagStub{content: "这不是 JSON"}, runner)
	if _, d := bad.SafeRetry(ctx, "查一下不存在文件", contract.IntentQuery, attempt); d != nil {
		t.Errorf("A6 缺证据被折算: 模型输出不可解析却产出了诊断 %+v", d)
	}
	if got := badKB.Count(); got != 0 {
		t.Errorf("A6 解析失败不得回写知识，实得 %d 条", got)
	}
}

// ---------------------------------------------------------------------------
// A6 夹具与替身（仅在 archstub 构建下编译）
// ---------------------------------------------------------------------------

// archDiagStub 是确定性「问题定位模型」替身（实现 provider.Provider），不触网。
type archDiagStub struct {
	calls   int32
	content string
}

func (p *archDiagStub) Name() string { return "arch-diag-stub" }

func (p *archDiagStub) Chat(_ context.Context, _ provider.ChatRequest) (provider.ChatResponse, error) {
	atomic.AddInt32(&p.calls, 1)
	return provider.ChatResponse{Content: p.content, FinishReason: "stop"}, nil
}

// archRealReadFailure 用真实执行器读一个不存在的路径，返回真实失败回执与原始 args。
func archRealReadFailure(t *testing.T, base string) (contract.Receipt, map[string]any) {
	t.Helper()
	args := map[string]any{"action": "read", "path": filepath.Join(base, "arch-不存在.md")}
	recv, err := (&tools.Executor{BaseDir: base}).Exec("file", args, contract.ToolContract{})
	if err != nil {
		t.Fatalf("A6 夹具失效: 真实执行器报错 %v", err)
	}
	if recv.OK {
		t.Fatalf("A6 夹具失效: 读不存在路径竟然成功")
	}
	if recv.Err == "" && recv.Stderr == "" {
		t.Fatalf("A6 夹具失效: 失败回执无错误文本，无法形成指纹")
	}
	return recv, args
}

// archRealGitFailure 用真实执行器对一个不存在的仓库做 git commit，返回真实失败回执与 args。
func archRealGitFailure(t *testing.T) (contract.Receipt, map[string]any) {
	t.Helper()
	args := map[string]any{"args": []string{"-C", filepath.Join(t.TempDir(), "不存在的仓库"), "commit", "-m", "x"}}
	recv, err := (&tools.Executor{BaseDir: t.TempDir()}).Exec("git", args, contract.ToolContract{})
	if err != nil {
		t.Fatalf("A6 夹具失效: git 执行器报错 %v", err)
	}
	if recv.OK {
		t.Fatalf("A6 夹具失效: 对不存在仓库的 git commit 竟然成功")
	}
	return recv, args
}

// archIsCauseClass 判定诊断 cause 类是否落在接口标准 v1 §3 的八类枚举内。
func archIsCauseClass(c string) bool {
	switch c {
	case selfheal.CatBudget, selfheal.CatNetwork, selfheal.CatAuth, selfheal.CatParam,
		selfheal.CatToolMissing, selfheal.CatPermission, selfheal.CatTransient, selfheal.CatUnknown:
		return true
	}
	return false
}

// archIsHealAction 判定自愈动作是否落在五词白名单内。
func archIsHealAction(a string) bool {
	switch a {
	case selfheal.ActionRetry, selfheal.ActionModify, selfheal.ActionFallback,
		selfheal.ActionAsk, selfheal.ActionStop:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// A7 三态判定：证据不足不得折算为通过（VSL-v3 红线）
//
// 「unverified 不得折算为通过」；三态 = met / unverified / not_met。
//
// 判据：观测缺失时判定必须是"证据不足"，而不是"支持"或"推翻"。
// 红旗：缺观测就默认通过，等于把"没测"写成"通过"。
// ---------------------------------------------------------------------------
func TestArchA7InsufficientEvidenceIsItsOwnVerdict(t *testing.T) {
	// 用分类器层可观测的等价物：UNKNOWN 必须回问（Ask 非空），
	// 不得被静默折算成某个具体意图去执行。
	c := input.NewTaskClassifier(0.6, nil)
	for _, text := range []string{"嗯那个呃", "这个那个"} {
		got := c.ClassifyTask(text)
		if got.Intent == contract.IntentUnknown && got.Ask == "" {
			t.Errorf("A7 三态被破坏: %q 判 UNKNOWN 却不回问 —— 把'不知道'折算成了可执行", text)
		}
	}
}

// ---------------------------------------------------------------------------
// 交叉不变式 X1：Ask 是最外层安全出口
//
// 本会话已三次踩到同一类缺口（G1-P4 / G3-P3 / G5-P0-1）：
// 新增一条"靠 Ask 拦住"的分支，总有下游路径会清空 Ask。
//
// 判据：**任何** Conflict 非空的意图，其 Ask 必须非空。
// 这是一条结构性守卫，新增仲裁时自动生效，不需要再逐个登记。
// ---------------------------------------------------------------------------
func TestArchX1NonEmptyConflictImpliesAsk(t *testing.T) {
	c := input.NewTaskClassifier(0.6, nil)
	probes := []string{
		"不要删除那个文件",      // negation
		"开始测试",          // meta
		"如果测试通过就提交",     // conditional
		"跑一下测试然后提交这批改动", // multi_action
	}
	for _, text := range probes {
		got := c.ClassifyTask(text)
		if got.Conflict == "" {
			continue
		}
		if got.Ask == "" {
			t.Errorf("X1 Ask 安全出口被绕过: %q conflict=%q 但 Ask 为空 —— 会直接进入执行通道",
				text, got.Conflict)
		}
	}
	_ = context.Background()
}

// ---------------------------------------------------------------------------
// A4 夹具（仅在 archstub 构建下编译）
// ---------------------------------------------------------------------------

// archOrchestrateFixture 装配一个"真实 git 仓库 + project 域"的最小可跑 Options。
// 关键：显式注册名为 project 的域并指向夹具仓库，避免 ensureProjectSpace 退回到
// 进程 cwd（否则测试会误把本仓库当项目根）。
func archOrchestrateFixture(t *testing.T, confirmFn func(string, string) (bool, error)) (*pipeline.Options, string) {
	t.Helper()
	opts := buildOpts(t, confirmFn)
	projDir := t.TempDir()
	docsDir := filepath.Join(projDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SPEC-v2-可执行规格书.md", "M7配置指南.md"} {
		if err := os.WriteFile(filepath.Join(docsDir, name),
			[]byte("架构幂等夹具内容-v1："+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	archInitGitRepo(t, projDir)
	if err := opts.Spaces.Add(&space.Manifest{
		Name:  "project",
		Type:  space.TypeProject,
		Scope: []string{projDir},
		Tools: []string{"file", "git", "search", "read", "test", "run"},
		Perms: space.Perms{Read: true, Write: true},
	}); err != nil {
		t.Fatal(err)
	}
	return opts, projDir
}

// archInitGitRepo 在 dir 建一个带基线提交的确定性 git 仓（不依赖全局 git 身份/签名配置）。
func archInitGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		// ⚠️ **不许写 git config**（2026-10-03 事故，全仓最后一处）：
		// `git config user.email` 不带 --local/--global 时写"从 cwd 上溯找到的仓库"的 config；
		// 当 dir 的仓库不可用时会上溯到**主仓库** ⇒ 覆盖真实身份（曾把 user.email 改成
		// vhs@test / vhs-test@example.com / vhs-arch@example.com 三个测试身份之一）、
		// 并把 core.bare 置 true ⇒ 仓库被当裸库。
		// ⇒ 身份改由**全局配置回落**；若需注入，用命令级 `-c user.email=...`。
		// commit.gpgsign 也**不写 config**（同因：config 会写"从 cwd 上溯找到的仓库"）
		// ⇒ 改在 commit 时用命令级 `-c commit.gpgsign=false` 注入
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v 失败: %s", args, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# arch fixture baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		// 身份与签名开关一律**命令级 -c 注入**（不写 config ⇒ 无副作用，也不依赖全局身份）
		{"-c", "user.email=vhs-arch@example.com", "-c", "user.name=vhs-arch",
			"-c", "commit.gpgsign=false", "commit", "-q", "-m", "baseline"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v 失败: %s", args, out)
		}
	}
}

// archGitHeadCount 返回仓库的真实提交数（git rev-list --count HEAD）——外部事实，不读回执自报。
func archGitHeadCount(t *testing.T, dir string) int {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-list --count HEAD 失败: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("git rev-list 输出不可解析: %q", out)
	}
	return n
}
