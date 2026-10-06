// Package route -- fastslow typesplit routeby(VHS-FASTSLOW-001). 
//
//	L0    basely( word/diffname/close  )-- ** ini.e.returnback,      type**
//	L0.5  JEV /api/decide        -- fast disconnect; ambiguous ⇒ **clarificationuseuser,    L1**
//	L1    deepseek-flash         -- onlyhasneedneed"    "onlyto(this layer  now   )
//	L2    reasoner
//
//    needrequire: ① ambiguous->clarification   L1; ②   200/ time  cur JEV   use ⇒    L0 + degraded; 
// ③  disconnectin    tgt judged_by=jev,  and    . 
package route

import (
	"context"
	"strings"
	"time"

	"voicesign-harness/hotcache"
	"voicesign-harness/plan"
)

// Level isrouteby  . 
type Level string

const (
	LevelL0  Level = "L0"
	LevelL05 Level = "L0.5"
	LevelL1  Level = "L1"
	LevelL2  Level = "L2"
)

// Action isbase routeby   . 
const (
	ActionAnswer   = "answer"   // haspipe ,  connect 
	ActionAskUser  = "ask_user" // clarificationuseuser(  ,    L1)
	ActionEscalate = "escalate" // needneed     ->   L1/L2
)

// Kind is JEV   disconnect scenario. ** nameclasstype +   **: `"route"`  classcharface  **  periodthenwrite out **
// (rule   : referent | permission | learnability | gap_class | custom). 
//
//  restrictityprevent send: base    ed  "  API"(kind="route"->400; payload  status->502)--
//   write    prevent send, **classtypeonwrite out onlyprevent send**. 
type Kind string

const (
	KindReferent     Kind = "referent"     // coreference resolution(  /  refer )
	KindPermission   Kind = "permission"   //    disconnect(    )
	KindLearnability Kind = "learnability" //   ity disconnect(need need  )
	KindGapClass     Kind = "gap_class"    //    class(  )
	KindCustom       Kind = "custom"       //  bot
)

// Candidate is give JEV    (**JEV only  candidates**, constraints   why). 
type Candidate struct {
	ID  string `json:"id"`
	Why string `json:"why,omitempty"`
}

// JEVResponse is /api/decide    . 
type JEVResponse struct {
	Choice     string   `json:"choice"`
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
	Evidence   []string `json:"evidence"`
	ModelID    string   `json:"model_id"`
}

// JEV isfast disconnectconnect (  :  nowsee JEVClient;   use  now). 
type JEV interface {
	Decide(ctx context.Context, req JEVRequest) (JEVResponse, error)
}

// LedgerEntry is  /    (safety   ). 
type LedgerEntry struct {
	Level     Level  `json:"level"`
	ModelID   string `json:"model_id,omitempty"`
	Reason    string `json:"reason"`
	Escalated bool   `json:"escalated"`
}

// Decision is  routebydecide . 
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
	// WMCAP   (by    side in;   data answer"as      N  "). 
	Capacity    int
	DemandFloor int
	Familiarity float64
	Capped      bool
	DropCount   int
}

// kindWords is** scenariosplit **rule(  : byobjtgt/ sent  signalword  kind). 
var kindWords = []struct {
	kind  Kind
	words []string
}{
	{KindPermission, []string{"能不能", "可以吗", "允许", "权限", "授权", "vault", "不可逆", "删除", "部署"}},
	{KindReferent, []string{"那个", "这个模块", "它", "指哪", "指的是", "哪个"}},
	{KindLearnability, []string{"记住", "学到", "教它", "以后都", "下次也"}},
	{KindGapClass, []string{"找谁", "谁负责", "缺什么", "谁来"}},
}

// KindFor by scenariosplit  kind(default KindCustom). 
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

// SituationFromMemory pipe     **   ** connectbecome JEV need **close izestate **(J2:   ,  is  base). 
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

// L1Model isslow  (deepseek-flash). 
type L1Model interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Router  raise L0(basely ->  closerouteby)-> L0.5(JEV)-> L1(unique  objtgt). 
type Router struct {
	Hot *hotcache.Cache
	// ServiceRouter is L0 **    **( close /api/route): basely  intimeonly ,  ini.e.returnback. 
	ServiceRouter ServiceRoute
	JEV           JEV
	// L1 isunique  objtgt(deepseek-flash). **only "needneed    "time  **,  as" changeapprove"  . 
	L1 L1Model
	// NeedsReasoning bycalluse voice " is    task"(unique  triggersend of ). 
	NeedsReasoning bool
	// WM  emptytime, **  routeby  writebase  body**( then     is"be  "). 
	WM *plan.WorkingMemory
	// Ledger  emptytime, **  routebycloseendall      **( connect =     but  isempty ). 
	Ledger    *Ledger
	Threshold float64
	Timeout   time.Duration
	// Kind asemptytimeby KindFor(question+text)   split . 
	Kind Kind
}

func (r *Router) threshold() float64 {
	if r.Threshold <= 0 {
		return 0.70
	}
	return r.Threshold
}

