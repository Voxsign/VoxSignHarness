// hooks.go —— 知己 · harness 主循环可选挂载适配层（M1 P0）。
//
// 设计目标（硬约束）：
//  1. 挂载是「可选」的：pipeline 侧不 NewZhiji / 不挂 Hooks 时，pipeline 原行为逐位不变。
//     本文件所有钩子都做了双 nil 防护——HarnessHooks 指针为 nil、或内部 *Zhiji 为 nil，
//     调用均为 no-op。pipeline 可无脑写 `o.Zhiji.OnInput(...)`，无需先判空。
//  2. zhiji 包不得 import pipeline（会循环依赖：pipeline → zhiji → pipeline）。
//     因此本文件用「值视图 OutcomeView + 归一化函数」做解耦：pipeline 侧用
//     `OutcomeView{...}` 从 Outcome 字段手工映射出一个值即可，不需要任何接口实现。
//
// ─────────────────────────── 挂点清单（pipeline/pipeline.go 实测行号） ───────────────────────────
// 以下行号基于 Run()（第 182 行起）当前版本；改代码后请重新 Grep 锚点核对。
//
// A1 OnInput —— 输入进入（raw 轨迹写后立即挂）
//   锚点：第 221 行 `emit(trajectory.Entry{Kind: trajectory.KindInputRaw, Content: text})`
//   在其后插入 1 行：
//       o.Zhiji.OnInput(corrected, 3)   // corrected=清洗后文本；importance 默认 3
//   注：空文本早退（203–210 行）不挂 OnInput（无有效输入）。
//
// A6 OnDecide —— 影子路由（只记录推荐，不改线上模型选择）
//   锚点：第 329 行 `out.Decision = decision`（risk 裁决算出、进入执行前）。
//   在其后插入 3–5 行：
//       p := zhiji.TaskProfile{Complexity: estimateComplexity(intent), Domain: intent.Space}
//       if rd, err := o.Zhiji.OnDecide(p); err == nil {
//           _ = rd // 影子模式：只落 shadow_log，线上仍走原模型（M1）；M2 再用 rd.ModelID
//       }
//   注：NewZhiji 后须 `z.Router.SetShadow(true)` 才会写 shadow_log。
//
// A3 OnTaskEnd —— 任务结束全量轨迹 + 反馈（产物落盘后、return 前挂）
//   锚点：第 459 行 `return out, nil`（主完成出口）。
//   在其前插入 2 行：
//       o.Zhiji.OnTaskEndView(ctx, OutcomeViewFromOutcome(out))
//       return out, nil
//   早退出口（209 空指令 / 289 待澄清 / 322 拦截 / 394 执行失败）M1 不强制挂；
//   若要全覆盖，可在每个 `return out, nil` 前同样调一次 OnTaskEndView（View 按
//   confirmed/Blocked/Receipts 如实填）。
//
// A7 Compress —— 长上下文压缩（执行前、上下文超阈值时挂）
//   锚点：当前 Run() 尚无显式长上下文分支；挂点同 A6，即第 329 行 decision 之后、执行之前。
//   在其后插入判定（阈值与 Compressor.MaxSummaryTokens*4 对齐，字符粗估）：
//       if ctxTokens := estimateTokens(intent.Context) + historyTokens(); ctxTokens > 16000 {
//           if cc, err := o.Zhiji.Compress(ctx, zhiji.CompressInput{Full: full, Segment: tail}); err == nil {
//               intent.Context = cc.Summary // 仅用压缩后的主体；goals/rules/pending 由压缩器原样保留
//           }
//       }
//
// A2 Start / Stop —— 反思线程生命周期（不在 Run() 内，server 启停时各挂一次）
//   锚点：server 启动（如 server.New / ListenAndServe 之前）挂：`zh.Start(ctx)`；
//   优雅退出（shutdown hook）挂：`zh.Stop(); zh.Close()`。
//   对应 pipeline 侧挂载 one-liner（server 启动时）：
//       o.Zhiji = zhiji.NewHarnessHooks(zh)   // zh==nil 时得到全 no-op hooks
//
// ─────────────────────────── 挂载函数（AttachZhiji 等价物） ───────────────────────────
// 真正的 `AttachZhiji(o *pipeline.Options, z *zhiji.Zhiji)` 写在 pipeline 包内（一行赋值），
// 因为 zhiji 不能 import pipeline。本文件只提供 NewHarnessHooks(z)。
package zhiji

