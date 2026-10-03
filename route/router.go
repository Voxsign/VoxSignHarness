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

// Router 串起 L0 → L0.5。Threshold 是"关联度/置信度够用"的门槛。
type Router struct {
	Hot       *hotcache.Cache
	JEV       JEV
	Threshold float64
	Timeout   time.Duration
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
	// ---- L0.5：JEV ----
	if r.JEV == nil {
		return Decision{
			Level: LevelL0, Action: ActionAskUser,
			Reason:   "无 JEV 可用且本地未命中：回问用户（不猜）",
			Degraded: true, DegradedReason: "JEV 未配置",
			Ledger: []LedgerEntry{{Level: LevelL0, Reason: "no-jev", Escalated: false}},
		}
	}
	if r.Timeout <= 0 {
		r.Timeout = 3 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	resp, err := r.JEV.Decide(cctx, JEVRequest{Kind: "custom", Question: question, Situation: sit, Options: options})
	if err != nil {
		// 硬要求②：**非 200/超时一律当 JEV 不可用 ⇒ 降级 L0 + degraded**（不假设它总对）
		return Decision{
			Level: LevelL0, Action: ActionAskUser,
			Reason:   "JEV 不可用，降级 L0 并回问（fail-open）",
			Degraded: true, DegradedReason: "JEV 调用失败：" + err.Error(),
			Ledger: []LedgerEntry{{Level: LevelL0, ModelID: "jev", Reason: "jev-unavailable", Escalated: false}},
		}
	}
	if resp.Choice == "ambiguous" || resp.Choice == "" {
		// 硬要求①：ambiguous ⇒ **回问用户，不升 L1**（JEV 说不清，问大模型也是猜）
		return Decision{
			Level: LevelL05, Action: ActionAskUser,
			Confidence: resp.Confidence, ModelID: resp.ModelID,
			Reason: "JEV 判为 ambiguous（" + resp.Reason + "）⇒ 回问用户，不升级 L1",
			Ledger: []LedgerEntry{{Level: LevelL05, ModelID: resp.ModelID, Reason: "jev-ambiguous", Escalated: false}},
		}
	}
	return Decision{
		Level: LevelL05, Action: ActionAnswer, Choice: resp.Choice,
		Confidence: resp.Confidence, ModelID: resp.ModelID, Reason: resp.Reason,
		Ledger: []LedgerEntry{{Level: LevelL05, ModelID: resp.ModelID, Reason: "jev-decided", Escalated: true}},
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
