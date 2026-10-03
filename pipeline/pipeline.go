// Package pipeline 是 M2 语音驱动开发的编排循环（freeze §3 pipeline）：
// 把「ASR 文本」从原文一路串到「四行回执」，中间经过 清洗→纠错→任务分类→指代消解→
// space_check→风险分级→确认→工具执行→独立校验→归因回写→四元缓存→四行回执。
//
// 本包是集成分片：space/refer/risk/verify/search/cache/tools 均为已交付只读依赖，
// 本包【不修改】它们的规则本体，只负责按冻结签名把它们编排起来。
// 规则语义（什么算越界 / 几类确认 / 不可逆清单）一律标注「搬 VSL」，不在本层重述。
package pipeline

// 【伪代码逻辑层】（必写模块，评审关卡产物。规则语义权威定义在 freeze §3 / SPEC v2 §2；
//  本层只描述 Run 的 13 阶段控制流 / 各阶段拒绝路径 / 确认分支 / 异常与中断处理，不可编译。）
//
// 全局不变量：
//   - 串行闸（用户拍板）：同一进程同时只允许一个 Run 在飞（Options.mu 互斥），
//     保护用户未提交改动，绝不覆盖/删除他人文件。
//   - start 事件 = input_raw 轨迹写入【完成】的时刻；end 事件 = 归因写入【完成】的时刻。
//     LoopMs = end-start 墙钟（含人工等待）；NetMs = 墙钟 − confirmFn/回问等待段。
//
// Run(ctx, o, text) -> Outcome, error：
//
//   gate.lock()                              // 串行闸；ctx 取消 → 立即返回且不破坏中间态
//   rid = "req-" + nano()
//   write(input_raw{text}); start = now()
//
//   阶段② clean:     cleaned = Cleaner.Clean(text); write(input_clean)
//   阶段③ correct:   corrected,corr = Dict.Correct(cleaned); write(input_correct)
//   阶段④ classify:  intent = TaskClassifier(corrected); write(intent)
//                    if intent.Ask != "": 跳到 [回问出口]
//   阶段⑤ refer:     intent = Refer.Resolve(intent, spaceOf(intent)); write(refer)
//                    if intent.Ask != "": 跳到 [回问出口]
//   阶段⑥ space:     space = 默认域(intent)（NOTE→vault-notes / 只读→global / 写意图须已点名）
//                    caps = planCaps(intent)
//                    verdict = space.Check(Registry, {Intent, Grant:{Auth:true}, ToolCaps:caps})
//                    write(space_check)
//                    if !verdict.Allowed: 跳到 [拦截出口]（verdict.Reason 进结果，绝不执行）
//   阶段⑦ risk:      imp = 机械信号（search 引用统计 + trajectory 热度；无 fs 时取 0）
//                    decision = risk.Evaluate(intent, imp)
//                    intent.Confirm = decision.Level（回填权威值）; write(risk)
//   阶段⑧ confirm:   wait0 = now()
//                    if cache.Get(quad) 命中: approved=true（不再问，决策复用）
//                    else:
//                      switch decision.Level:
//                        auto:   approved=true（不打断）
//                        light:  approved = ConfirmFn(rid, lightQuestion)
//                        strong: if Guard.ShouldDowngrade(path): approved=true（汇总待复核，不打断）
//                                else:            approved = ConfirmFn(rid, strongQuestion)
//                        human:  approved = ConfirmFn(rid, humanQuestion)   // 永远问
//                    waitMs += now()-wait0
//                    write(confirm)
//                    if !approved: 跳到 [未放行出口]（Confirmed=false，不执行）
//                    cache.Set(quad, decision.Level)
//   阶段⑨ exec:     for action in planActions(intent):
//                        receipt = Exec.Exec(tool, args(with log_dir 注入), contract)
//                        Receipts = append(Receipts, receipt)
//                    write(receipts)
//                    拒绝路径：任一 receipt.Blocked != "" → 不重试，原样进结果
//   阶段⑩ verify:    spec = planVerify(intent, verdict)
//                    if spec != nil: Verify = Verifier.Run(spec)
//                    else:           Verify = {unverifiable, "M2 未定义该校验"}
//                    write(verify)
//   阶段⑪ attribution: cls = classifyAttribution(intent, verdict, Receipts, Verify, corr)
//                      attr = Attribution{rid, discuss, cls, evidence, suggestion}
//                      write(kind=attribution)  // ← end = now()
//                      append discuss.jsonl（人可见结论，下一轮注入；不自动改词典/策略）
//   阶段⑫ cache:   四元组已在⑧ Set；此处仅在漂移/拦截时 InvalidateSpace（不命中即跳过）
//   阶段⑬ view:   View = renderView(intent, verdict, decision, Receipts, Verify, Ask)
//                  write(final)
//
//   LoopMs = end-start; NetMs = LoopMs - waitMs
//   return Outcome, nil
//
// [回问出口]   View.result="未执行（需回问：…）"; Attribution.class=context; end 照常计量
// [拦截出口]   View.result="BOUNDARY_VIOLATION/…"; Attribution.class=context; 不执行不校验
// [未放行出口] View.result="待确认（已拒绝/未放行）"; 不执行；Attribution.class=model
//
// 异常：Dict/Trace/Refer 为 nil 时对应阶段薄降级（不报错）；ctx 提前取消 → 返回 error，
//   但已落盘的轨迹条目保留（append-only，不回滚）。
//
// Summary(o, since) -> string：
//   读当日 trajectory-YYYYMMDD.jsonl，按 域 / 意图 / 失败原因 聚合；手机可读纯文本。

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"voicesign-harness/cache"
	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/ground"
	"voicesign-harness/input"
	"voicesign-harness/memory"
	"voicesign-harness/provider"
	"voicesign-harness/refer"
	"voicesign-harness/risk"
	"voicesign-harness/search"
	"voicesign-harness/selfheal"
	"voicesign-harness/space"
	"voicesign-harness/tools"
	"voicesign-harness/trajectory"
	"voicesign-harness/verify"
)

// Options 是编排循环的全部依赖（冻结签名）。未导出字段为集成层实现细节（串行闸/确认疲劳防护）。
type Options struct {
	Cfg      *config.Config
	Dict     *memory.Dictionary
	Spaces   *space.Registry
	Refer    *refer.Resolver
	Cache    *cache.Store
	Verifier *verify.Verifier
	Tools    *tools.Registry
	Exec     *tools.Executor

	// ConfirmFn 阻塞等待人工放行（CLI=stdin / server=HTTP）；返回 false 表示拒绝。
	ConfirmFn func(taskID, question string) (bool, error)
	Trace     *trajectory.Trajectory

	// Ground（M3 #37）认知切片注入器；nil 时薄降级（空 context）。
	Ground *ground.Ground

	// Providers（M7）LLM 接线层；nil 时纯规则/纯 search 路径不阻断。
	Providers *provider.Registry

	// selfhealSvc 异常自愈层（三环）；懒装配。Providers==nil 时恒为 nil（零开销跳过，主链不变）。
	selfhealSvc *selfheal.Service

	mu    *sync.Mutex
	guard *risk.Guard
}

// AskOption 是 need_ask 回问的结构化候选（M4-3 ①：点选即续跑，机器可读）。
type AskOption struct {
	ID    string `json:"id"`    // 稳定候选 id（edit/query/note/commit/... 或 refer 目标 id）
	Label string `json:"label"` // 一句话中文 label
}

// Outcome 是一次 Run 的完整产物（冻结签名）。
type Outcome struct {
	RequestID    string               `json:"request_id"`
	Intent       contract.Intent      `json:"intent"`
	Verdict      space.Verdict        `json:"verdict"`
	Decision     risk.Decision        `json:"decision"`
	Confirmed    bool                 `json:"confirmed"`
	Receipts     []contract.Receipt   `json:"receipts"`
	Verify       verify.Result        `json:"verify"`
	Attribution  contract.Attribution `json:"attribution"`
	View         contract.ReceiptView `json:"view"`
	Ask          string               `json:"ask,omitempty"`
	Options      []AskOption          `json:"options,omitempty"` // M4-3 结构化候选
	ContextBlock string               `json:"context_block,omitempty"`
	LoopMs       int64                `json:"loop_ms"`
	NetMs        int64                `json:"net_ms"`
}

// discussLogName 是 discuss-log 文件名（<log_dir>/discuss.jsonl）。
const discussLogName = "discuss.jsonl"