import (
	"context"
	"errors"
)

// HarnessHooks 是 zhiji 对 pipeline 主循环的可选挂载适配层（双 nil-safe）。
// 生命周期：server 启动时 NewHarnessHooks(z) 构造一次，挂到 pipeline.Options 的可选字段；
// 请求级 Run() 调用只读访问，不做修改（反思线程写日志走独立 goroutine，并发安全）。
type HarnessHooks struct {
	z *Zhiji
}

// NewHarnessHooks 挂载一个 zhiji 实例。传 nil 得到全 no-op 的 hooks（等价于未挂载，
// pipeline 零行为变化）；即使返回值再被赋成 nil 指针，所有方法也都已做 h==nil 防护。
func NewHarnessHooks(z *Zhiji) *HarnessHooks {
	return &HarnessHooks{z: z}
}

// OnInput 输入进入钩子（no-op 安全）。summary=清洗后输入；importance 默认 3。
func (h *HarnessHooks) OnInput(summary string, importance float64) {
	if h == nil || h.z == nil {
		return
	}
	h.z.OnInput(summary, importance)
}

// BeforeDecision 决策前基线注入钩子（no-op 安全：未挂载返回 nil, nil，pipeline 跳过注入）。
func (h *HarnessHooks) BeforeDecision(ctx context.Context, query string) (*Baseline, error) {
	if h == nil || h.z == nil {
		return nil, nil
	}
	return h.z.BeforeDecision(ctx, query)
}

// OnTaskEnd 任务结束轨迹钩子（no-op 安全）。调用方应优先用 OnTaskEndView（自动组 CallLog+反馈）。
func (h *HarnessHooks) OnTaskEnd(ctx context.Context, log CallLog) error {
	if h == nil || h.z == nil {
		return nil
	}
	return h.z.OnTaskEnd(ctx, log)
}

// OnDecide 影子路由钩子（no-op 安全：未挂载返回零值决策，shadow=false，不影响线上）。
func (h *HarnessHooks) OnDecide(p TaskProfile) (RouteDecision, error) {
	if h == nil || h.z == nil {
		return RouteDecision{}, nil
	}
	return h.z.OnDecide(p)
}

// Compress 长上下文压缩钩子（no-op 安全：未挂载返回零值，pipeline 用原上下文）。
func (h *HarnessHooks) Compress(ctx context.Context, in CompressInput) (CompressedContext, error) {
	if h == nil || h.z == nil {
		return CompressedContext{}, nil
	}
	return h.z.Compress(ctx, in)
}

// Start 启动反思线程（no-op 安全）。server 启动时调用一次。
func (h *HarnessHooks) Start(ctx context.Context) {
	if h == nil || h.z == nil {
		return
	}
	h.z.Start(ctx)
}

// Stop 停止反思线程（no-op 安全）。server 优雅退出时调用。
func (h *HarnessHooks) Stop() {
	if h == nil || h.z == nil {
		return
	}
	h.z.Stop()
}

// ─────────────────────────── Outcome → zhiji 信号适配（值视图解耦） ───────────────────────────

// OutcomeView 是 pipeline.Outcome 的最小视图（值类型，避免 import pipeline 循环依赖）。
// pipeline 侧由 Outcome 字段手工映射构造；语义对齐注释列在字段后。
type OutcomeView struct {
	RequestID    string  // out.RequestID（日志/反馈 taskID）
	TaskProfile  string  // 任务画像标识（必填；OnTaskEnd 要求非空）。建议填 intent.Intent+Space
	Model        string  // 实际执行模型（归因用）
	Cost         float64 // token 相对成本
	Confirmed    bool    // out.Confirmed（人工确认通过）
	HadFailure   bool    // out.Receipts 中存在 OK=false（≈ pipeline.hasFailure）
	Blocked      bool    // out.Receipts 中存在 Blocked != ""（安全拦截=revoked）
	VerifyFail   bool    // out.Verify.Status == verify.StatusFail
	UserOutcome  string  // 用户显式反馈："success"|"failed"|""（最高优先级信号源）
}

