// Package route —— 快慢模型分层路由（VHS-FASTSLOW-001）。
//
//	L0    本地（热词/别名/关联度）—— **命中即返回，不问任何模型**
//	L0.5  JEV /api/decide        —— 快判断；ambiguous ⇒ **回问用户，不升 L1**
//	L1    deepseek-flash         —— 只有需要"长篇推理"才到（本层未实现在此包）
//	L2    reasoner
//
// 三条硬要求：① ambiguous→回问不升 L1；② 非 200/超时一律当 JEV 不可用 ⇒ 降级 L0 + degraded；
// ③ 判断入工作记忆标 judged_by=jev，不与事实混放。
package route

import (
	"context"
	"strings"
	"time"

	"voicesign-harness/hotcache"
	"voicesign-harness/plan"
)

// Level 是路由层级。
type Level string

const (
	LevelL0  Level = "L0"
	LevelL05 Level = "L0.5"
	LevelL1  Level = "L1"
	LevelL2  Level = "L2"
)

// Action 是本次路由的动作。
const (
	ActionAnswer   = "answer"   // 有把握，直接答
	ActionAskUser  = "ask_user" // 回问用户（不猜、不升 L1）
	ActionEscalate = "escalate" // 需要长篇推理 → 升 L1/L2
)

// Kind 是 JEV 的判断场景。**具名类型 + 常量**：`"route"` 这类字面量在**编译期就写不出来**
// （规范枚举：referent | permission | learnability | gap_class | custom）。
//
// 机制性防复发：本轮实测踩过两次"猜 API"（kind="route"→400；payload 形状→502）——
// 教训写进文档不防复发，**类型上写不出来才防复发**。
type Kind string

const (
	KindReferent     Kind = "referent"     // 指代消解（那个/它 指谁）
	KindPermission   Kind = "permission"   // 授权判断（能不能做）
	KindLearnability Kind = "learnability" // 可学性判断（要不要记住）
	KindGapClass     Kind = "gap_class"    // 缺口归类（找谁）
	KindCustom       Kind = "custom"       // 兜底
)

// Candidate 是送给 JEV 的候选（**JEV 只认 candidates**，constraints 放 why）。
type Candidate struct {
	ID  string `json:"id"`
	Why string `json:"why,omitempty"`
}

// JEVResponse 是 /api/decide 的响应。
type JEVResponse struct {
	Choice     string   `json:"choice"`
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
	Evidence   []string `json:"evidence"`
	ModelID    string   `json:"model_id"`
}

// JEV 是快判断接口（留桩：实现见 JEVClient；测试用假实现）。
type JEV interface {
	Decide(ctx context.Context, req JEVRequest) (JEVResponse, error)
}

// LedgerEntry 是升级/降级台账（全程留痕）。
type LedgerEntry struct {
	Level     Level  `json:"level"`
	ModelID   string `json:"model_id,omitempty"`
	Reason    string `json:"reason"`
	Escalated bool   `json:"escalated"`
}

// Decision 是一次路由决策。
type Decision struct {
	Level          Level
	Action         string
	Choice         string
	Confidence     float64
	ModelID        string
	Reason         string
	Degraded       bool
	DegradedReason string
	Ledger         []LedgerEntry
}

// kindWords 是**场景分派**规则（不猜：按目标/问句里的信号词选 kind）。
var kindWords = []struct {
	kind  Kind
	words []string
}{
	{KindPermission, []string{"能不能", "可以吗", "允许", "权限", "授权", "vault", "不可逆", "删除", "部署"}},
	{KindReferent, []string{"那个", "这个模块", "它", "指哪", "指的是", "哪个"}},
	{KindLearnability, []string{"记住", "学到", "教它", "以后都", "下次也"}},
	{KindGapClass, []string{"找谁", "谁负责", "缺什么", "谁来"}},
}

// KindFor 按场景分派 kind（默认 KindCustom）。
func KindFor(text string) Kind {
	t := strings.ToLower(text)
	for _, r := range kindWords {
		for _, w := range r.words {
			if strings.Contains(t, strings.ToLower(w)) {
				return r.kind
			}
		}
	}
	return KindCustom
}