// Run 执行完整 13 阶段编排循环。ctx 取消会中止等待人工确认，但已落盘轨迹不回滚。
func Run(ctx context.Context, o *Options, text string) (Outcome, error) {
	// ⭐ 必填检查（Lead 2026-10-03 真跑实证：o==nil / 空 Options ⇒ **panic**，不是返回错误）。
	// 原则：**"崩"与"报错"的区别是 —— 崩了没有任何人能看到原因**（且若崩在后台 goroutine，recover 也抓不到）。
	if o == nil {
		return Outcome{}, fmt.Errorf("pipeline.Run: Options 为 nil（调用方必须提供完整 Options）")
	}
	if o.Spaces == nil {
		return Outcome{}, fmt.Errorf("pipeline.Run: Spaces 未配置（域门禁缺失 ⇒ 拒绝执行）")
	}
	// 注：**不检查 Providers** —— 其文档明确"nil 时纯规则/纯 search 路径不阻断"（Options.Providers 注释）。
	// 若在此强求，会把"无 LLM 也能跑"的设计判死。仅当真要用 LLM 时才在对应路径处理。
	if o.mu == nil {
		o.mu = &sync.Mutex{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.guard == nil {
		o.guard = risk.NewGuard()
	}

	out := Outcome{RequestID: newRequestID()}
	if strings.TrimSpace(text) == "" {
		out.Ask = "空指令，没听清，请再说一遍"
		out.View = contract.ReceiptView{
			Action: "（空指令）", Files: "—", Result: "未执行（需回问：" + out.Ask + "）", Undo: "—",
		}
		o.write(trajectory.Entry{RequestID: out.RequestID, Kind: trajectory.KindFinal, Content: contract.RenderReceipt(out.View)})
		return out, nil
	}

	cleaner := input.NewCleaner(nil)
	emit := func(e trajectory.Entry) {
		if e.RequestID == "" {
			e.RequestID = out.RequestID
		}
		o.write(e)
	}

	// ① input_raw 轨迹写入完成 → start 事件（#45）。
	emit(trajectory.Entry{Kind: trajectory.KindInputRaw, Content: text})
	start := time.Now()
	var waitMs time.Duration

	// ② clean
	cleaned := cleaner.Clean(text)
	emit(trajectory.Entry{Kind: trajectory.KindInputClean, Content: cleaned})

	// ③ dict correct
	corrected := cleaned
	var corrections []contract.Correction
	if o.Dict != nil {
		corrected, corrections = o.Dict.Correct(cleaned)
	}
	emit(trajectory.Entry{Kind: trajectory.KindInputCorrec, Content: corrected})

	// ④ TaskClassifier.ClassifyTask（空间候选取自注册域列表）
	classifier := input.NewTaskClassifier(confOf(o.Cfg), spaceHints(o.Spaces))
	intent := classifier.ClassifyTask(corrected)
	intent.RawText = text
	intent.Corrections = corrections

	// M7 ① 意图分类 LLM 回退：规则低置信/UNKNOWN 且像自然语言问句时 → fast provider 补分类。
	intent = o.llmIntentFallback(ctx, intent, corrected)
	emit(trajectory.Entry{Kind: trajectory.KindIntent, Intent: &intent})

	// ⑤ refer 消解（不覆盖分类器已显式填好的字段）；M4-5：同时拿 refer 目标候选。
	// M7 修复（Codex/gpt-6-luna 外部诊断 2026-10-02）：按意图门控——
	// QUERY 高置信（>=0.8）的"这个/那个"是普通口语代词，跳过指代消解，
	// 否则 refer 候选为空会写 Ask"你说的「这个」指的是哪个？"→ need_ask（答非所问）。
	var referOpts []refer.Option
	if o.Refer != nil && shouldResolveRefer(&intent) {
		resolved, opts, err := o.Refer.ResolveOptions(&intent, intent.Space)
		if err == nil && resolved != nil {
			prevAsk := intent.Ask
			intent = *resolved
			referOpts = opts
			// Codex 收紧（2026-10-02）：refer 新写 Ask（"指的是哪个"）时，
			// 若歧义不阻止执行（陈述/查询有实体/记录类）→ 清掉继续执行，不因指代 Ask。
			if intent.Ask != "" && intent.Ask != prevAsk && !clarificationBlocksExecution(&intent, referOpts) {
				intent.Ask = ""
			}
		}
	}
	emit(trajectory.Entry{Kind: "refer", Intent: &intent})

	// 回问出口：分类器/指代任一层要回问 → 不执行。
	// M3 #37：回问是真实的"模型/人需要更多上下文"点，这里注入 ground 认知切片。
	if intent.NeedsClarification() {
		snap := o.renderGround()
		intent.Context = append(intent.Context, snap.ProjectMap...)
		out.Intent = intent
		out.Ask = intent.Ask
		// Codex 收紧（2026-10-02）：options 按澄清意图动态生成，不再塞固定"改文件/查代码/记想法/提交"。
		out.Options = mergeAskOptions(optionsForIntent(&intent, referOpts), referOpts)
		out.ContextBlock = snap.Block
		out.Attribution = o.attribution(out.RequestID, contract.AttrContext,
			"待澄清："+intent.Ask, "轨迹 kind=intent/refer", "下次给出具体域/对象后重试")
		o.writeAttribution(out.Attribution)
		out.LoopMs = time.Since(start).Milliseconds()
		out.NetMs = out.LoopMs - waitMs.Milliseconds()
		out.View = contract.ReceiptView{
			Action: shortAction(intent), Files: "—",
			Result: "未执行（需回问：" + intent.Ask + "）", Undo: "—（未执行）",
		}
		o.writeTaskMetrics(intent, out, false)
		o.write(trajectory.Entry{RequestID: out.RequestID, Kind: trajectory.KindFinal, Content: contract.RenderReceipt(out.View)})
		return out, nil
	}

	// ⑥ space select + Check
	spaceID := defaultSpaceFor(intent)
	intent.Space = spaceID
	caps := planCaps(intent)
	verdict := space.Check(o.Spaces, space.CheckInput{
		Intent:   intent,
		Grant:    space.Grant{Authorized: true},
		ToolCaps: caps,
		// Contracts 有意传 nil：B/C 测试与冻结语义均以工具族名（file/git/read/…）做族级门禁；
		// 契约逐 cap 的 risk 分级在 risk.Evaluate 阶段消费，不在此重复交集。
	})
	b, _ := json.Marshal(verdict)
	emit(trajectory.Entry{Kind: "space_check", Content: string(b)})
	out.Intent = intent
	out.Verdict = verdict

	// 拦截出口：space_check 拒绝 → 绝不执行。
	if !verdict.Allowed {
		out.Attribution = o.attribution(out.RequestID, contract.AttrContext,
			"space_check 拒绝（"+verdict.Reason+"）", "轨迹 kind=space_check",
			"先注册/确认域，或换到已授权的项目域")
		o.writeAttribution(out.Attribution)
		out.LoopMs = time.Since(start).Milliseconds()
		out.NetMs = out.LoopMs - waitMs.Milliseconds()
		out.View = contract.ReceiptView{
			Action: shortAction(intent), Files: "—",
			Result: "BOUNDARY_VIOLATION：" + reasonText(verdict.Reason), Undo: "—（未执行）",
		}
		o.writeTaskMetrics(intent, out, false)
		o.write(trajectory.Entry{RequestID: out.RequestID, Kind: trajectory.KindFinal, Content: contract.RenderReceipt(out.View)})
		return out, nil
	}

	// ⑦ risk.Evaluate（机械信号：fs 引用统计 + 热度；无真实项目时取 0）
	imp := o.mechanicalImpact(intent)
	decision := risk.Evaluate(intent, imp)
	intent.Confirm = decision.Level
	out.Decision = decision
	db, _ := json.Marshal(decision)
	emit(trajectory.Entry{Kind: "risk", Content: string(db)})

	// ⑧ confirm（按 Decision 分派；四元缓存命中=不再问）
	// 安全不变量（SPEC #40/#29）：human=不可逆，永远走 ConfirmFn，
	// 从四元缓存 Get 与 Set 双双排除——"不可逆不可被学习掉"。
	wait0 := time.Now()
	approved := false
	quad := quadKey(intent, decision.Level)
	cacheable := decision.Level != contract.ConfirmHuman
	if o.Cache != nil && cacheable {
		if cached, ok := o.Cache.Get(quad); ok {
			// cached 只在 Set 处写入 decision.Level；"approved" 是历史兼容串，一并放行。
			approved = cached == decision.Level || cached == "approved"
		}
	}
	if !approved {
		switch decision.Level {
		case contract.ConfirmAuto:
			approved = true // 不打断
		case contract.ConfirmLight:
			approved = o.confirm(ctx, out.RequestID, "轻确认："+decision.Reason+"，放行？(y/n)")
		case contract.ConfirmStrong:
			if o.guard.ShouldDowngrade(targetPath(intent)) {
				approved = true // 同路径连续强确认疲劳 → 降为汇总待复核
			} else {
				approved = o.confirm(ctx, out.RequestID, "强确认："+decision.Reason+"，放行？(y/n)")
			}
		case contract.ConfirmHuman:
			q := "人工放行（不可逆）：" + decision.Reason
			// M4-4：COMMIT 前把未提交改动数写进确认问题，绝不覆盖/改写历史。
			if it := intent; it.Intent == contract.IntentCommit {
				if root := o.projectRootForCommit(it); root != "" {
					if n := gitDirtyCount(root); n >= 0 {
						q += fmt.Sprintf("；项目 %s 当前有 %d 个未提交改动，提交将包含它们（git add -A + commit，不改写历史）", root, n)
					}
				}
			}
			approved = o.confirm(ctx, out.RequestID, q+"，放行？(y/n)")
		}
	}
	// M4-1：是否真问过人工（auto 不打断不算等待；waitMs 的 µs 级 overhead 不计）。
	humanWait := decision.Level != contract.ConfirmAuto
	waitMs += time.Since(wait0)
	out.Confirmed = approved
	emit(trajectory.Entry{Kind: "confirm", Content: fmt.Sprintf("level=%s approved=%v", decision.Level, approved)})
	o.recordDecision(out.RequestID, intent, decision, approved)
	if o.Cache != nil && approved && cacheable {
		// human 级绝不写缓存（不可逆永远人工）。
		_ = o.Cache.Set(quad, decision.Level)
	}

	// 未放行出口：不执行。
	if !approved {
		out.Attribution = o.attribution(out.RequestID, contract.AttrModel,
			"用户未放行（decision="+decision.Level+"）", "轨迹 kind=confirm", "如属误拒可加入四元缓存放行")
		o.writeAttribution(out.Attribution)
		out.LoopMs = time.Since(start).Milliseconds()
		out.NetMs = out.LoopMs - waitMs.Milliseconds()
		out.View = contract.ReceiptView{
			Action: shortAction(intent), Files: targetFiles(intent),
			Result: "待确认（" + decision.Level + "，未放行）", Undo: "—（未执行）",
		}
		o.write(trajectory.Entry{RequestID: out.RequestID, Kind: trajectory.KindFinal, Content: contract.RenderReceipt(out.View)})
		return out, nil
	}

	// ⑨ Exec（仅已过 space_check + risk 的动作）
	receipts := o.execActions(ctx, intent)
	out.Receipts = receipts
	emit(trajectory.Entry{Kind: trajectory.KindReceipts, Receipts: receipts})

	// ⑨-bis 异常自愈层（可选）：有失败回执 → 诊断 + 只读安全重放（限 2 轮）。
	// 诊断层未配置/失败一律不阻断；重放成功的新回执合并进 out.Receipts，诊断结论供归因。
	if hasFailure(receipts) {
		if repaired := o.repairFailed(ctx, intent, receipts); len(repaired) > 0 {
			receipts = append(receipts, repaired...)
			out.Receipts = receipts
			emit(trajectory.Entry{Kind: trajectory.KindReceipts, Receipts: receipts})
		}
	}

	// ⑩ verify 独立校验
	spec := o.planVerify(intent, o.logDir())
	if spec != nil && o.Verifier != nil {
		res, err := o.Verifier.Run(*spec)
		if err != nil {
			out.Verify = verify.Result{Status: verify.StatusUnverifiable, Detail: err.Error()}
		} else {
			out.Verify = res
		}
	} else {
		out.Verify = verify.Result{Status: verify.StatusUnverifiable, Detail: "M2 未为该意图定义独立校验"}
	}
	vb, _ := json.Marshal(out.Verify)
	emit(trajectory.Entry{Kind: "verify", Content: string(vb)})

	// ⑩-bis verify fail → 带 tool:"verify" 进诊断层（不自动重跑 verify，结论供归因）。
	if out.Verify.Status == verify.StatusFail {
		if svc := o.selfheal(); svc != nil {
			_ = svc.Diagnose(ctx, intent.RawText, intent.Intent, []selfheal.Trace{
				selfheal.NewTrace("verify", "", map[string]any{"evidence": out.Verify.Evidence}, out.Verify.Detail),
			})
		}
	}

	// ⑪ attribution + 轨迹（end = 归因写入完成）
	cls, detail, suggestion := classifyAttribution(intent, receipts, out.Verify, corrections)
	// ⑪-bis：执行类归因且诊断层有结论时，用诊断 suggestion 替换静态建议，detail 附根因；
	// 无诊断结论 → 静态建议逐字不变。
	if svc := o.selfheal(); svc != nil {
		if d := svc.LastDiagnosis(); d != nil && cls == contract.AttrExec {
			suggestion = d.Suggestion
			detail = detail + "（诊断根因：" + d.RootCause + "）"
		}
	}
	out.Attribution = o.attribution(out.RequestID, cls, detail, evidenceOf(receipts, out.Verify), suggestion)
	o.writeAttribution(out.Attribution)

	// ⑫ cache：四元组已在⑧ Set；拦截/漂移路径在 o.attribution 里无需额外失效。
	// ⑬ view
	out.View = renderView(intent, verdict, decision, receipts, out.Verify, approved)
	o.write(trajectory.Entry{RequestID: out.RequestID, Kind: trajectory.KindFinal, Content: contract.RenderReceipt(out.View)})

	out.LoopMs = time.Since(start).Milliseconds()
	out.NetMs = out.LoopMs - waitMs.Milliseconds()

	// ⑭ 任务指标入轨迹（#52 摘要聚合源）：结构化 JSON 一行。
	o.writeTaskMetrics(intent, out, humanWait)
	return out, nil
}

// taskMetricsPayload 是写入轨迹的单行结构化指标（#52/#M4-1 摘要按此聚合）。
type taskMetricsPayload struct {
	Kind    string `json:"kind"` // "task_metrics"
	Space   string `json:"space"`
	Intent  string `json:"intent"`
	AttrCls string `json:"attr_class"`
	LoopMs  int64  `json:"loop_ms"`
	NetMs   int64  `json:"net_ms"`
	HadWait bool   `json:"had_wait"` // M4-1：有人工等待/LLM 才计入 Net 均值
	OK      bool   `json:"ok"`
}

func (o *Options) writeTaskMetrics(it contract.Intent, out Outcome, humanWait bool) {
	p := taskMetricsPayload{
		Kind:    "task_metrics",
		Space:   it.Space,
		Intent:  it.Intent,
		AttrCls: out.Attribution.Class,
		LoopMs:  out.LoopMs,
		NetMs:   out.NetMs,
		HadWait: humanWait,
		OK:      out.Verify.Status != verify.StatusFail && !hasFailure(out.Receipts),
	}
	b, _ := json.Marshal(p)
	o.write(trajectory.Entry{RequestID: out.RequestID, Kind: "task_metrics", Content: string(b)})
}

// ---------- 集成层小工具（薄封装，规则语义均在被编排的包内） ----------

func confOf(cfg *config.Config) float64 {
	if cfg != nil && cfg.Input.IntentConf > 0 {
		return cfg.Input.IntentConf
	}
	return 0.6
}

func spaceHints(r *space.Registry) []input.SpaceHint {
	if r == nil {
		return nil
	}
	var hints []input.SpaceHint
	for _, name := range r.List() {
		hints = append(hints, input.SpaceHint{Name: name})
	}
	return hints
}

// defaultSpaceFor 按意图类别落默认域（搬 VSL：只读→global 兜底；NOTE→vault-notes 追加；
// 写意图必须由分类器/指代点名域，否则落到 global 只读会被 space_check 自然拒绝）。
func defaultSpaceFor(it contract.Intent) string {
	if it.Space != "" {
		return it.Space
	}
	switch it.Intent {
	case contract.IntentNote:
		return "vault-notes"
	case contract.IntentQuery, contract.IntentAsk:
		return "global"
	case contract.IntentOrchestrate:
		return "project" // 多步编排=读文档+写文件+git 提交，落在项目域
	case contract.IntentRegisterTool:
		return "project" // 2026-10-04 能力自举：注册工具=写类意图，需可写域（global 只读会拒）
	case contract.IntentEdit, contract.IntentDebug, contract.IntentTest,
		contract.IntentCommit, contract.IntentDeploy:
		// 2026-10-04 用户实测"跑一下测试/提交一下代码"→越界（BOUNDARY_VIOLATION）：
		// 写/执行类意图无点名域时默认落 project（可写+test/run/git 工具），消除语音场景"越界"。
		return "project"
	default:
		return "global"
	}
}

// planCaps 把意图翻译成待调工具族名（与 manifest.Tools 词表对齐）。
func planCaps(it contract.Intent) []string {
	switch it.Intent {
	case contract.IntentNote:
		return []string{"note", "file-append", "read"}
	case contract.IntentQuery, contract.IntentAsk:
		return []string{"read", "query"}
	case contract.IntentEdit, contract.IntentDebug:
		return []string{"file", "read", "run"}
	case contract.IntentTest:
		return []string{"test", "run", "read"}
	case contract.IntentCommit:
		return []string{"git", "read"}
	case contract.IntentOrchestrate:
		// 组织者式多步链：读文档（file/search）→ 写文件（file）→ git 提交（git）。
		return []string{"file", "read", "git", "search"}
	case contract.IntentDeploy:
		return []string{"deploy", "http", "read"}
	case contract.IntentRegisterTool:
		return []string{"read"}
	default:
		return []string{"read"}
	}
}

// execActions 把意图翻译成具体工具动作（M2 最小可执行集；NOT E/QUERY 走通即可）。
func (o *Options) execActions(ctx context.Context, it contract.Intent) []contract.Receipt {
	if o.Exec == nil {
		return []contract.Receipt{{Tool: "pipeline", OK: false, Err: "执行器未配置"}}
	}
	logDir := o.logDir()
	switch it.Intent {
	case contract.IntentOrchestrate:
		return o.execOrchestrate(ctx, it, logDir)
	case contract.IntentRegisterTool:
		// 2026-10-04 用户强要求"后台必须有能力扩展能力，自迭代自更新"：
		// REGISTER_TOOL 意图真正落地注册（此前只给 caps，未执行）。
		return o.execRegisterTool(ctx, it, logDir)
	case contract.IntentNote:
		path := filepath.Join(logDir, "notes.md")
		args := map[string]any{
			"action":  "append",
			"path":    path,
			"content": "\n- " + time.Now().Format("2006-01-02 15:04") + " " + it.CorrectedText + "\n",
			"log_dir": logDir,
		}
		return []contract.Receipt{o.run("file", args)}
	case contract.IntentQuery, contract.IntentAsk:
		// 自迭代第二段（2026-10-04）：语音自举注册的契约能力优先执行。
		// 文本命中已注册能力（如「远程控制电脑」）→ 调真实执行器，不再回"无法远程控制"。
		if capName := o.matchVoiceContract(&it); capName != "" {
			args := map[string]any{
				"cap":     capName,
				"text":    it.CorrectedText,
				"pattern": it.CorrectedText,
				"log_dir": logDir,
			}
			return []contract.Receipt{o.run("remote-desktop", args)}
		}
		pattern := it.CorrectedText
		if it.Params != nil && it.Params["object"] != "" {
			pattern = it.Params["object"]
		}
		args := map[string]any{"pattern": pattern, "kind": "text"}
		recv := o.run("search", args)
		// M7 ②：用 LLM 把 search 结果转成自然语言回答（失败回退 search stdout）。
		if answer := o.queryLLMAnswer(context.Background(), it.RawText, recv.Stdout); answer != "" {
			recv.Stdout = answer
		}
		return []contract.Receipt{recv}
	case contract.IntentCommit:
		// M4-4：在项目域 scope 根执行真实 git 提交（git add -A + commit；不改写历史）。
		root := o.projectRootForCommit(it)
		if root == "" {
			return []contract.Receipt{{Tool: "git", OK: false, Err: "未解析到项目域根（COMMIT 需注册 project 域）"}}
		}
		msg := strings.TrimSpace(it.CorrectedText)
		if msg == "" {
			msg = "vhs: commit"
		}
		var stdout strings.Builder
		add := exec.Command("git", "add", "-A")
		add.Dir = root
		if out, err := add.CombinedOutput(); err != nil {
			return []contract.Receipt{{Tool: "git", OK: false, Err: "git add 失败: " + string(out)}}
		}
		cm := exec.Command("git", "commit", "-m", msg)
		cm.Dir = root
		if out, err := cm.CombinedOutput(); err != nil {
			// 无改动也返回 OK=false（不报错历史）；回执带 stdout。
			stdout.Write(out)
			return []contract.Receipt{{Tool: "git", OK: false, Stdout: stdout.String(), Err: "git commit: " + err.Error()}}
		} else {
			stdout.Write(out)
		}
		// 取新提交 hash 作为回执证据（fs 事实）。
		log := exec.Command("git", "log", "-1", "--format=%H %s")
		log.Dir = root
		if lout, err := log.Output(); err == nil {
			stdout.WriteString("\n" + strings.TrimSpace(string(lout)))
		}
		return []contract.Receipt{{Tool: "git", OK: true, Stdout: stdout.String()}}
	default:
		// EDIT/DEBUG/TEST/COMMIT/DEPLOY：M2 编排层只接通门禁与回执，不擅自起子进程写项目库；
		// 返回一条占位回执，由后续里程碑接模型工具循环。这样门禁/校验/归因链路在 M2 已闭环可测。
		return []contract.Receipt{{
			Tool: "pipeline", OK: true,
			Stdout: "M2 已过 space_check+risk+confirm，动作待模型工具循环落地（intent=" + it.Intent + "）",
		}}
	}
}

func (o *Options) run(tool string, args map[string]any) contract.Receipt {
	c, _ := o.Tools.Get(tool)
	recv, err := o.Exec.Exec(tool, args, c)
	if err != nil {
		recv.OK = false
		recv.Err = err.Error()
	}
	return recv
}

// matchVoiceContract 检测口语文本是否命中语音自举注册的契约能力（Source=="voice"）。
// 命中返回能力名（如「远程控制电脑」），未命中返回空串。仅查询类意图走此优先路径，
// 执行动作类意图仍走各自专用执行器（test/git/file…）。
func (o *Options) matchVoiceContract(it *contract.Intent) string {
	if o.Tools == nil {
		return ""
	}
	text := it.CorrectedText
	if text == "" {
		text = it.RawText
	}
	for _, c := range o.Tools.All() {
		if c.Source != "voice" || c.Name == "" {
			continue
		}
		if strings.Contains(text, c.Name) {
			return c.Name
		}
	}
	return ""
}

// ---------- 多步编排（ORCHESTRATE，组织者式路由） ----------
//
// 【设计取舍：方案 B】复合/长任务由组织者确定性拆成 read→summarize→write→commit 子序列，
// 在 harness 自身闭环跑完（不外包给外部 shell 脚本）。动作计划不由模型 function-calling 产出，
// 模型只参与"summarize 内容"那一步（noJSON 纯文本，失败回退确定性拼接）。
//
// 硬约束：
//   - 所有文件读/写路径都经 space.ResolveScopePath(root, path) 双重 containment 校验，
//     白名单外路径直接返回失败回执，绝不落盘；
//   - git 提交只 `git add -- <生成文件>`，绝不 `git add -A`，避免扫入无关未跟踪文件；
//   - 复用既有 ⑥ space_check / ⑦ risk(不可逆=human) / ⑧ 人工确认闸，一次确认放行整条链。

const (
	orchestrateMaxSourceBytes = 8000 // 每份源文档读入上限（防爆上下文）
	orchestrateMaxSources     = 6
)

// defaultOrchestrateSources 组织者默认纳入汇总的设计/沟通记录文档（docs/ 下按文件名命中；
// 不存在则跳过，不阻断）。
var defaultOrchestrateSources = []string{
	"SPEC-v2-可执行规格书.md",
	"详细设计-语音驱动开发-v2定稿-20261002.md",
	"异常自愈架构-问题定位模型.md",
	"M7配置指南.md",
	"全会话记录.md",
}

// gitTopLevel 探测服务器进程工作目录所在 git 仓库根（`git rev-parse --show-toplevel`）。
// m7-serve.sh 在仓库根启动二进制，故 cwd 即仓库根；探测失败返回空串。
func gitTopLevel() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = wd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ensureProjectSpace 惰性确保 project 域已注册且 scope 指向 git 仓库根。
//
// 全新环境（rm -rf /tmp/vhs-m7 后重建）spaces/ 为空，长任务会因 projectRootForCommit 未解析而 FAILED。
// 这里自动探测 git toplevel 并注册 project 域（落盘到日志目录 spaces/，与 m7-serve.sh 防清理一致）。
// 硬约束：仅注册 project 域，tools/权限取最小集（read+write），不扩大写权限范围；
// 已存在带非空 scope 的 project 域则不覆盖（尊重用户/运维手工注册）。
func (o *Options) ensureProjectSpace() {
	if o == nil || o.Spaces == nil {
		return
	}
	if m, ok := o.Spaces.Get("project"); ok && m != nil && len(m.Scope) > 0 {
		return // 已有带 scope 的 project 域，不动
	}
	root := gitTopLevel()
	if root == "" {
		log.Printf("[ensureProjectSpace] 未探测到 git 仓库根（cwd=%s），跳过自动注册", mustGetwd())
		return
	}
	if err := o.Spaces.Add(&space.Manifest{
		Name:  "project",
		Type:  space.TypeProject,
		Scope: []string{root + "/**"},
		Tools: []string{"file", "git", "search", "read", "test", "run"},
		Perms: space.Perms{Read: true, Write: true},
	}); err != nil {
		log.Printf("[ensureProjectSpace] 自动注册 project 域失败: %v", err)
		return
	}
	log.Printf("[ensureProjectSpace] 已自动注册 project 域 scope=%s/**", root)
}

func mustGetwd() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "?"
}

// execOrchestrate 跑 read→summarize→write→commit 多步链，每步产出真实回执。
func (o *Options) execOrchestrate(ctx context.Context, it contract.Intent, logDir string) []contract.Receipt {
	// 全新环境（rm -rf /tmp/vhs-m7 后重建）spaces/ 为空 → 惰性自动注册 project 域，
	// scope 指向 git 仓库根，落盘到日志目录 spaces/，长任务即开即用（不覆盖已有注册）。
	o.ensureProjectSpace()
	root := o.projectRootForCommit(it)
	if root == "" {
		return []contract.Receipt{{Tool: "orchestrate", OK: false,
			Err: "多步编排需注册 project 域且 scope 指向项目根（projectRootForCommit 未解析）"}}
	}
	var receipts []contract.Receipt
	nextSeq := func() int { return len(receipts) + 1 }

	// 1) 发现源文档并逐个做域内白名单校验（docs/ 下命中默认清单）。
	docDir := filepath.Join(root, "docs")
	type srcDoc struct{ abs, base string }
	var sources []srcDoc
	for _, name := range defaultOrchestrateSources {
		abs := filepath.Join(docDir, name)
		clean, ok := space.ResolveScopePath(root, abs)
		if !ok {
			return append(receipts, contract.Receipt{Seq: nextSeq(), Tool: "orchestrate",
				OK: false, Err: "白名单外路径被拒绝（越界）: " + abs})
		}
		if _, err := os.Stat(clean); err != nil {
			continue // 源文档不存在则跳过
		}
		sources = append(sources, srcDoc{abs: clean, base: name})
		if len(sources) >= orchestrateMaxSources {
			break
		}
	}

	// 2) 逐份读（file read，真实回执）；任一失败 → 不写不提交。
	var contents []string
	for _, s := range sources {
		recv := o.run("file", map[string]any{"action": "read", "path": s.abs})
		recv.Seq = nextSeq()
		receipts = append(receipts, recv)
		if !recv.OK {
			return receipts
		}
		contents = append(contents, "# 源文档: "+s.base+"\n"+truncateStr(recv.Stdout, orchestrateMaxSourceBytes))
	}

	// 3) 汇总成一份文档（LLM noJSON 纯文本；不可用则确定性拼接，仍产出真实文件）。
	title := strings.TrimSpace(it.Params["target_doc"])
	if title == "" {
		title = "VoiceSign-Harness-全景开发文档"
	}
	merged := strings.Join(contents, "\n\n")
	body := o.llmSummarize(ctx, title, merged)
	if strings.TrimSpace(body) == "" {
		names := make([]string, 0, len(sources))
		for _, s := range sources {
			names = append(names, s.base)
		}
		body = deterministicSummary(title, names, merged)
	}
	doc := "# " + title + "\n\n" +
		"> 本文件由 VoiceSign Harness 多步编排（ORCHESTRATE：读→汇总→写→提交）自动生成。\n\n" +
		body + "\n"

	// 4) 写目标文件（白名单校验后走 file write，真实回执，含备份标记）。
	targetAbs := filepath.Join(docDir, title+".md")
	cleanTarget, ok := space.ResolveScopePath(root, targetAbs)
	if !ok {
		return append(receipts, contract.Receipt{Seq: nextSeq(), Tool: "orchestrate",
			OK: false, Err: "白名单外写路径被拒绝（越界）: " + targetAbs})
	}
	wrecv := o.run("file", map[string]any{
		"action": "write", "path": cleanTarget, "content": doc, "log_dir": logDir,
	})
	wrecv.Seq = nextSeq()
	receipts = append(receipts, wrecv)
	if !wrecv.OK {
		return receipts
	}

	// 5) 定向 git 提交（只 add 生成文件，绝不 git add -A）。
	crecv := o.commitTargetPath(root, cleanTarget, "vhs(orchestrate): 生成《"+title+"》多步编排落地")
	crecv.Seq = nextSeq()
	receipts = append(receipts, crecv)
	return receipts
}

// llmSummarize 用 fast provider 把多份文档内容整理成一份 Markdown 文档（noJSON 纯文本，
// 与 QUERY 回答层同款：显式关闭 json_object，不发 temperature=0）。任何失败 → 返回空串。
//
// 【推理模型预算】gpt-6-luna 是推理模型，reasoning 会吃掉 max_completion_tokens；
// 1500 全被思考吃光→finish_reason=length、content 空。故预算给到 8000，并在 system 里
// 要求"直接输出正文、勿长篇推理"。失败/空必须打日志（对照 queryLLMAnswer，不再吞错）。
func (o *Options) llmSummarize(ctx context.Context, title, merged string) string {
	if o == nil || o.Providers == nil {
		log.Printf("[llmSummarize] providers nil")
		return ""
	}
	p, err := o.Providers.Get("fast")
	if err != nil {
		log.Printf("[llmSummarize] fast provider unavailable: %v", err)
		return ""
	}
	resp, err := p.Chat(ctx, provider.ChatRequest{
		Messages: []contract.Message{
			{Role: "system", Content: "你是技术文档整理助手。把下面多份设计文档/沟通记录整理成一份结构清晰的中文《全景开发文档》，带二级分节。直接输出 Markdown 正文：不要 JSON、不要复述本指令、不要长篇推理、不要解释你做了什么。"},
			{Role: "user", Content: "目标文档标题：" + title + "\n\n源文档内容：\n" + truncateStr(merged, 12000)},
		},
		MaxTokens:      8000,
		ResponseFormat: noJSON(),
	})
	if err != nil {
		log.Printf("[llmSummarize] fast Chat err: %v", err)
		return ""
	}
	if strings.TrimSpace(resp.Content) == "" {
		log.Printf("[llmSummarize] fast Chat empty content (finish_reason may be length; reasoning budget exhausted)")
		return ""
	}
	c := strings.TrimSpace(resp.Content)
	// provider 级默认 json_object 时可能仍包一层 JSON 壳，解出文本字段。
	if strings.HasPrefix(c, "{") {
		var j map[string]any
		if err := json.Unmarshal([]byte(c), &j); err == nil {
			for _, k := range []string{"text", "content", "response", "answer", "markdown"} {
				if s, ok := j[k].(string); ok && strings.TrimSpace(s) != "" {
					c = strings.TrimSpace(s)
					break
				}
			}
		}
		if strings.HasPrefix(c, "{") {
			log.Printf("[llmSummarize] returned unparsable JSON shell: %.160s", c)
			return ""
		}
	}
	return c
}

// deterministicSummary 无模型时的确定性汇总（结构化拼接源文档），保证仍产出真实文件。
func deterministicSummary(title string, sourceNames []string, merged string) string {
	var sb strings.Builder
	sb.WriteString("## 概览\n\n")
	sb.WriteString("本文件由多步编排自动汇总，纳入以下 " + strconvItoa(len(sourceNames)) + " 份源文档：\n\n")
	for _, name := range sourceNames {
		fmt.Fprintf(&sb, "- %s\n", name)
	}
	sb.WriteString("\n## 各文档\n\n")
	sb.WriteString("> 注：本次未调用 LLM 润色（模型不可用），内容为源文档结构化拼接。\n\n")
	sb.WriteString("## 全文摘录\n\n```markdown\n")
	sb.WriteString(truncateStr(merged, 6000))
	sb.WriteString("\n```\n")
	return sb.String()
}


// sanitizePathPart 把标题转为安全的路径分段（保留中英文数字与连字符）。
func sanitizePathPart(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			sb.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff:
			sb.WriteRune(r)
		default:
			sb.WriteRune('-')
		}
	}
	out := strings.Trim(sb.String(), "-")
	if out == "" {
		out = "implementation"
	}
	return out
}