// FeedbackSignal 是归一化后的反馈信号（含来源分级与可信度）。
type FeedbackSignal struct {
	Outcome    string  // success|failed|revoked|judge
	Source     string  // user_feedback|tool_error|confirm_flag|llm_judge
	Confidence float64 // 0–1（来源越硬，可信度越高）
	Reason     string  // 一句话归因（可审计）
}

// ClassifyFeedback 按信号源优先级把一次任务产物归一成反馈信号。
// 优先级：用户反馈 > 工具报错(Receipts OK=false) > 完成标志(Confirmed/正常 return) > LLM-judge。
// revoked（安全拦截 Blocked）作为工具报错的特例单独识别，优先于普通 failed。
func ClassifyFeedback(v OutcomeView) FeedbackSignal {
	// 1. 用户显式反馈（最高优先级，可信度最高）
	if v.UserOutcome == "success" || v.UserOutcome == "failed" {
		return FeedbackSignal{
			Outcome:    v.UserOutcome,
			Source:     "user_feedback",
			Confidence: 0.95,
			Reason:     "用户显式反馈",
		}
	}
	// 2. 安全拦截 = revoked（工具报错的特例，单独识别）
	if v.Blocked {
		return FeedbackSignal{
			Outcome:    "revoked",
			Source:     "tool_error",
			Confidence: 0.9,
			Reason:     "安全拦截（Receipt.Blocked 非空）",
		}
	}
	// 3. 工具报错（回执失败或 verify 未过）
	if v.HadFailure || v.VerifyFail {
		return FeedbackSignal{
			Outcome:    "failed",
			Source:     "tool_error",
			Confidence: 0.8,
			Reason:     "工具回执失败或 verify 未过",
		}
	}
	// 4. 完成标志（人工确认通过 / 正常 return）
	if v.Confirmed {
		return FeedbackSignal{
			Outcome:    "success",
			Source:     "confirm_flag",
			Confidence: 0.6,
			Reason:     "人工确认通过",
		}
	}
	// 5. 兜底：无显式信号，交 LLM-judge
	return FeedbackSignal{
		Outcome:    "judge",
		Source:     "llm_judge",
		Confidence: 0.4,
		Reason:     "无显式信号，交 LLM-judge",
	}
}

// BuildCallLog 把 OutcomeView 组装成一条 zhiji CallLog（OnTaskEnd 入参）。
func BuildCallLog(v OutcomeView) CallLog {
	sig := ClassifyFeedback(v)
	return CallLog{
		ID:          v.RequestID,
		TaskProfile: v.TaskProfile,
		Model:       v.Model,
		Outcome:     sig.Outcome,
		Cost:        v.Cost,
	}
}

// OnTaskEndView 是 OnTaskEnd 的适配层（no-op 安全）：
// 先 BuildCallLog 全量轨迹回写，再按 ClassifyFeedback 的来源分级写一条反馈信号。
// 反馈写入失败不阻断主链路（轨迹已落盘，反馈是自举训练集补充）。
func (h *HarnessHooks) OnTaskEndView(ctx context.Context, v OutcomeView) error {
	if h == nil || h.z == nil {
		return nil
	}
	log := BuildCallLog(v)
	if log.TaskProfile == "" {
		return errors.New("zhiji: OnTaskEndView 需要 task_profile")
	}
	if err := h.z.OnTaskEnd(ctx, log); err != nil {
		return err
	}
	sig := ClassifyFeedback(v)
	if h.z.Contract != nil {
		// 反馈信号分级写入（taskID=RequestID，便于与轨迹 join）。错误吞掉不阻断。
		_ = h.z.Contract.LogFeedback(ctx, v.RequestID, sig.Outcome, sig.Confidence)
	}
	return nil
}