// Route   split routeby, and    Ledger time**        **. 
func (r *Router) Route(ctx context.Context, text, question string, sit Situation) Decision {
	kind := r.Kind
	if kind == "" {
		kind = KindFor(question + " " + text)
	}
	d := r.routeOnce(ctx, text, question, sit)
	//         (P4): base  sent casescenario , close     body . 
	if r.WM != nil {
		r.WM.Remember("situation", plan.BoardItem{Element: text, Source: "route:" + string(d.Level)})
		if d.Choice != "" {
			r.WM.Remember("working_set", plan.BoardItem{Element: d.Choice, Source: "route:" + string(d.Level)})
		}
	}
	if r.Ledger != nil {
		if err := r.Ledger.Write(d, question, kind); err != nil {
			// fail-open(       disconnectrouteby), but**    **,  allow  . 
			d.Ledger = append(d.Ledger, LedgerEntry{Level: d.Level, Reason: "ledger_write_failed:" + err.Error()})
		}
	}
	return d
}

// routeOnce is pos split routeby; question/situation bycalluse give. 
// options    =     id + "ambiguous"(J1/J3:  allow"   ", and send   emptytime). 
func (r *Router) routeOnce(ctx context.Context, text, question string, sit Situation) Decision {
	var ledgerNote []LedgerEntry
	options := []string{"ambiguous"}
	for _, c := range sit.Candidates {
		options = append(options, c.ID)
	}
	// ---- L0: basely ini.e.returnback,     type ----
	if r.Hot != nil {
		if res, ok := r.Hot.Lookup(text); ok && res.Score >= r.threshold() {
			return Decision{
				Level: LevelL0, Action: ActionAnswer, Choice: res.Canonical,
				Confidence: res.Score, Reason: "L0 本地命中（" + res.Route + "），不问任何模型",
				Ledger: []LedgerEntry{{Level: LevelL0, Reason: "local-hit:" + res.Route, Escalated: false}},
			}
		}
	}
	// ---- L0     :  close /api/route( call type)----
	if r.ServiceRouter != nil {
		name, ok, err := r.ServiceRouter.Lookup(ctx, text)
		switch {
		case err != nil:
			// fail-open but**  **:   use !=  has
			ledgerNote = append(ledgerNote, LedgerEntry{Level: LevelL0, Reason: "route-unavailable:" + err.Error(), Escalated: false})
		case ok && name != "":
			return Decision{
				Level: LevelL0, Action: ActionAnswer, Choice: name, Confidence: 0.9,
				Reason: "L0 第二梯队：网关 /api/route 唯一命中（靠 aliases，抗 ASR 变形，不调模型）",
				Ledger: append(ledgerNote, LedgerEntry{Level: LevelL0, Reason: "gateway-route", Escalated: false}),
			}
		default:
			//  empty   see:   in/  in     to L0.5( then      inrate). 
			ledgerNote = append(ledgerNote, LedgerEntry{Level: LevelL0, Reason: "route_ambiguous=true", Escalated: false})
		}
	}
	// ----   (uniquetriggersend : calluse voice "needneed    ")----
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
	// ---- L0.5: JEV ----
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
		kind = KindFor(question + " " + text) // ① by scenariosplit ( again as custom)
	}
	if kind == KindCustom {
		// ①  empty see: split       ( then"split  approve"   become"  ly  JEV")
		ledgerNote = append(ledgerNote, LedgerEntry{Level: LevelL05, Reason: "kind_fallback=true", Escalated: false})
	}
	resp, err := r.JEV.Decide(cctx, JEVRequest{Kind: kind, Question: question, Situation: sit, Options: options})
	if err != nil {
		//  needrequire②: **  200/ time  cur JEV   use ⇒    L0 + degraded**(     to)
		return Decision{
			Level: LevelL0, Action: ActionAskUser,
			Reason:   "JEV 不可用，降级 L0 并回问（fail-open）",
			Degraded: true, DegradedReason: "JEV 调用失败：" + err.Error(),
			Ledger: append(ledgerNote, LedgerEntry{Level: LevelL0, ModelID: "jev", Reason: "jev-unavailable", Escalated: false}),
		}
	}
	if resp.Choice == "ambiguous" || resp.Choice == "" {
		//  needrequire①: ambiguous ⇒ **clarificationuseuser,    L1**(JEV    ,    typealsois )
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

// RememberJudgement pipe JEV  disconnectwrite    ( needrequire③: tgt judged_by, **  and    **). 
func RememberJudgement(w *plan.WorkingMemory, d Decision) {
	if w == nil || d.Level != LevelL05 || d.Action != ActionAnswer {
		return
	}
	w.Remember("working_set", plan.BoardItem{
		Element: "判断:" + d.Choice, Source: "jev:" + d.ModelID,
		Inferred: false, //  is disconnect, is**out  disconnect**--use JudgedBy  split, but is disconnect 
		JudgedBy: "jev",
	})
}