// asciiSlug 把标题转为 ASCII 安全名（module path 用；非 ASCII 一律 '-'，空则回退 impl）。
func asciiSlug(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	out := strings.Trim(sb.String(), "-")
	if out == "" {
		out = "impl"
	}
	return out
}

// llmGenerateImplement（L-01 验收 2026-10-03）：需求文档 → LLM 生成**完整可运行**
// 的 Go 实现 → 写盘 → go build 真编译验证 → 编译错误回喂修复（≤2 轮）。
//
// **逐文件生成**（2026-10-03 真跑实测）：aiops 网关对 /api/model/chat 有 ~60s 硬上限
// （裸调 12000 tokens 60.6s → 504 Gateway Time-out）；单文件生成实测 36s/11913 字符 ✅。
// ⇒ 拆成 main.go（自包含完整服务）一次调用 + go.mod/README 小文件，避免网关 504。
//
// 返回 (files, note)：files=nil 表示 LLM 不可用/迭代耗尽（调用方回落确定性骨架）；
// note 为失败原因或"编译全绿（N 轮）"。
func (o *Options) llmGenerateImplement(ctx context.Context, title, doc, skelDir, memory string) (map[string]string, string) {
	if o == nil || o.Providers == nil {
		return nil, "providers nil"
	}
	// 清掉骨架残留 .go（2026-10-03 实证：骨架 domain.go/router.go 与 LLM 自包含 main.go 冲突，
	// 编译报 ./router.go:21 undefined: writeJSON → 修复轮永远修不掉非 LLM 生成的文件）。
	// LLM 自包含生成后只保留 LLM 产物（main.go/go.mod/README），目录内其他 .go 一律移除。
	if ents, err := os.ReadDir(skelDir); err == nil {
		for _, e := range ents {
			n := e.Name()
			if strings.HasSuffix(n, ".go") {
				_ = os.Remove(filepath.Join(skelDir, n))
			}
		}
	}
	// 模型调度（2026-10-03）：fast 失败自动降级 center→strong→gpt-mini，不再单点卡死。
	// genWithPref：按 pref 降级链调度（fast=deepseek 等；gpt4o/gpt-mini 质量最稳）。
	// 2026-10-03 真跑实证：①fast 曾 1.8s 返回<200 字符短输出 → 判无效（输出质量无法被调度层感知）；
	// ②gpt-4o 输出必带 ```go Markdown 围栏 → 直接写盘必编译失败（expected 'package'）→ stripCodeFence 剥离。
	genWithPref := func(pref []string, minLen int, sysMsg, usrMsg string) (string, string) {
		resp, _, err := o.Providers.ChatWithFallback(ctx, pref, provider.ChatRequest{
			Messages: []contract.Message{
				{Role: "system", Content: sysMsg},
				{Role: "user", Content: usrMsg},
			},
			// 2026-10-03 真跑实证：MaxTokens 上限会令模型用满 12000 tokens → 网关 60s 504/超时；
			// 不设上限 → 模型自然收敛（裸调实测 36s/11913 字符）。故此处不传 MaxTokens。
			ResponseFormat: noJSON(),
		})
		if err != nil {
			return "", "fast Chat err: " + err.Error()
		}
		c := stripCodeFence(resp.Content)
		if len(c) < minLen {
			return "", fmt.Sprintf("LLM 输出过短(%d 字符)判为无效，调度跳过", len(c))
		}
		return c, ""
	}
	// 质量门按文件分级：main.go≥200（完整服务）；go.mod≥20（module+go 行仅 ~35 字符，
	// 2026-10-03 实证 200 会误杀合法 go.mod → 整个实现失败）；README≥50。
	gen := func(sysMsg, usrMsg string) (string, string) {
		return genWithPref([]string{"fast", "center", "strong", "gpt-mini"}, 200, sysMsg, usrMsg)
	}

	// 2026-10-03 模型调度实证：需求全文（7966 字符）直接喂 LLM → 大 prompt + 完整生成必然
	// 超网关 60s 上限（504）。正解：喂**能力清单摘要**（小 prompt，10s 级收敛），
	// 语义判据由证据门（P0 关键字 + go build 真编译）兜底，缺口经 RoundEvidence 回喂。
	req := "目标产品标题：" + title + "\n\nP0 能力清单（按此实现，可合理扩展）：\n" + implCapabilityBrief + memoryBlock(memory)
	if o.RoundEvidence != "" {
		// L-01 架构：第 2+ 轮携带上一轮证据门缺口（对齐 dsh goal-round 的"完成前收集证据"）。
		req += "\n\n上一轮证据门缺口（本轮必须补齐后才能验收）：\n" + o.RoundEvidence + "\n"
	}
	files := map[string]string{}

	// ① main.go：自包含完整服务（词典/纠错/意图/反馈/JSONL 落盘/端点/健康检查）——大文件独立调用。
	sysMain := "你是资深 Go 工程师。只输出 main.go 的**完整代码文本**（自包含、可直接 go build 通过的服务）。" +
		"硬性要求：①仅用标准库，零第三方依赖；②不许留 TODO/占位/伪代码；③实现需求文档 P0 核心能力（词典增删查/纠错/意图分类/反馈/数据 JSONL 落盘 append-only）；" +
		"④需求文档要求/提及的**每一个 /v1/ 端点**都必须用 http.HandleFunc(\"/v1/...\", …) 字面量逐一注册（验收会按需求端点清单逐端点核对，缺一即不合格）；" +
		"⑤可独立运行（监听 127.0.0.1，addr/data-dir 用 flag 或环境变量）。" +
		"纯文本输出，不要 Markdown 围栏、不要 JSON、不要解释。"
	mainCode, note := genWithPref([]string{"gpt4o", "gpt-mini", "fast", "center", "strong"}, 200, sysMain, req)
	if mainCode == "" {
		return nil, "main.go 生成失败: " + note
	}
	files["main.go"] = mainCode

	// ② go.mod：小文件独立调用。
	modCode, note := genWithPref([]string{"fast", "center", "strong", "gpt-mini"}, 20, "你是 Go 工程师。只输出 go.mod 的完整文本：module 名用 harness-output/impl（ASCII 小写，中文 module 非法），go 版本 1.21。纯文本，不要围栏。", req)
	if modCode == "" {
		return nil, "go.mod 生成失败: " + note
	}
	files["go.mod"] = modCode

	// ③ README.md：小文件，失败不致命（跳过仍可编译）。
	if rd, rn := gen("你是技术文档作者。输出 README.md 的简短中文运行说明（启动命令/端点/数据文件）。纯文本，不要围栏。", req); rd != "" {
		files["README.md"] = rd
	} else {
		log.Printf("[llmGenerateImplement] README 生成跳过（%s）", rn)
	}

	if err := writeFilesToDisk(skelDir, files); err != nil {
		return nil, "写盘失败: " + err.Error()
	}
	// 真编译验证（不许桩）：go build -C <skelDir> ./...（Go 1.20+ -C 支持，runCmd Dir 固定故用 -C）。
	buildRecv := o.run("run", map[string]any{"command": []string{"go", "build", "-C", skelDir, "./..."}})
	if buildRecv.OK {
		return files, "编译全绿（0 轮修复）"
	}
	// 编译失败 → ①确定性清理未使用 import（机械错误，LLM 修复不稳定——2026-10-03 实证 3 轮仍失败）；
	// ②仍失败才回喂 LLM（≤3 轮）。
	log.Printf("[llmGenerate] 编译失败（首轮），错误：\n%s", truncateStr(buildRecv.Stdout+"\n"+buildRecv.Stderr, 1200))
	// 调试保留：LLM 产物副本（骨架兜底会覆盖写盘产物，这里留一份供编译错误分析）。
	_ = os.WriteFile("/tmp/llm_main_debug.go", []byte(files["main.go"]), 0o644)
	cleanErrs := buildRecv.Stdout + "\n" + buildRecv.Stderr
	lastErr := truncateStr(cleanErrs, 2500)
	if cleaned, n := removeUnusedImports(files["main.go"], cleanErrs); cleaned != "" {
		log.Printf("[llmGenerate] 确定性清理 %d 个未使用 import，重编译", n)
		files["main.go"] = cleaned
		if err := writeFilesToDisk(skelDir, files); err == nil {
			recv := o.run("run", map[string]any{"command": []string{"go", "build", "-C", skelDir, "./..."}})
			if recv.OK {
				return files, fmt.Sprintf("编译全绿（确定性清理 %d 个未使用 import）", n)
			}
			// 关键：清理后仍有错误（非 import），必须更新 lastErr——
			// 否则修复轮拿旧错误（import）修，永远修不掉清理后暴露的真实错误（2026-10-03 实证循环）。
			log.Printf("[llmGenerate] 清理后仍失败，更新修复轮错误信息：\n%s", truncateStr(recv.Stdout+"\n"+recv.Stderr, 800))
			lastErr = truncateStr(recv.Stdout+"\n"+recv.Stderr, 2500)
		}
	}
	for i := 1; i <= 3; i++ {
		mainCode, note = genWithPref([]string{"gpt4o", "gpt-mini", "fast", "center", "strong"}, 200, sysMain+"\n\n上一轮 main.go **编译失败**，请仅修复编译错误后重新输出**完整 main.go**（不要省略其它部分）。\n\n编译错误：\n"+lastErr, req)
		if mainCode == "" {
			return nil, "main.go 修复失败: " + note
		}
		// 修复轮产物同样先做确定性 import 清理（LLM 重生成必带未用 import——
		// 2026-10-03 实证：不清理则修复轮永远卡在 "imported and not used"，3 轮耗尽）。
		if c2, n2 := removeUnusedImports(mainCode, lastErr); c2 != "" {
			log.Printf("[llmGenerate] 修复轮 %d 确定性清理 %d 个未使用 import", i, n2)
			files["main.go"] = c2
		} else {
			files["main.go"] = mainCode
		}
		if err := writeFilesToDisk(skelDir, files); err != nil {
			return nil, "写盘失败: " + err.Error()
		}
		buildRecv = o.run("run", map[string]any{"command": []string{"go", "build", "-C", skelDir, "./..."}})
		if buildRecv.OK {
			return files, fmt.Sprintf("编译全绿（%d 轮修复）", i)
		}
		lastErr = truncateStr(buildRecv.Stdout+"\n"+buildRecv.Stderr, 2500)
	}
	return nil, "编译迭代 3 轮仍未通过"
}