// SituationFromMemory 把工作记忆的**四块板**桥接成 JEV 要的**结构化态势**（J2：紧凑，不是长文本）。
func SituationFromMemory(w *plan.WorkingMemory) Situation {
	if w == nil {
		return Situation{}
	}
	sit := Situation{}
	for _, it := range w.BoundedWorkingSet() {
		why := it.Source
		if it.JudgedBy != "" {
			why += "（判断）"
		}
		sit.Candidates = append(sit.Candidates, Candidate{ID: it.Element, Why: why})
	}
	for _, c := range w.Constraints {
		sit.Constraints = append(sit.Constraints, c.Element)
	}
	for _, it := range w.Situation {
		sit.Memory = append(sit.Memory, it.Element)
	}
	for _, it := range w.OpenItems {
		sit.Memory = append(sit.Memory, "待决:"+it.Element)
	}
	return sit
}

// L1Model 是慢通道（deepseek-flash）。
type L1Model interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Router 串起 L0（本地 → 网关路由）→ L0.5（JEV）→ L1（唯一升级目标）。
type Router struct {
	Hot *hotcache.Cache
	// ServiceRouter 是 L0 **第二梯队**（网关 /api/route）：本地未命中时才问，命中即返回。
	ServiceRouter ServiceRoute
	JEV           JEV
	// L1 是唯一升级目标（deepseek-flash）。**只在"需要多步推理"时升级**，不为"想更准"升级。
	L1 L1Model
	// NeedsReasoning 由调用方声明"这是多步推理任务"（唯一升级触发器之一）。
	NeedsReasoning bool
	Threshold      float64
	Timeout        time.Duration
	// Kind 为空时按 KindFor(question+text) 自动分派。
	Kind Kind
}

func (r *Router) threshold() float64 {
	if r.Threshold <= 0 {
		return 0.70
	}
	return r.Threshold
}

