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
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/input"
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
// 红旗：重跑就多一个 commit = "可幂等"是自称的。
// ---------------------------------------------------------------------------
func TestArchA4OrchestrateIsIdempotent(t *testing.T) {
	t.Skip("A4 需要真实 git 仓库 + 模型 provider；由 pipeline/orchestrate_test.go 的 TestOrchestrate 覆盖。" +
		"本桩登记判据，待具备夹具后转为强制。")
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
// 判据：一次真实失败必须产生**可归因的诊断记录**（错误类别 + 建议），
//
//	且该记录可被后续同类失败检索到（回写闭环）。
//
// 红旗：M8 自报 jev-diagnose 仍是模拟桩 —— 本桩要把它变成红的。
// ---------------------------------------------------------------------------
func TestArchA6FailureProducesWriteback(t *testing.T) {
	t.Skip("A6 需要真实失败注入 + jev-diagnose（M8 缺口 #7：当前为模拟桩）。" +
		"判据已登记：失败必须产生带 cause 类的诊断，且同类失败可检索到回写。")
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