// parseGenFiles 解析 LLM 返回的 {"files":{...}} JSON（容忍 ```json 围栏与前后杂质）。
func parseGenFiles(content string) (map[string]string, string) {
	c := strings.TrimSpace(content)
	if i := strings.Index(c, "```"); i >= 0 {
		// 去掉首个围栏行与结尾围栏
		rest := c[i:]
		if j := strings.Index(rest, "\n"); j >= 0 {
			rest = rest[j+1:]
		}
		if k := strings.LastIndex(rest, "```"); k >= 0 {
			rest = rest[:k]
		}
		c = strings.TrimSpace(rest)
	}
	if i := strings.Index(c, "{"); i >= 0 {
		c = c[i:]
	}
	if j := strings.LastIndex(c, "}"); j >= 0 {
		c = c[:j+1]
	}
	var parsed struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal([]byte(c), &parsed); err != nil {
		return nil, "LLM 输出非合法 JSON: " + err.Error()
	}
	if len(parsed.Files) == 0 {
		return nil, "LLM 输出空文件集"
	}
	return parsed.Files, ""
}

// writeFilesToDisk 把生成文件写入 skelDir（先建目录）。
func writeFilesToDisk(dir string, files map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, content := range files {
		if name != filepath.Base(name) {
			return fmt.Errorf("非法文件名（含路径分隔）: %q", name)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// removeUnusedImports 从 Go 源码确定性移除编译错误报告中的未使用 import（机械错误）。
// 匹配 go build 错误形如 `./main.go:4:2: "bufio" imported and not used`；import 块内的
// `	"bufio"` 行逐行删除。2026-10-03 实证：LLM 修复轮对这类错误修复不稳定（3 轮仍失败），
// 确定性清理是 harness 的工程兜底（不依赖 LLM 运气）。返回清理后的源码与清理数量。
func removeUnusedImports(src, buildOut string) (string, int) {
	unused := map[string]bool{}
	for _, m := range regexp.MustCompile(`"(.*?)" imported and not used`).FindAllStringSubmatch(buildOut, -1) {
		if len(m) == 2 && m[1] != "" {
			unused[m[1]] = true
		}
	}
	if len(unused) == 0 {
		return "", 0
	}
	var sb strings.Builder
	removed := 0
	for _, line := range strings.Split(src, "\n") {
		trim := strings.TrimSpace(line)
		skip := false
		for imp := range unused {
			if trim == `"`+imp+`"` {
				skip = true
				break
			}
		}
		if skip {
			removed++
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return strings.TrimSuffix(sb.String(), "\n"), removed
}

// EvidenceGaps（L-01 架构 2026-10-03）：对一次实现的产物做**证据门**检查，
// 返回缺口列表；空列表 = 证据门全过（产物存在且非空、真编译通过、P0 能力存在性）。
//
// 判据（对齐 dsh"完成前收集证据"）：
//  1. 产物目录存在且 main.go 非空（骨架/占位/空文件 → 缺口）
//  2. main.go 不含 TODO/占位标记（LLM 语义实现 vs 确定性骨架兜底的区分）
//  3. go build -C <产物根> ./... 真编译通过（不许桩）
//  4. P0 能力存在性：词典/纠错/意图/反馈/JSONL 落盘/健康检查（关键字扫描产物源码）
func (o *Options) EvidenceGaps(out *Outcome) []string {
	if out == nil {
		return []string{"任务无产物（Outcome 为空）"}
	}
	// 从回执提取 harness-output/ 产物根目录（stdout 形如 "writed: /abs/harness-output/<title>/main.go"）。
	// 2026-10-03 修复：stdout 多行（writed 路径 + VHS_BACKUP_PATH: ...），TrimPrefix 后 line 是整块，
	// HasSuffix(main.go) 永远失败 → implRoot 空 → Glob 回退取错目录（读到别的任务骨架 → 判"含 todo"假 FAIL）。
	// 只取第一行（writed 路径行）。
	implRoot := ""
	for _, r := range out.Receipts {
		stdout := strings.TrimSpace(r.Stdout)
		if nl := strings.IndexByte(stdout, '\n'); nl > 0 {
			stdout = stdout[:nl]
		}
		line := strings.TrimSpace(strings.TrimPrefix(stdout, "writed:"))
		if p := filepath.Clean(line); strings.Contains(p, "harness-output") {
			if strings.HasSuffix(p, "main.go") || strings.HasSuffix(p, "go.mod") {
				implRoot = filepath.Dir(p)
			}
		}
	}
	if implRoot == "" {
		// 回执未必含路径：回退扫项目根 harness-output/。
		if candidates, _ := filepath.Glob("harness-output/*"); len(candidates) > 0 {
			implRoot = candidates[len(candidates)-1]
		}
	}
	if implRoot == "" {
		return []string{"未找到实现产物目录（harness-output/ 缺失）"}
	}

	var gaps []string
	mainPath := filepath.Join(implRoot, "main.go")
	mainBytes, err := os.ReadFile(mainPath)
	if err != nil || len(mainBytes) == 0 {
		return append(gaps, "main.go 缺失或为空（"+mainPath+"）")
	}
	mainSrc := string(mainBytes)
	// 判据 2：骨架/占位检测（确定性骨架的特征注释）。
	low := strings.ToLower(mainSrc)
	for _, marker := range []string{"todo", "占位", "not implemented", "code generated by voice sign harness"} {
		if strings.Contains(low, marker) {
			gaps = append(gaps, "main.go 仍是骨架/占位（含 `"+marker+"` 标记），未产出完整实现")
			break
		}
	}
	// 判据 4：P0 能力存在性（关键字扫描——判据为"有实现痕迹"，非语义级）。
	type p0 struct{ key, name string }
	p0s := []p0{
		{"dictionary", "词典（增删查）"}, {"dict", "词典"},
		{"correct", "纠错"}, {"intent", "意图分类"},
		{"feedback", "反馈学习"}, {"jsonl", "JSONL 落盘"},
		{"append", "append-only 落盘"}, {"/v1/health", "健康检查"},
		{"v1/process", "业务端点 /v1/process"},
	}
	seen := map[string]bool{}
	for _, p := range p0s {
		if seen[p.name] {
			continue
		}
		seen[p.name] = true
		if !strings.Contains(low, p.key) {
			gaps = append(gaps, "缺 P0 能力实现："+p.name+"（源码中未见 `"+p.key+"`）")
		}
	}
	// 判据 3：真编译（不许桩）。
	buildRecv := o.run("run", map[string]any{"command": []string{"go", "build", "-C", implRoot, "./..."}})
	if !buildRecv.OK {
		gaps = append(gaps, "真编译失败："+truncateStr(buildRecv.Stderr, 300))
	}
	// 判据 5：需求端点 ↔ 产物路由端点存在性（洞 1，2026-10-04 无人工干预测试复现：
	// 需求 7 端点产物仅 2 端点（/v1/health+/v1/process），P0 关键字仍判"齐全"放行）。
	// 需求侧：从 o.Document（需求全文）提取所有出现的 /v1/xxx 字面量；
	// 产物侧：从 main.go 提取 http.HandleFunc("/v1/xxx") 已注册端点；
	// 差集 = 缺口 → 证据门 FAIL → 多轮修订（RoundEvidence 回喂 LLM 补齐端点）。
	if o.Document != "" {
		reqEP := endpointRefsOf(o.Document)
		prodEP := endpointHandlersOf(mainSrc)
		for ep := range reqEP {
			if !prodEP[ep] {
				gaps = append(gaps, "缺需求端点实现："+ep+"（需求文档要求，产物未注册 http.HandleFunc）")
			}
		}
	}
	return gaps
}

// endpointRefsOf 提取文本中出现的所有 /v1/xxx 端点字面量（需求侧：文档里提到的即算需求端点）。
func endpointRefsOf(src string) map[string]bool {
	set := map[string]bool{}
	re := regexp.MustCompile(`/v1/[a-z_]+`)
	for _, m := range re.FindAllString(src, -1) {
		set[m] = true
	}
	return set
}

// endpointHandlersOf 提取源码中 http.HandleFunc("/v1/xxx", …) 已注册的端点（产物侧：注册才算实现）。
func endpointHandlersOf(src string) map[string]bool {
	set := map[string]bool{}
	re := regexp.MustCompile(`HandleFunc\("/v1/[a-z_]+`)
	for _, m := range re.FindAllString(src, -1) {
		set[strings.TrimPrefix(m, `HandleFunc("`)] = true
	}
	return set
}

// deterministicImplementSkeleton 生成可编译 Go 代码骨架（LLM 不可用时仍产出）。
// 文件：README.md / go.mod / main.go / router.go / domain.go（服务骨架 + 需求映射）。
// 目标：harness-output/<title>/ 下 `go build ./...` 可通过；实现逻辑留 TODO 交实现阶段。
func deterministicImplementSkeleton(title, doc string) map[string]string {
	// module 名必须 ASCII（Go 限制：中文 module path 非法）；目录名可保留中文。
	mod := "harness-output/" + asciiSlug(title)
	if mod == "harness-output/" {
		mod = "harness-output/impl"
	}
	// 从需求文档提取章节标题作为领域要点注释（真实结构，不编造）。
	var chapters []string
	for _, ln := range strings.Split(doc, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "#") && !strings.HasPrefix(ln, "## 全文") {
			chapters = append(chapters, strings.TrimLeft(ln, "# "))
		}
	}
	if len(chapters) > 12 {
		chapters = chapters[:12]
	}
	chapNote := "  // 需求章节：\n"
	for _, c := range chapters {
		chapNote += "  //  - " + c + "\n"
	}

	mainGo := `// Code generated by VoiceSign Harness (ORCHESTRATE kind=implement). DO NOT EDIT manually.
// 目标产品：` + title + `
// 骨架：可编译服务入口（配置/路由/健康检查/鉴权占位）。实现逻辑见 domain.go 与 router.go 的 TODO。
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := flag.String("addr", envOr("VHS_ADDR", "127.0.0.1:8787"), "监听地址（默认回环，非回环拒绝）")
	dataDir := flag.String("data-dir", envOr("VHS_DATA", "./data"), "数据目录")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	app := NewApp(*dataDir)

	mux := http.NewServeMux()
	app.RegisterRoutes(mux) // router.go

	srv := &http.Server{Addr: *addr, Handler: mux}
	log.Printf("服务监听 %s（数据目录 %s）", *addr, *dataDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("服务退出: %v", err)
	}
}

// writeJSON 统一 JSON 响应（与 vhs-asr 契约一致）。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
`

	routerGo := `// Code generated by VoiceSign Harness (ORCHESTRATE kind=implement). DO NOT EDIT manually.
// 路由注册：/v1/health 健康检查 + /v1/process 核心处理（契约骨架，TODO 由实现阶段填充）。
package main

import "net/http"

// App 是服务装配根对象（领域逻辑在 domain.go）。
type App struct {
	DataDir string
}

func NewApp(dataDir string) *App { return &App{DataDir: dataDir} }

// RegisterRoutes 注册全部端点（鉴权占位：回环绑定 + 可选 token 头）。
func (a *App) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/health", a.handleHealth)
	mux.HandleFunc("/v1/process", a.handleProcess)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data_dir": a.DataDir})
}

// handleProcess 核心处理端点：TODO 按需求文档实现（意图/词典/反馈/个性化闭环）。
func (a *App) handleProcess(w http.ResponseWriter, r *http.Request) {
	// TODO(实现阶段): 解析请求体 → 领域处理 → 响应（含 traces 留痕）
	writeJSON(w, http.StatusNotImplemented, map[string]any{"ok": false, "error": "骨架未实现"})
}
`

	domainGo := `// Code generated by VoiceSign Harness (ORCHESTRATE kind=implement). DO NOT EDIT manually.
// 领域层骨架：按需求文档拆分的模块接口与 TODO。
package main

//` + chapNote + `
// TODO(实现阶段)：
//  1. 意图/词典/反馈等核心机制的领域实现（对照需求文档各章）
//  2. 数据层持久化（文件/DB，append-only、有界、过期策略）
//  3. 非功能验收：鉴权、上限、性能分步计时、红线 4 条逐条落地
//  4. 验收映射：每达成一条验收标准回填证据（见 docs/<title>-实现计划.md）
`

	goMod := "module " + mod + "\n\ngo 1.21\n"

	readme := `# ` + title + `（Harness 产出代码骨架）

> 由 VoiceSign Harness 多步编排（ORCHESTRATE kind=implement）自动生成，2026-10-03。

## 内容
- main.go 服务入口（配置/健康检查/鉴权占位）
- router.go 路由注册（/v1/health、/v1/process）
- domain.go 领域层 TODO（需求章节见文件头注释）

## 使用
` + "```bash\ncd " + mod + "\ngo build ./...   # 骨架可编译\n" + "```\n" + `

## 需求映射
- 实现计划（完整）：docs/` + sanitizePathPart(title) + `-实现计划.md
- 需求全文：用户提交的附件 document（见实现计划「需求文档全文摘录」）

## 状态
骨架阶段（可编译、可运行 /v1/health）；核心逻辑待实现阶段按需求文档填充。
`

	return map[string]string{
		"README.md": readme,
		"go.mod":    goMod,
		"main.go":   mainGo,
		"router.go": routerGo,
		"domain.go": domainGo,
	}
}

// deterministicImplementPlan 确定性生成实现计划（LLM 不可用时的降级，仍产出真实可迭代文件）。
// 结构：目标 → 需求要点提取（章节标题/关键词）→ 模块清单 → 接口/数据契约 → 验收映射 → 实施步骤。
func deterministicImplementPlan(title, doc string) string {
	var sb strings.Builder
	sb.WriteString("## 实现目标\n\n")
	fmt.Fprintf(&sb, "- 产品：%s\n", title)
	sb.WriteString("- 依据：用户提供的需求文档（见文末全文摘录）\n\n")

	sb.WriteString("## 需求要点（确定性提取）\n\n")
	lines := strings.Split(doc, "\n")
	type hdr struct {
		level int
		text  string
	}
	headers := make([]hdr, 0, 32)
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "#") {
			level := 0
			for _, r := range ln {
				if r != '#' {
					break
				}
				level++
			}
			text := strings.TrimSpace(strings.TrimLeft(ln, "#"))
			headers = append(headers, hdr{level, text})
		}
	}
	if len(headers) == 0 {
		sb.WriteString("- 文档未含 Markdown 标题；建议人工复核需求结构。\n")
	} else {
		for _, h := range headers {
			if h.level == 1 {
				fmt.Fprintf(&sb, "- **%s**\n", h.text)
			} else if h.level == 2 {
				fmt.Fprintf(&sb, "  - %s\n", h.text)
			}
		}
	}

	sb.WriteString("\n## 建议模块划分（骨架，待实现时细化）\n\n")
	sb.WriteString("| 模块 | 职责 | 关键接口 |\n|---|---|---|\n")
	sb.WriteString("| cmd/ 入口 | 服务装配与启动 | main() |\n")
	sb.WriteString("| server/ 路由 | HTTP 端点与鉴权 | /v1/* |\n")
	sb.WriteString("| 领域逻辑 | 需求核心机制（意图/词典/反馈） | 领域服务方法 |\n")
	sb.WriteString("| 数据层 | 持久化（文件/DB） | 读写接口 |\n")

	sb.WriteString("\n## 验收映射（以需求文档验收标准为准）\n\n")
	sb.WriteString("> 由实现阶段逐条对照需求文档验收标准展开，每达成一条回填证据。\n\n")

	sb.WriteString("## 实施步骤（骨架）\n\n")
	sb.WriteString("1. 解析需求文档，抽取模块与接口契约\n")
	sb.WriteString("2. 搭服务骨架（路由/配置/数据目录）\n")
	sb.WriteString("3. 实现核心机制，逐模块真跑验证\n")
	sb.WriteString("4. 对照验收标准逐条复核，补证据\n")
	sb.WriteString("5. 交付（含自测与验收报告）\n\n")

	sb.WriteString("## 需求文档全文摘录\n\n```markdown\n")
	sb.WriteString(truncateStr(doc, 6000))
	sb.WriteString("\n```\n")
	return sb.String()
}


// commitTargetPath 定向提交单个文件：git add -- <abs> →（无 diff 则幂等跳过）→ git commit → git log -1 取 hash。
// 绝不 `git add -A`，避免扫入工作区无关未跟踪文件。
// 幂等：该文件相对暂存区/HEAD 无变化（重跑同内容）时，不触发 "nothing to commit" 失败，
// 而是视为成功收尾，receipt 注明"内容无变化，跳过提交（已是最新）"。
func (o *Options) commitTargetPath(root, absPath, msg string) contract.Receipt {
	add := exec.Command("git", "add", "--", absPath)
	add.Dir = root
	if out, err := add.CombinedOutput(); err != nil {
		return contract.Receipt{Tool: "git", OK: false, Err: "git add 失败: " + string(out)}
	}
	// 仅看本路径是否进入暂存区（有 diff）；空=无变化 → 幂等跳过提交。
	ch := exec.Command("git", "diff", "--cached", "--name-only", "--", absPath)
	ch.Dir = root
	names, _ := ch.Output()
	log := func() string {
		l := exec.Command("git", "log", "-1", "--format=%H %s")
		l.Dir = root
		if lout, err := l.Output(); err == nil {
			return strings.TrimSpace(string(lout))
		}
		return ""
	}
	if len(strings.TrimSpace(string(names))) == 0 {
		return contract.Receipt{Tool: "git", OK: true,
			Stdout: "内容无变化，跳过提交（已是最新）\n" + log()}
	}
	cm := exec.Command("git", "commit", "-m", msg)
	cm.Dir = root
	cout, err := cm.CombinedOutput()
	var stdout strings.Builder
	stdout.Write(cout)
	if err != nil {
		return contract.Receipt{Tool: "git", OK: false, Stdout: stdout.String(), Err: "git commit: " + err.Error()}
	}
	if l := log(); l != "" {
		stdout.WriteString("\n" + l)
	}
	return contract.Receipt{Tool: "git", OK: true, Stdout: stdout.String()}
}

// planVerify 给可独立复核的意图生成 verify.Spec；否则返回 nil（unverifiable）。
// COMMIT 的独立证据直接落在回执 stdout（git log -1 输出），不另设 verify.Kind（verify 包不可改）。
func (o *Options) planVerify(it contract.Intent, logDir string) *verify.Spec {
	switch it.Intent {
	case contract.IntentNote:
		return &verify.Spec{Kind: "file", Args: []string{filepath.Join(logDir, "notes.md")}, BaseDir: logDir}
	default:
		return nil
	}
}

// projectRootForCommit 解析 COMMIT 意图域的项目根（M4-4）。
//
// 【伪代码逻辑层】（执行根解析属裁决逻辑）：
//
//	if intent.Intent != COMMIT: return ""（其他意图保持 logDir）。
//	m = o.Spaces.Get(intent.Space)；无 manifest → return ""。
//	对 m.Scope 第 1 条：剥 "/**" 后缀 → Clean → 必须是已存在目录 → 否则 return ""。
//	硬约束：根不得 ==/under logDir（防把 logDir 当项目库）。
//	return 根路径。
func (o *Options) projectRootForCommit(it contract.Intent) string {
	if it.Intent != contract.IntentCommit && it.Intent != contract.IntentOrchestrate {
		return ""
	}
	if o == nil || o.Spaces == nil {
		return ""
	}
	m, ok := o.Spaces.Get(it.Space)
	if !ok || m == nil || len(m.Scope) == 0 {
		return ""
	}
	root := strings.TrimSuffix(m.Scope[0], "/**")
	root = filepath.Clean(root)
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return ""
	}
	return root
}

// gitDirtyCount 在 root 跑 `git status --porcelain` 统计未提交改动行数。
// 非 git 仓/命令失败 → 返回 -1（保守：确认文案不写计数，但仍提交）。
func gitDirtyCount(root string) int {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// mechanicalImpact 用真实 fs 机械算影响面（RefCount/HasTest/Heat）。
//
// 【伪代码逻辑层】（必写模块：引用定义/排除规则/上限；阈值定义搬 VSL risk.StaticImpact）
//
// 控制流：
//
//	target = targetPath(it)；空 → return 零值（回退 auto/small，绝不臆造）。
//	m = o.Spaces.Get(it.Space)；无 manifest → return 零值。
//	roots = m.Scope 剥 "/**" 后取目录；
//	  硬排除（M2 失败模式防回归）：跳过任何 ==/under logDir 的 root，
//	  Ignore 注入 [".git","node_modules","memory","data"]——轨迹/discuss/decisions/词典
//	  自身绝不能被算成"引用"。
//	if len(roots)==0: return 零值。
//	hits = search.FindText(target, {Roots:roots, Ignore})
//	RefCount = 去重后命中文件数；封顶 refCap（默认 50，防大库爆量）。
//	HasTest = 任一 root 下存在 *_test.go 文件（filepath.Walk 一级深度即可）。
//	Heat    = 当日 trajectory-*.jsonl 中含 target basename 的行数（字段匹配才计数）。
//	return {RefCount, HasTest, Heat}。
//
// 异常：search 返回 err → RefCount=0（保守 small）；轨迹文件读不到 → Heat=0。
const refCap = 50

func (o *Options) mechanicalImpact(it contract.Intent) risk.ImpactInput {
	imp := risk.ImpactInput{}
	target := targetPath(it)
	if target == "" {
		return imp
	}
	if o.Spaces == nil {
		return imp
	}
	m, ok := o.Spaces.Get(it.Space)
	if !ok {
		return imp
	}

	// scope roots，剥 /**/* 后缀
	logAbs, _ := filepath.Abs(o.logDir())
	var roots []string
	for _, s := range m.Scope {
		s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(s), "/**"), "/*")
		if s == "" || s == "." || strings.HasPrefix(s, "~") {
			continue
		}
		abs, err := filepath.Abs(s)
		if err != nil {
			continue
		}
		// 硬排除：root 落在 log_dir 内（防把轨迹/决策日志当引用）
		if logAbs != "" && (abs == logAbs || strings.HasPrefix(abs, logAbs+string(os.PathSeparator))) {
			continue
		}
		roots = append(roots, abs)
	}
	if len(roots) == 0 {
		return imp
	}

	ignore := []string{".git", "node_modules", "memory", "data"}

	// RefCount = 引用 target 的去重文件数
	if hits, err := search.FindText(target, search.Options{Roots: roots, Ignore: ignore}); err == nil {
		files := map[string]bool{}
		for _, h := range hits {
			files[h.File] = true
		}
		imp.RefCount = len(files)
		if imp.RefCount > refCap {
			imp.RefCount = refCap
		}
	}

	// HasTest = scope 内存在 *_test.go
	imp.HasTest = hasTestFile(roots, ignore)

	// Heat = 当日轨迹中命中 target basename 的次数
	imp.Heat = o.trajectoryHeat(target)
	return imp
}