// Route 执行分层路由；question/situation 由调用方给（situation 由工作记忆四块板渲染）。
// options 自动 = 各候选 id + "ambiguous"（J1/J3：允许"说不清"，且不发明答案空间）。
func (r *Router) Route(ctx context.Context, text, question string, sit Situation) Decision {
	var ledgerNote []LedgerEntry
	options := []string{"ambiguous"}
	for _, c := range sit.Candidates {
		options = append(options, c.ID)
	}
	// ---- L0：本地命中即返回，绝不问模型 ----
	if r.Hot != nil {
		if res, ok := r.Hot.Lookup(text); ok && res.Score >= r.threshold() {
			return Decision{
				Level: LevelL0, Action: ActionAnswer, Choice: res.Canonical,
				Confidence: res.Score, Reason: "L0 本地命中（" + res.Route + "），不问任何模型",
				Ledger: []LedgerEntry{{Level: LevelL0, Reason: "local-hit:" + res.Route, Escalated: false}},
			}
		}
	}
	// ---- L0 第二梯队：网关 /api/route（不调模型）----
	if r.ServiceRouter != nil {
		name, ok, err := r.ServiceRouter.Lookup(ctx, text)
		switch {
		case err != nil:
			// fail-open 但**留痕**：不可用 ≠ 没有
			ledgerNote = append(ledgerNote, LedgerEntry{Level: LevelL0, Reason: "route-unavailable:" + err.Error(), Escalated: false})
		case ok && name != "":
			return Decision{
				Level: LevelL0, Action: ActionAnswer, Choice: name, Confidence: 0.9,
				Reason: "L0 第二梯队：网关 /api/route 唯一命中（靠 aliases，抗 ASR 变形，不调模型）",
				Ledger: append(ledgerNote, LedgerEntry{Level: LevelL0, Reason: "gateway-route", Escalated: false}),
			}
		default:
			// 落空必须可见：多命中/未命中不得静默落到 L0.5（否则永远不知道命中率）。
			ledgerNote = append(ledgerNote, LedgerEntry{Level: LevelL0, Reason: "route_ambiguous=true", Escalated: false})
		}
	}
	// ---- 升级（唯一触发器：调用方声明"需要多步推理"）----
	if r.NeedsReasoning {
		if r.L1 == nil {
			return Decision{Level: LevelL05, Action: ActionAskUser, Degraded: true,
				DegradedReason: "需要多步推理但 L1 未配置",
				Reason:         "无可用的慢通道，回问用户（不假装答得了）",
				Ledger:         append(ledgerNote, LedgerEntry{Level: LevelL1, Reason: "l1-unconfigured", Escalated: false})}
		}
		t := r.Timeout
		if t <= 0 {
			t = 5 * time.Second
		}
		cctx, cancel := context.WithTimeout(ctx, t)
		defer cancel()
		out, err := r.L1.Complete(cctx, text)
		if err != nil {
			return Decision{Level: LevelL0, Action: ActionAskUser, Degraded: true,
				DegradedReason: "L1 调用失败：" + err.Error(),
				Reason:         "慢通道失败，降级并回问",
				Ledger:         append(ledgerNote, LedgerEntry{Level: LevelL1, Reason: "l1-failed", Escalated: true})}
		}
		return Decision{Level: LevelL1, Action: ActionAnswer, Choice: out,
			Reason: "需要多步推理 ⇒ 升级 L1（deepseek-flash）",
			Ledger: append(ledgerNote, LedgerEntry{Level: LevelL1, Reason: "needs-reasoning", Escalated: true})}
	}
	// ---- L0.5：JEV ----
	if r.JEV == nil {
		return Decision{
			Level: LevelL0, Action: ActionAskUser,
			Reason:   "无 JEV 可用且本地未命中：回问用户（不猜）",
			Degraded: true, DegradedReason: "JEV 未配置",
			Ledger: append(ledgerNote, LedgerEntry{Level: LevelL0, Reason: "no-jev", Escalated: false}),
		}
	}
	if r.Timeout <= 0 {
		r.Timeout = 3 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	kind := r.Kind
	if kind == "" {
		kind = KindFor(question + " " + text) // ① 按场景分派（不再恒为 custom）
	}
	if kind == KindCustom {
		// ① 落空可见：分派失败必须留痕（否则"分派不准"会静默成"泛泛地问 JEV"）
		ledgerNote = append(ledgerNote, LedgerEntry{Level: LevelL05, Reason: "kind_fallback=true", Escalated: false})
	}
	resp, err := r.JEV.Decide(cctx, JEVRequest{Kind: kind, Question: question, Situation: sit, Options: options})
	if err != nil {
		// 硬要求②：**非 200/超时一律当 JEV 不可用 ⇒ 降级 L0 + degraded**（不假设它总对）
		return Decision{
			Level: LevelL0, Action: ActionAskUser,
			Reason:   "JEV 不可用，降级 L0 并回问（fail-open）",
			Degraded: true, DegradedReason: "JEV 调用失败：" + err.Error(),
			Ledger: append(ledgerNote, LedgerEntry{Level: LevelL0, ModelID: "jev", Reason: "jev-unavailable", Escalated: false}),
		}
	}
	if resp.Choice == "ambiguous" || resp.Choice == "" {
		// 硬要求①：ambiguous ⇒ **回问用户，不升 L1**（JEV 说不清，问大模型也是猜）
		return Decision{
			Level: LevelL05, Action: ActionAskUser,
			Confidence: resp.Confidence, ModelID: resp.ModelID,
			Reason: "JEV 判为 ambiguous（" + resp.Reason + "）⇒ 回问用户，不升级 L1",
			Ledger: append(ledgerNote, LedgerEntry{Level: LevelL05, ModelID: resp.ModelID, Reason: "jev-ambiguous", Escalated: false}),
		}
	}
	return Decision{
		Level: LevelL05, Action: ActionAnswer, Choice: resp.Choice,
		Confidence: resp.Confidence, ModelID: resp.ModelID, Reason: resp.Reason,
		Ledger: append(ledgerNote, LedgerEntry{Level: LevelL05, ModelID: resp.ModelID, Reason: "jev-decided", Escalated: true}),
	}
}

// RememberJudgement 把 JEV 判断写入工作记忆（硬要求③：标 judged_by，**不得与事实混放**）。
func RememberJudgement(w *plan.WorkingMemory, d Decision) {
	if w == nil || d.Level != LevelL05 || d.Action != ActionAnswer {
		return
	}
	w.Remember("working_set", plan.BoardItem{
		Element: "判断:" + d.Choice, Source: "jev:" + d.ModelID,
		Inferred: false, // 不是推断，是**外部判断**——用 JudgedBy 区分，而不是推断位
		JudgedBy: "jev",
	})
}