// hasTestFile 在 roots 下找 *_test.go（尊重 ignore 段）。
func hasTestFile(roots, ignore []string) bool {
	for _, root := range roots {
		found := false
		_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
			if err != nil || found {
				return nil
			}
			base := fi.Name()
			for _, pat := range ignore {
				if ok, _ := filepath.Match(pat, base); ok {
					if fi.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
			if fi.IsDir() {
				return nil
			}
			if strings.HasSuffix(base, "_test.go") {
				found = true
				return filepath.SkipAll
			}
			return nil
		})
		if found {
			return true
		}
	}
	return false
}

// trajectoryHeat 数当日轨迹文件中含 target basename 的行数。
func (o *Options) trajectoryHeat(target string) int {
	base := filepath.Base(target)
	if base == "" || base == "." {
		return 0
	}
	path := filepath.Join(o.logDir(), "trajectory-"+time.Now().Format("20060102")+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	heat := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, base) {
			heat++
		}
	}
	return heat
}

func (o *Options) logDir() string {
	if o.Cfg != nil && o.Cfg.Global.LogDir != "" {
		return o.Cfg.Global.LogDir
	}
	return os.TempDir()
}

// fastResponseMs 返回快速响应阈值（Global.FastResponseMs，<=0 归一化为 10000）。
func (o *Options) fastResponseMs() int {
	if o.Cfg != nil && o.Cfg.Global.FastResponseMs > 0 {
		return o.Cfg.Global.FastResponseMs
	}
	return 10000
}

// selfheal 懒装配异常自愈层。Providers==nil（testOptions 现状/纯规则路径）→ 恒 nil，
// 诊断层零开销跳过，主链行为 100% 不变。diag provider 未注册 → 模型环 nil（知识库环仍可用）。
func (o *Options) selfheal() *selfheal.Service {
	if o == nil || o.Providers == nil {
		return nil
	}
	if o.selfhealSvc != nil {
		return o.selfhealSvc
	}
	var diag provider.Provider
	if p, err := o.Providers.Get("diag"); err == nil {
		diag = p
	}
	kb := selfheal.OpenKB(filepath.Join(o.logDir(), selfheal.KBFileName))
	o.selfhealSvc = selfheal.NewService(kb, diag, o.run)
	return o.selfhealSvc
}

// repairFailed 对失败回执跑异常自愈（可选层）：诊断 + 只读安全重放（限 2 轮）。
// 重放成功的新回执由主链合并进 out.Receipts；诊断结论暂存供归因。诊断层未配置/失败 → 返回 nil。
func (o *Options) repairFailed(ctx context.Context, it contract.Intent, receipts []contract.Receipt) []contract.Receipt {
	svc := o.selfheal()
	if svc == nil {
		return nil
	}
	var repaired []contract.Receipt
	for i := range receipts {
		r := receipts[i]
		if r.OK {
			continue
		}
		att := selfheal.Attempt{Tool: r.Tool, Args: o.argsForFailed(it, r.Tool), Receipt: r}
		if nr, _ := svc.SafeRetry(ctx, it.RawText, it.Intent, att); nr != nil {
			repaired = append(repaired, *nr)
		}
	}
	return repaired
}

// argsForFailed 为重放重建最小参数（只有只读族失败才会真重放；写类重建了也被 IsReadOnly 拦下）。
func (o *Options) argsForFailed(it contract.Intent, tool string) map[string]any {
	switch tool {
	case "search":
		pattern := it.CorrectedText
		if it.Params != nil && it.Params["object"] != "" {
			pattern = it.Params["object"]
		}
		return map[string]any{"pattern": pattern, "kind": "text"}
	case "file":
		// NOTE 追加（写类，不会被自动重放）。
		return map[string]any{"action": "append", "path": filepath.Join(o.logDir(), "notes.md"), "log_dir": o.logDir()}
	default:
		return map[string]any{}
	}
}

// renderGround 渲染认知切片（#37）；Ground 未配置 → 空快照（薄降级，Ask 照常走）。
func (o *Options) renderGround() ground.Snapshot {
	if o.Ground != nil {
		return o.Ground.Render()
	}
	return ground.Snapshot{}
}

// recordDecision 在确认闸落盘一条裁决（#37 decisions.jsonl 数据源）。
func (o *Options) recordDecision(rid string, it contract.Intent, d risk.Decision, approved bool) {
	if o.Ground == nil {
		return
	}
	confirm := "auto_skipped"
	if d.Level != contract.ConfirmAuto {
		if approved {
			confirm = "approved"
		} else {
			confirm = "rejected"
		}
	}
	_ = o.Ground.RecordDecision(ground.Decision{
		Ts:       time.Now().Format(time.RFC3339),
		TaskID:   rid,
		Intent:   it.Intent,
		Decision: d.Level,
		Confirm:  confirm,
		Reason:   d.Reason,
	})
}

func quadKey(it contract.Intent, level string) cache.QuadKey {
	ref := ""
	if it.Target != nil {
		ref = it.Target.Entity
	}
	return cache.QuadKey{Intent: it.Intent, Space: it.Space, Perm: level, Ref: ref}
}

func targetPath(it contract.Intent) string {
	if it.Params != nil {
		if p := it.Params["object"]; p != "" {
			return p
		}
		if p := it.Params["path"]; p != "" {
			return p
		}
	}
	if it.Target != nil {
		return it.Target.Entity
	}
	return ""
}

// ---------- 归因（六格） ----------

func classifyAttribution(it contract.Intent, receipts []contract.Receipt, v verify.Result, corr []contract.Correction) (cls, detail, suggestion string) {
	switch {
	case len(corr) > 0:
		return contract.AttrInput, "ASR 经词典纠错 " + strconvItoa(len(corr)) + " 处后完成", "把高频误识别固化进词典"
	case hasFailure(receipts):
		return contract.AttrExec, "执行回执失败：" + firstErr(receipts), "检查环境/路径/权限后重试"
	case v.Status == verify.StatusFail:
		return contract.AttrExec, "独立校验未通过：" + v.Detail, "verify 不读自报，按真实 fs 修正"
	case v.Status == verify.StatusUnverifiable && it.Intent == contract.IntentNote:
		return contract.AttrModel, "NOTE 已追加，无独立校验必要", ""
	default:
		return contract.AttrModel, "意图分类正确，执行与校验通过", ""
	}
}

func hasFailure(rs []contract.Receipt) bool {
	for _, r := range rs {
		if !r.OK {
			return true
		}
	}
	return false
}

func firstErr(rs []contract.Receipt) string {
	for _, r := range rs {
		if !r.OK {
			if r.Err != "" {
				return r.Err
			}
			if r.Stderr != "" {
				return r.Stderr
			}
		}
	}
	return "未知执行错误"
}

// ---------- 视图（四行回执，SPEC §2.41） ----------

func renderView(it contract.Intent, v space.Verdict, d risk.Decision, rs []contract.Receipt, vr verify.Result, confirmed bool) contract.ReceiptView {
	view := contract.ReceiptView{
		Action: shortAction(it),
		Files:  targetFiles(it),
		Undo:   undoText(it, rs),
	}
	switch {
	case !confirmed:
		view.Result = "待确认（" + d.Level + "）"
	case hasFailure(rs) || vr.Status == verify.StatusFail:
		reason := firstErr(rs)
		if vr.Status == verify.StatusFail {
			reason = "verify 未通过：" + vr.Detail
		}
		view.Result = "FAILED：" + truncateStr(reason, 60)
	default:
		view.Result = "OK（" + confirmWord(d.Level) + "）"
		// M7：工具 stdout 有实质内容（QUERY 的 LLM 回答/降级文案等纯文本）时，
		// 四行回执的"结果"展示回答文本（截断 400；此前 120 会把多行结果吞掉后半），
		// 不再只显示"OK（自动执行）"空壳。
		if len(rs) > 0 {
			if s := strings.TrimSpace(rs[0].Stdout); s != "" && !strings.HasPrefix(s, "{") {
				view.Result = truncateStr(s, 400)
			}
		}
	}
	// ORCHESTRATE 多步链：回执要展示真实动作链（读 N 份文档 → 写文件 → git commit hash），
	// 不能只取第一条 file-read 的文档正文。
	if it.Intent == contract.IntentOrchestrate && !hasFailure(rs) {
		view.Action = "多步编排：读文档→汇总→写文件→git提交"
		view.Files = orchestrateFiles(rs)
		view.Result = orchestrateChainResult(rs)
		view.Undo = "不可撤销（已人工放行并 git 提交）"
	}
	return view
}

// orchestrateFiles 从多步回执里提取写文件目标（file write 回执里的 "writed: <path>"）。
func orchestrateFiles(rs []contract.Receipt) string {
	for _, r := range rs {
		if r.Tool == "file" && strings.HasPrefix(r.Stdout, "writed:") {
			return strings.TrimSpace(strings.TrimPrefix(r.Stdout, "writed:"))
		}
	}
	return "—"
}

// orchestrateChainResult 生成多步链的一句话结果（读 N 份 → commit hash）。
func orchestrateChainResult(rs []contract.Receipt) string {
	reads := 0
	for _, r := range rs {
		if r.Tool == "file" && !strings.HasPrefix(r.Stdout, "writed:") {
			reads++
		}
	}
	hash := ""
	for _, r := range rs {
		if r.Tool == "git" && r.OK {
			// git log -1 行格式："<hash> <subject>"，取最后一行首个 token。
			lines := strings.Split(strings.TrimSpace(r.Stdout), "\n")
			last := lines[len(lines)-1]
			if f := strings.Fields(last); len(f) > 0 && len(f[0]) >= 7 {
				hash = f[0]
			}
		}
	}
	return fmt.Sprintf("多步链完成：读 %d 份文档→汇总生成→git commit %s", reads, hash)
}

// intentCandidates 产出低置信回问的结构化意图候选（M4-3 ①）。
//
// 【伪代码逻辑层】（候选集生成属裁决逻辑）：
//
//	返回 4 个稳定意图候选 {id,label}：edit/query/note/commit。
//	id 与 label 一一对应、稳定可测；answer 传 id 时 server 续跑据此映射为强关键词前缀。
//
// optionsForIntent 按澄清意图动态生成候选（Codex/gpt-6-luna 诊断 2026-10-02）：
// 不再塞固定"改文件/查代码/记想法/提交"——
//
//	【伪代码逻辑层】
//	EDIT/DEBUG → refer 解析出的目标文件候选（无 → nil，不塞无关项）
//	QUERY → 回答范围/对象（当前无安全派生源 → nil，保留简短 Ask 文本）
//	NOTE → 笔记归属（无候选源 → nil）
//	其余 → nil
//	无安全候选时保留 Ask 文本即可（Codex："若无法安全地产生有效候选，保留简短 Ask 文本"）。
//
// 验证器：pipeline.TestCodexOptionsForIntent。
func optionsForIntent(it *contract.Intent, referOpts []refer.Option) []AskOption {
	switch it.Intent {
	case contract.IntentEdit, contract.IntentDebug:
		return referToAskOptions(referOpts)
	default:
		return nil
	}
}

// referToAskOptions 转换 refer 目标候选为 AskOption。
func referToAskOptions(referOpts []refer.Option) []AskOption {
	out := make([]AskOption, 0, len(referOpts))
	for _, o := range referOpts {
		out = append(out, AskOption{ID: o.ID, Label: o.Label})
	}
	return out
}

// shouldResolveRefer 判定是否对当前意图执行 refer 指代消解（M7，外部模型诊断方案定稿）。
//
// 【伪代码逻辑层】（裁决逻辑，方案来源：Codex/gpt-6-luna 外部诊断 2026-10-02 定稿）：
//
//  1. 口语问句特征（?？吗呢怎么如何为什么哪）→ false
//     （"如果这个效果好/看看效果怎么样"里的"这个/那个"是口语代词，不是操作指代——22:04 真机证据）。
//
//  2. 文件操作动词（把/将/打开/改/修/提交/删/建/换/设/存/写/跑/记/部署/上线/发布…）→ true
//     （操作指代强信号；"打开上次那个"即使被判 QUERY 也解析）。
//
//  3. QUERY 高置信（>=0.8）→ 仅裸指代（"查一下这个"对象悬空）仍解析；有实体（"这个方案"）不解析。
//
//  4. UNKNOWN（无操作动词）→ false（陈述引用/元指令，如"我那个前端的问题又不过来"——不因指代 Ask）。
//
//  5. NOTE/EDIT/COMMIT/DEBUG → true（真操作指代消解保持原行为）。
//
//     验证器：pipeline.TestShouldResolveReferGate（16 用例）+ Codex 9 项回归测试。
func shouldResolveRefer(it *contract.Intent) bool {
	text := it.CorrectedText
	if strings.ContainsAny(text, "?？吗呢怎么如何为什么哪") {
		return false
	}
	if hasFileOpVerb(text) {
		return true // 文件操作动词 → 操作指代（强操作信号），即使 QUERY 也解析
	}
	switch it.Intent {
	case contract.IntentQuery:
		// 查询对象裸指代（"查一下这个"）→ 真歧义仍解析；有实体（"这个方案"）→ 不解析
		if it.Confidence >= 0.8 {
			return isBareReferent(text)
		}
		return true
	case contract.IntentNote, contract.IntentEdit, contract.IntentCommit, contract.IntentDebug:
		return true
	case contract.IntentUnknown:
		// 陈述引用/元指令（"我那个前端的问题又不过来"）→ 不因指代 Ask，走分类器回问
		return false
	default:
		return true
	}
}

// fileOpVerbs 文件/记录操作动词集（Codex 诊断 2026-10-02）：命中视为"操作指代"强信号。
var fileOpVerbs = []string{"把", "将", "打开", "改", "修", "提交", "删", "建", "换", "设", "存", "写", "跑", "记", "部署", "上线", "发布", "复制", "移动", "重命名"}

// hasAnySubstr 子串匹配（input.containsAny 为包私有，pipeline 用同语义本地实现）。
func hasAnySubstr(text string, keywords []string) bool {
	for _, k := range keywords {
		if k != "" && strings.Contains(text, k) {
			return true
		}
	}
	return false
}

func hasFileOpVerb(text string) bool {
	return hasAnySubstr(text, fileOpVerbs)
}

// isBareReferent 判定指代词后无实质对象（"查一下这个"/"这个呢"→裸；"这个方案"→非裸）。
// ASR 噪音填充（"这个哈你真的开始推进起来"）后仍有实质内容 → 非裸，不触发指代 Ask。
func isBareReferent(text string) bool {
	for _, p := range []string{"这个", "那个"} {
		if i := strings.Index(text, p); i >= 0 {
			rest := text[i+len(p):]
			trimmed := strings.Trim(rest, " ，。、！？!?；;哈呀哦嗯啊呢吗吧的")
			return trimmed == ""
		}
	}
	return false
}

// clarificationBlocksExecution 判定 refer 歧义是否阻止执行（Codex/gpt-6-luna 2026-10-02）：
// 有候选 → 阻止（需用户选择）；文件操作动词 → 阻止（操作对象不明会出错）；
// 查询裸指代 → 阻止（对象悬空）；其余（查询有实体/记录类）→ 不阻止（不因指代 Ask）。
func clarificationBlocksExecution(it *contract.Intent, referOpts []refer.Option) bool {
	if len(referOpts) > 0 {
		return true
	}
	if hasFileOpVerb(it.CorrectedText) {
		return true
	}
	return isBareReferent(it.CorrectedText)
}

// llmIntentFallback（M7 ①）：规则低置信/UNKNOWN 且像自然语言问句时，调 fast provider 补分类。
//
// 【伪代码逻辑层】（新裁决逻辑）：
//
//	if o.Providers == nil: return intent（纯规则路径）。
//	trigger = (intent.Intent==UNKNOWN || intent.Confidence < 0.6) && 文本含 [?？吗呢怎么如何为什么]。
//	if !trigger: return intent。
//	调 fast.Chat(system="你是意图分类器，输出 JSON {intent,confidence}")
//		user=原始文本 + 可用域列表。
//	解析 JSON：合法且 intent∈{NOTE,QUERY,EDIT,COMMIT} → 覆盖 intent；否则保留规则结果。
//	任何 err/超时 → return intent（不阻断）。
func (o *Options) llmIntentFallback(ctx context.Context, it contract.Intent, text string) contract.Intent {
	if o == nil || o.Providers == nil {
		return it
	}
	// 评审 P4（G1）/ P3（G3）：进入"绝不执行"确认态的结果**不得**被 LLM 回退覆盖。
	// 回退命中后会把 it.Ask 清空，而「Ask != '' → 绝不执行」是安全红线：
	//   - 否定：「不要删除那个文件吗？」会被清 Ask 后判 EDIT 执行；
	//   - 元指令：「开始测试吗」同理（评审 G3-P3）；
	//   - 条件句：「如果测试通过就提交吗」同理；
	//   - 多动作：「把报价改成中文然后跑一下测试，行吗？」（评审 G5-P0-1）。
	//
	// 结构性不变式（技能 §4）：**仲裁已发生，且已落在 Ask 确认态** → 一律豁免。
	// 即 `Conflict != "" && Ask != ""`。原实现是白名单 switch
	// （negation/meta/conditional/multi_action），于是同一类缺口连踩三次：
	// 每新增一条"靠 Ask 拦住"的仲裁分支，就忘了登记到这里。白名单必然漏 ——
	// 实测就漏收了 ConflictDebugPlan（input/taskintent.go:717-721，Ask 非空，
	// 却不在白名单里，回退可以把"要给思路还是直接修"这个确认态直接清掉）。
	//
	// 为什么是 Conflict+Ask 两者，而不是只用其中任一：
	//   - 只用 `Conflict != ""`：会误伤 ConflictDelete / ConflictNoteVsDeploy /
	//     ConflictAskVsOp 这些 **Ask 为空的合法可执行路径**（删除由下游域/风险门禁管，
	//     不该在这里被挡住回退）；
	//   - 只用 `Ask != ""`：会误杀回退本身 —— ClassifyTask 初始化即带
	//     `Ask: taskAskTemplate`（"你是想让我做什么？"），UNKNOWN/低置信出口
	//     必然 Ask 非空，于是本函数永不触发（M7 ① 静默失效）。
	//     分类器把两种 Ask 混用了：**默认分类 Ask**（UNKNOWN 模板/低置信）与
	//     **仲裁 Ask**（安全停）。Conflict 非空正是"这是仲裁 Ask"的可观测标志。
	//
	// 回归保护见 pipeline/intentfallback_regression_test.go（四类双向反例）。
	if it.Conflict != "" && it.Ask != "" {
		return it
	}
	hasQ := strings.ContainsAny(text, "?？吗呢怎么如何为什么哪")
	// M7 复验补强：含问句特征时，规则未判 QUERY（UNKNOWN/低置信/误判其他意图如 NOTE）
	// 一律调 LLM 复查——规则词典对口语长问句常误判（22:04 真机："我现在测试一下…看看效果怎么样"
	// 被规则判 NOTE 高置信，若只看低置信则 fallback 永不触发）。已是 QUERY 则直接信任规则。
	if !hasQ {
		return it
	}
	if it.Intent == contract.IntentQuery {
		return it
	}
	p, err := o.Providers.Get("fast")
	if err != nil {
		return it
	}
	resp, err := p.Chat(ctx, provider.ChatRequest{
		Messages: []contract.Message{
			{Role: "system", Content: "你是 VoxSign 意图分类器。只输出 JSON：{\"intent\":\"NOTE|QUERY|EDIT|COMMIT\",\"confidence\":0.0-1.0}。意图含义：NOTE=记笔记，QUERY=问答/查询，EDIT=改文件，COMMIT=提交。"},
			{Role: "user", Content: text},
		},
		MaxTokens: 64,
	})
	if err != nil || resp.Content == "" {
		return it
	}
	// 极简 JSON 解析（零依赖）。
	var parsed struct {
		Intent     string  `json:"intent"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil {
		return it
	}
	valid := map[string]bool{
		contract.IntentNote: true, contract.IntentQuery: true,
		contract.IntentEdit: true, contract.IntentCommit: true,
	}
	if !valid[parsed.Intent] {
		return it
	}
	it.Intent = parsed.Intent
	if parsed.Confidence > 0 {
		it.Confidence = parsed.Confidence
	}
	// 覆盖成功后清掉旧 UNKNOWN 澄清残留（否则 NeedsClarification 仍触发回问；
	// refer 层若目标仍歧义会重新填 Ask）。
	it.Ask = ""
	return it
}

// noJSON 返回 false 指针：显式关闭本次调用的 response_format（QUERY 回答层要纯文本）。
func noJSON() *bool {
	v := false
	return &v
}

// queryLLMAnswer（M7 ②）：QUERY 搜索后调 fast 生成自然语言回答。
// 失败（网络/预算超限/超时）→ 返回友好降级文案（不再空壳"OK（自动执行）"）。
//
// 异常自愈接线（可选层，不改变成功路径与降级文案逐字）：
//   - 测量 fast.Chat 墙钟：慢但成功 → 不丢回答，仅带 model:"fast" 进诊断层记一笔供归因；
//   - err/空内容 → 先诊断（budget/network/param 分类），diag 可用且 action=retry/modify →
//     指数退避重试（≤2 轮），成功返回回答并回写知识库；失败或 diag 不可用 → 逐字降级文案。
func (o *Options) queryLLMAnswer(ctx context.Context, original string, searchStdout string) string {
	// ⚠️ **归因必须来自真实错误**（Lead 实测：日志里是 HTTP 401 invalid_api_key，
	// 对外却说"预算可能已用尽或网络异常" ⇒ 用户会去等明天、去查网络，而真正要做的是换 key）。
	reason := ""
	degradedNow := func() string { return degradeMsg(reason, searchStdout) }
	if o == nil || o.Providers == nil {
		return degradedNow()
	}
	p, err := o.Providers.Get("fast")
	if err != nil {
		log.Printf("[queryLLMAnswer] fast provider unavailable: %v", err)
		reason = err.Error()
		return degradedNow()
	}

	// callFast 发一次 fast 调用并解 JSON 壳；ok=false 表示失败/空/意外壳（与原逻辑一致）。
	var lastErr error
	callFast := func() (string, bool) {
		resp, err := p.Chat(ctx, provider.ChatRequest{
			Messages: []contract.Message{
				{Role: "system", Content: "你是 VoxSign 助手。根据用户问题和检索结果给简洁中文回答。只输出回答文本本身，不要输出 JSON、不要做意图分类、不要输出任何结构化格式。"},
				{Role: "user", Content: "用户问题：" + original + "\n检索结果：" + searchStdout},
			},
			MaxTokens: 400,
			// 回答层要纯文本：显式关掉 json_object（provider 级默认开启）。
			// 否则模型在"不要输出 JSON"+json_object 矛盾指令下输出无意义 JSON 壳（{"x":0}），
			// 解包失败会误判成"模型不可用"降级（M7 实测 2026-10-03）。
			ResponseFormat: noJSON(),
		})
		if err != nil {
			log.Printf("[queryLLMAnswer] fast Chat err: %v", err)
			lastErr = err // 真实错误：供归因使用（不许对外说"预算/网络"）
			return "", false
		}
		if strings.TrimSpace(resp.Content) == "" {
			log.Printf("[queryLLMAnswer] fast Chat empty content")
			return "", false
		}
		// fast 配了 json_object response_format——模型输出 JSON 壳；解出文本字段还原纯文本回答。
		content := strings.TrimSpace(resp.Content)
		if strings.HasPrefix(content, "{") {
			var j map[string]any
			if err := json.Unmarshal([]byte(content), &j); err == nil {
				for _, k := range []string{"text", "response", "content", "answer", "message", "error"} {
					if s, ok := j[k].(string); ok && strings.TrimSpace(s) != "" {
						content = strings.TrimSpace(s)
						break
					}
				}
			}
			if strings.HasPrefix(content, "{") {
				log.Printf("[queryLLMAnswer] fast returned unexpected JSON shell: %.160s", content)
				return "", false
			}
		}
		return content, true
	}

	start := time.Now()
	content, ok := callFast()
	elapsed := time.Since(start)

	if ok {
		// 慢但成功：不丢弃已有回答，只把慢响应带 model 名进诊断层（供归因/后续学习）。
		if elapsed > time.Duration(o.fastResponseMs())*time.Millisecond {
			if svc := o.selfheal(); svc != nil {
				_ = svc.Diagnose(ctx, original, contract.IntentQuery, []selfheal.Trace{
					selfheal.NewTrace("llm", "fast", map[string]any{"model": "fast"}, "slow response"),
				})
			}
		}
		return content
	}

	// err/空内容 → 先诊断；diag 可用且可重试 → 指数退避重试（≤2 轮）。
	svc := o.selfheal()
	if svc != nil {
		d := svc.Diagnose(ctx, original, contract.IntentQuery, []selfheal.Trace{
			selfheal.NewTrace("llm", "fast", map[string]any{"model": "fast"}, "fast chat 失败或空内容"),
		})
		if d != nil && (d.Action == selfheal.ActionRetry || d.Action == selfheal.ActionModify) {
			for round := 0; round < d.MaxRetries(); round++ {
				select {
				case <-ctx.Done():
				case <-time.After(d.Wait(round)):
				}
				if c2, ok2 := callFast(); ok2 {
					svc.KB.Remember(*d)
					return c2
				}
			}
		}
	}
	if lastErr != nil {
		reason = lastErr.Error()
	}
	return degradedNow()
}

// mergeAskOptions 合并意图候选与 refer 目标候选（id 去重，上限 8）。
func mergeAskOptions(base []AskOption, referOpts []refer.Option) []AskOption {
	seen := map[string]bool{}
	out := make([]AskOption, 0, len(base)+len(referOpts))
	for _, o := range base {
		if !seen[o.ID] {
			seen[o.ID] = true
			out = append(out, o)
		}
	}
	for _, o := range referOpts {
		if len(out) >= 8 {
			break
		}
		if !seen[o.ID] {
			seen[o.ID] = true
			out = append(out, AskOption{ID: o.ID, Label: o.Label})
		}
	}
	return out
}

func shortAction(it contract.Intent) string {
	obj := targetPath(it)
	if obj == "" {
		return it.Intent
	}
	return it.Intent + " " + obj
}

func targetFiles(it contract.Intent) string {
	p := targetPath(it)
	if p == "" {
		return "—"
	}
	return p
}

func undoText(it contract.Intent, rs []contract.Receipt) string {
	switch it.Intent {
	case contract.IntentCommit, contract.IntentDeploy:
		return "不可撤销（不可逆，已人工确认）"
	}
	// M4-3：用 C 交付的结构化 VHS_BACKUP_PATH: 标记解析具体备份文件。
	for _, r := range rs {
		if p := tools.ParseBackupPath(r.Stdout); p != "" {
			return "备份 " + filepath.Base(p) + "（" + filepath.Dir(p) + "）"
		}
	}
	return "—（只读/无备份）"
}

func confirmWord(level string) string {
	switch level {
	case contract.ConfirmHuman:
		return "人工放行"
	case contract.ConfirmStrong:
		return "强确认后执行"
	case contract.ConfirmLight:
		return "轻确认后执行"
	default:
		return "自动执行"
	}
}

func reasonText(r string) string {
	switch r {
	case "unknown_space":
		return "未注册域"
	case "drift":
		return "域漂移（scope 路径失效）"
	case "default_deny":
		return "权限交集为空"
	case "boundary_violation":
		return "越界（不在域声明的工具/范围内）"
	case "cross_ref_deny":
		return "跨域引用未声明"
	default:
		return r
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func strconvItoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// ---------- 轨迹/discuss 薄封装（nil-safe） ----------

func (o *Options) write(e trajectory.Entry) {
	if o.Trace != nil {
		_ = o.Trace.Write(e) // #44：轨迹写失败不阻断只读任务
	}
}

func (o *Options) attribution(rid, cls, detail, evidence, suggestion string) contract.Attribution {
	return contract.Attribution{
		RequestID: rid, Stage: "discuss", Class: cls,
		Detail: detail, Evidence: evidence, Suggestion: suggestion,
		Ts: time.Now().Format(time.RFC3339),
	}
}

func (o *Options) writeAttribution(a contract.Attribution) {
	// 用 Content 携带完整归因 JSON（trajectory.Entry 无 Attrib 字段，按 #44 用 content 落）
	b, _ := json.Marshal(a)
	o.write(trajectory.Entry{RequestID: a.RequestID, Kind: "attribution", Content: string(b)})
	o.appendDiscussLog(a)
}

func evidenceOf(rs []contract.Receipt, v verify.Result) string {
	parts := []string{}
	for _, r := range rs {
		parts = append(parts, "receipt:"+r.Tool)
	}
	if v.Status != "" {
		parts = append(parts, "verify:"+v.Status)
	}
	return strings.Join(parts, ",")
}

// appendDiscussLog 追加一条人可见结论到 <log_dir>/discuss.jsonl（下一轮注入；不自动改词典/策略）。
func (o *Options) appendDiscussLog(a contract.Attribution) {
	dir := o.logDir()
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, discussLogName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(a)
	_, _ = f.Write(append(b, '\n'))
}

// confirm 包装 ConfirmFn，ctx 取消时返回 false（不等待）。
func (o *Options) confirm(ctx context.Context, taskID, question string) bool {
	if o.ConfirmFn == nil {
		return false
	}
	type res struct{ ok bool }
	ch := make(chan res, 1)
	go func() {
		ok, err := o.ConfirmFn(taskID, question)
		if err != nil {
			ok = false
		}
		ch <- res{ok}
	}()
	select {
	case r := <-ch:
		return r.ok
	case <-ctx.Done():
		return false
	}
}

func newRequestID() string {
	return "req-" + time.Now().Format("150405.000000") + "-" + fmt.Sprintf("%x", time.Now().UnixNano()%0xffff)
}

// ---------- Summary（每日摘要） ----------

// Summary 聚合当日（since 之后）轨迹，按域/归因 class 分组，算认知闭环均值与通过率。
// #52：手机可读纯文本；数据源=轨迹 kind=task_metrics（结构化单行）。
func Summary(o *Options, since time.Time) (string, error) {
	dir := o.logDir()
	day := time.Now().Format("20060102")
	path := filepath.Join(dir, "trajectory-"+day+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "（当日暂无轨迹）", nil
		}
		return "", fmt.Errorf("读取当日轨迹失败: %w", err)
	}

	var (
		byIntent = map[string]int{}
		bySpace  = map[string]int{}
		byAttr   = map[string]int{}
		total    int
		ok       int
		sumLoop  int64
		waitN    int // M4-1：有人工等待/LLM 的任务数（Net 均值分母）
		sumWait  int64
		pureN    int // 纯管线任务数（Net 均值分母外）
	)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e trajectory.Entry
		if jerr := json.Unmarshal([]byte(line), &e); jerr != nil {
			continue
		}
		// #52 主聚合源：结构化 task_metrics 行
		if e.Kind == "task_metrics" {
			var p taskMetricsPayload
			if json.Unmarshal([]byte(e.Content), &p) != nil {
				continue
			}
			total++
			byIntent[p.Intent]++
			if p.Space != "" {
				bySpace[p.Space]++
			}
			if p.AttrCls != "" {
				byAttr[p.AttrCls]++
			}
			sumLoop += p.LoopMs
			if p.HadWait {
				waitN++
				sumWait += p.NetMs
			} else {
				pureN++
			}
			if p.OK {
				ok++
			}
			continue
		}
		// 兼容旧轨迹：无 task_metrics 时仍数 intent 行（M2 历史）
		if e.Intent != nil && e.Kind == trajectory.KindIntent {
			total++
			byIntent[e.Intent.Intent]++
			if e.Intent.Space != "" {
				bySpace[e.Intent.Space]++
			}
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "VoxSign 当日摘要（%s）\n", day)
	if total == 0 {
		sb.WriteString("（当日暂无任务）\n")
		return sb.String(), nil
	}
	fmt.Fprintf(&sb, "任务数：%d\n", total)
	passRate := 100.0
	if total > 0 {
		passRate = float64(ok) / float64(total) * 100
	}
	avgLoop := sumLoop / int64(total)
	fmt.Fprintf(&sb, "通过率：%.0f%%（%d/%d）\n", passRate, ok, total)
	fmt.Fprintf(&sb, "认知闭环：Loop 均值 %dms", avgLoop)
	if waitN > 0 {
		fmt.Fprintf(&sb, " / Net 均值 %dms（含等待/LLM 任务 %d 个）", sumWait/int64(waitN), waitN)
	}
	fmt.Fprintf(&sb, "\n")
	if pureN > 0 {
		fmt.Fprintf(&sb, "纯管线任务（无等待/LLM）%d 个，不计入 Net 均值\n", pureN)
	}
	writeCounts(&sb, "按意图", byIntent)
	writeCounts(&sb, "按域", bySpace)
	writeCounts(&sb, "按归因", byAttr)
	return sb.String(), nil
}

func writeCounts(sb *strings.Builder, title string, m map[string]int) {
	if len(m) == 0 {
		return
	}
	type kv struct {
		k string
		v int
	}
	var items []kv
	for k, v := range m {
		items = append(items, kv{k, v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].v != items[j].v {
			return items[i].v > items[j].v
		}
		return items[i].k < items[j].k
	})
	fmt.Fprintf(sb, "%s：\n", title)
	for _, it := range items {
		fmt.Fprintf(sb, "  %s ×%d\n", it.k, it.v)
	}
}

// degradeMsg 由**真实错误**生成降级文案（归因必须准确；未知才说未知）。
//
// v2.3（2026-10-04 用户指令："语气词很重要，别像机器人"）：模型不可用时
// 不再输出"（上游限流（429）…）"机器人括号，改本地人话模板——承认问题、
// 给可行动建议、带语气词。归因仍来自 attributeLLMError（真实错误，不许瞎说）。
func degradeMsg(rawErr, searchStdout string) string {
	reason := attributeLLMError(rawErr)
	action := "我这边调整一下就能接着干"
	switch {
	case strings.Contains(reason, "鉴权"):
		action = "需要检查一下后台的 API key 配置"
	case strings.Contains(reason, "限流"), strings.Contains(reason, "额度/预算"):
		action = "等额度恢复（一般明天就好），你也可以换一条模型通道"
	case strings.Contains(reason, "超时"):
		action = "我刚才是超时了，你再说一遍我马上重试"
	case strings.Contains(reason, "原因未知"):
		action = "我去查一下后台日志再告诉你"
	}
	prefix := "哎呀，这条我一时没答上来——" + reason + "。" + action + "。"
	if s := strings.TrimSpace(searchStdout); s != "" {
		return prefix + "\n不过检索到一点线索：\n" + s
	}
	return prefix
}

// attributeLLMError 把真实错误归因成人能行动的一句话。
//
// 判据要求（Lead 2026-10-03）：
//
//	401/403 ⇒ **鉴权/API key**（不得出现"预算/网络"）；429 ⇒ 限流；5xx ⇒ 上游；
//	超时 ⇒ 超时；**未知 ⇒ 未知 + 原始错误文本**（不许套用通用话术）。
func attributeLLMError(rawErr string) string {
	e := strings.ToLower(rawErr)
	switch {
	case strings.Contains(e, "401"), strings.Contains(e, "403"),
		strings.Contains(e, "invalid_api_key"), strings.Contains(e, "auth_error"),
		strings.Contains(e, "unauthorized"), strings.Contains(e, "api key"):
		return "鉴权失败（API key 无效/未配置）—— 请更换或配置 key，重试前无需等待"
	case strings.Contains(e, "429"), strings.Contains(e, "rate limit"), strings.Contains(e, "too many requests"):
		return "上游限流（429）—— 稍后重试"
	case strings.Contains(e, "500"), strings.Contains(e, "502"), strings.Contains(e, "503"), strings.Contains(e, "504"):
		return "上游服务错误（HTTP 5xx）—— 非本机问题，稍后重试"
	case strings.Contains(e, "timeout"), strings.Contains(e, "deadline"), strings.Contains(e, "超时"):
		return "调用超时"
	case strings.Contains(e, "budget"), strings.Contains(e, "预算"), strings.Contains(e, "quota"):
		return "额度/预算用尽"
	case strings.TrimSpace(rawErr) == "":
		return "模型服务暂不可用（原因未知：未取得错误信息）"
	default:
		return "模型调用失败（原因未知）：" + rawErr
	}
}

// execRegisterTool —— 2026-10-04 用户强要求"后台必须有能力扩展能力，自迭代自更新"：
// REGISTER_TOOL 意图执行：抽取能力名 → 生成工具契约 → Registry.Register 落盘 →
// 回复人话确认（"好，我来增加「XX」能力"）。能力名抽不到/注册表未绑定 → 明确回执，不静默失败。
func (o *Options) execRegisterTool(ctx context.Context, it contract.Intent, logDir string) []contract.Receipt {
	name := extractCapabilityName(it.CorrectedText)
	if name == "" {
		return []contract.Receipt{{
			Tool: "register", OK: false, Err: "没听清要增加什么能力。请说清楚，比如「增加一个远程控制电脑的能力」",
		}}
	}
	if o.Tools == nil {
		return []contract.Receipt{{
			Tool: "register", OK: false, Err: "工具注册表未配置，无法落盘",
		}}
	}
	c := contract.ToolContract{
		Name:          name,
		Version:       "1.0",
		Source:        "voice",
		Caps:          []string{name},
		Params:        map[string]string{"args": "string,optional"},
		SideEffects:   []string{"语音注册（自举）"},
		AllowedSpaces: []string{"project", "sandbox"},
		Risk:          map[string]string{name: "medium"},
		RegisteredAt:  time.Now().Format(time.RFC3339),
	}
	if err := o.Tools.Register(c, true); err != nil {
		return []contract.Receipt{{Tool: "register", OK: false, Err: "注册失败: " + err.Error()}}
	}
	// 能力契约已落盘（自举第一步）；执行器实现在后续迭代接入。
	return []contract.Receipt{{
		Tool: "register", OK: true,
		Stdout: "好，我来增加「" + name + "」能力：已登记为可扩展工具（语音自举注册）。" +
			"接下来我会把它接成可执行能力——你说「" + name + "」相关的具体需求，我就能真正上手。",
	}}
}

// extractCapabilityName 从注册请求中抽取能力名：
// 「你必须增加一个 远程控制电脑 的能力」→ 远程控制电脑；「加一个 压缩图片 的工具」→ 压缩图片。
func extractCapabilityName(text string) string {
	re := regexp.MustCompile(`(?:增加|加|注册|新增|添加|创建|新建|搞|做|接入|上架)(?:一个|个|个新的|新的|一种|一项)?(?:工具|能力|功能|技能|插件|小工具)?(?:的)?([\p{Han}A-Za-z0-9\-_ ]+?)(?:的能力|的功能|的工具|的技能|的插件|吧|呢|了|。|？|\?|，|,|$)`)
	m := re.FindStringSubmatch(text)
	if len(m) > 1 {
		name := strings.TrimSpace(m[1])
		name = strings.TrimRight(name, "的了")
		if name != "" && len([]rune(name)) <= 20 {
			return name
		}
	}
	// 兜底：去尾虚词后取整句（≤20 字，防把整段抱怨当能力名）。
	t := strings.TrimSpace(text)
	t = strings.TrimRight(t, "的了吧呢。？?!！，, ")
	if t != "" && len([]rune(t)) <= 20 {
		return t
	}
	return ""
}
