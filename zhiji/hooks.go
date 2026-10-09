// hooks.go --    · harness           (M1 P0). 
//
//   objtgt(  end): 
//  1.   is"  " : pipeline side  NewZhiji /    Hooks time, pipeline orig as   change. 
//     basefile has  all   nil preventprotect--HarnessHooks refer as nil, orin  *Zhiji as nil, 
//     calluseare no-op. pipeline  no write `o.Zhiji.OnInput(...)`, noneedfirst empty. 
//  2. zhiji     import pipeline(   dependency: pipeline -> zhiji -> pipeline). 
//     because basefileuse"value   OutcomeView +   ize num" resolve : pipeline sideuse
//     `OutcomeView{...}` from Outcome charseg    out  valuei.e. ,  needneed  connect  now. 
//
// ───────────────────────────  ptlist(pipeline/pipeline.go    id) ───────────────────────────
// byunder idbaseat Run()(  182  raise)curbefore base; modify codeafter heavynew Grep  pt to. 
//
// A1 OnInput --  in in(raw tracewriteafter i.e. )
//    pt:   221   `emit(trajectory.Entry{Kind: trajectory.KindInputRaw, Content: text})`
//    itsafter in 1  : 
//       o.Zhiji.OnInput(corrected, 3)   // corrected=cleanafter base; importance default 3
//   note: empty base  (203–210  )   OnInput(nohas  in). 
//
// A6 OnDecide --   routeby(only    ,  modifylineon type  )
//    pt:   329   `out.Decision = decision`(risk  decide out,  in  before). 
//    itsafter in 3–5  : 
//       p := zhiji.TaskProfile{Complexity: estimateComplexity(intent), Domain: intent.Space}
//       if rd, err := o.Zhiji.OnDecide(p); err == nil {
//           _ = rd //    form: only  shadow_log, lineon  orig type(M1); M2 againuse rd.ModelID
//       }
//   note: NewZhiji after  `z.Router.SetShadow(true)` only write shadow_log. 
//
// A3 OnTaskEnd -- taskcloseendsafety trace + rev (artifact  after, return before )
//    pt:   459   `return out, nil`( doneexit). 
//    itsbefore in 2  : 
//       o.Zhiji.OnTaskEndView(ctx, OutcomeViewFromOutcome(out))
//       return out, nil
//     exit(209 emptyrefer  / 289     / 322 block / 394     )M1   restrict ; 
//   ifneedsafetyoverwrite,      `return out, nil` beforesamekindcall   OnTaskEndView(View by
//   confirmed/Blocked/Receipts e.g.  ). 
//
// A7 Compress --  onunder   (  before, onunder   valuetime )
//    pt: curbefore Run()  no form onunder branch;  ptsame A6, i.e.  329   decision ofafter,   ofbefore. 
//    itsafter in  ( valueand Compressor.MaxSummaryTokens*4 to , char   ): 
//       if ctxTokens := estimateTokens(intent.Context) + historyTokens(); ctxTokens > 16000 {
//           if cc, err := o.Zhiji.Compress(ctx, zhiji.CompressInput{Full: full, Segment: tail}); err == nil {
//               intent.Context = cc.Summary // onlyuse  after  body; goals/rules/pending by   origkindkeep 
//           }
//       }
//
// A2 Start / Stop -- rev line occur  period(   Run() in, server startstoptime    )
//    pt: server start (e.g. server.New / ListenAndServe ofbefore) : `zh.Start(ctx)`; 
//      out(shutdown hook) : `zh.Stop(); zh.Close()`. 
//   to  pipeline side   one-liner(server start time): 
//       o.Zhiji = zhiji.NewHarnessHooks(zh)   // zh==nil time tosafety no-op hooks
//
// ───────────────────────────    num(AttachZhiji etc  ) ───────────────────────────
//  pos  `AttachZhiji(o *pipeline.Options, z *zhiji.Zhiji)` write  pipeline  in(   value), 
// becauseas zhiji    import pipeline. basefileonly provide NewHarnessHooks(z). 
package zhiji

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// HarnessHooks is zhiji to pipeline            (  nil-safe). 
// occur  period: server start time NewHarnessHooks(z)     ,  to pipeline.Options    charseg; 
//  require  Run() calluseread-only  ,   modify(rev line writeday     goroutine, andsendsafesafety). 
type HarnessHooks struct {
	z *Zhiji
}

// NewHarnessHooks      zhiji  example.   nil  tosafety no-op   hooks(etc at   , 
// pipeline   aschangeize); i.e. returnbackvalueagainbe become nil refer ,  has  alsoallalready  h==nil preventprotect. 
func NewHarnessHooks(z *Zhiji) *HarnessHooks {
	return &HarnessHooks{z: z}
}

// OnInput  in in  (no-op safesafety). summary=cleanafter in; importance default 3. 
func (h *HarnessHooks) OnInput(summary string, importance float64) {
	if h == nil || h.z == nil {
		return
	}
	h.z.OnInput(summary, importance)
}

// BeforeDecision decide beforebaselinenotein  (no-op safesafety:    returnback nil, nil, pipeline  ednotein). 
func (h *HarnessHooks) BeforeDecision(ctx context.Context, query string) (*Baseline, error) {
	if h == nil || h.z == nil {
		return nil, nil
	}
	return h.z.BeforeDecision(ctx, query)
}

// OnTaskEnd taskcloseendtrace  (no-op safesafety). calluse   firstuse OnTaskEndView(    CallLog+rev ). 
func (h *HarnessHooks) OnTaskEnd(ctx context.Context, log CallLog) error {
	if h == nil || h.z == nil {
		return nil
	}
	return h.z.OnTaskEnd(ctx, log)
}

// OnDecide   routeby  (no-op safesafety:    returnback valuedecide , shadow=false,    lineon). 
func (h *HarnessHooks) OnDecide(p TaskProfile) (RouteDecision, error) {
	if h == nil || h.z == nil {
		return RouteDecision{}, nil
	}
	return h.z.OnDecide(p)
}

// Compress  onunder     (no-op safesafety:    returnback value, pipeline useorigonunder ). 
func (h *HarnessHooks) Compress(ctx context.Context, in CompressInput) (CompressedContext, error) {
	if h == nil || h.z == nil {
		return CompressedContext{}, nil
	}
	return h.z.Compress(ctx, in)
}

// Start start rev line (no-op safesafety). server start timecalluse  . 
func (h *HarnessHooks) Start(ctx context.Context) {
	if h == nil || h.z == nil {
		return
	}
	h.z.Start(ctx)
}

// Stop stopstoprev line (no-op safesafety). server    outtimecalluse. 
func (h *HarnessHooks) Stop() {
	if h == nil || h.z == nil {
		return
	}
	h.z.Stop()
}

// ─────────────────────────── Outcome -> zhiji signal  (value  resolve ) ───────────────────────────

// OutcomeView is pipeline.Outcome      (valueclasstype,    import pipeline   dependency). 
// pipeline sideby Outcome charseg      ; semanticto note list charsegafter. 
type OutcomeView struct {
	RequestID    string  // out.RequestID(day /rev  taskID)
	TaskProfile  string  // task  tgt (  ; OnTaskEnd needrequire empty).     intent.Intent+Space
	Model        string  //      type(attributionuse)
	Cost         float64 // token  tobecomebase
	Confirmed    bool    // out.Confirmed(humanconfirm ed)
	HadFailure   bool    // out.Receipts instore  OK=false(~= pipeline.hasFailure)
	Blocked      bool    // out.Receipts instore  Blocked != ""(safesafetyblock=revoked)
	VerifyFail   bool    // out.Verify.Status == verify.StatusFail
	UserOutcome  string  // useuser formrev : "success"|"failed"|""(   first signal )
}

// FeedbackSignal is  izeafter rev signal(   split and   ). 
type FeedbackSignal struct {
	Outcome    string  // success|failed|revoked|judge
	Source     string  // user_feedback|tool_error|confirm_flag|llm_judge
	Confidence float64 // 0–1(    ,      )
	Reason     string  //  sent attribution(   )
}

// ClassifyFeedback bysignal  first pipe  taskartifact  becomerev signal. 
//  first : useuserrev  >     (Receipts OK=false) > donetgt (Confirmed/pos  return) > LLM-judge. 
// revoked(safesafetyblock Blocked) as      example   diff,  firstat   failed. 
func ClassifyFeedback(v OutcomeView) FeedbackSignal {
	// 1. useuser formrev (   first ,      )
	if v.UserOutcome == "success" || v.UserOutcome == "failed" {
		return FeedbackSignal{
			Outcome:    v.UserOutcome,
			Source:     "user_feedback",
			Confidence: 0.95,
			Reason:     "用户显式反馈",
		}
	}
	// 2. safesafetyblock = revoked(      example,    diff)
	if v.Blocked {
		return FeedbackSignal{
			Outcome:    "revoked",
			Source:     "tool_error",
			Confidence: 0.9,
			Reason:     "安全拦截（Receipt.Blocked 非空）",
		}
	}
	// 3.     (back   or verify  ed)
	if v.HadFailure || v.VerifyFail {
		return FeedbackSignal{
			Outcome:    "failed",
			Source:     "tool_error",
			Confidence: 0.8,
			Reason:     "工具回执失败或 verify 未过",
		}
	}
	// 4. donetgt (humanconfirm ed / pos  return)
	if v.Confirmed {
		return FeedbackSignal{
			Outcome:    "success",
			Source:     "confirm_flag",
			Confidence: 0.6,
			Reason:     "人工确认通过",
		}
	}
	// 5.  bot: no formsignal,   LLM-judge
	return FeedbackSignal{
		Outcome:    "judge",
		Source:     "llm_judge",
		Confidence: 0.4,
		Reason:     "无显式信号，交 LLM-judge",
	}
}

// BuildCallLog pipe OutcomeView   become   zhiji CallLog(OnTaskEnd in ). 
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

// OnTaskEndView is OnTaskEnd     (no-op safesafety): 
// first BuildCallLog safety tracewrite-back, againby ClassifyFeedback    split write  rev signal. 
// rev write    disconnect chainroute(tracealready  , rev is     patchfill). 
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
		// rev signalsplit write(taskID=RequestID, thenatandtrace join). error    disconnect. 
		_ = h.z.Contract.LogFeedback(ctx, v.RequestID, sig.Outcome, sig.Confidence)
	}
	return nil
}

// ActiveEntities 返回所有 active 实体的描述字符串，用于注入 LLM context。
// nil-safe。
func (h *HarnessHooks) ActiveEntities() string {
	if h == nil || h.z == nil || h.z.Entities == nil {
		return ""
	}
	es := h.z.Entities.AllActive()
	if len(es) == 0 {
		return ""
	}
	out := ""
	for _, e := range es {
		line := e.Canonical
		if len(e.Aliases) > 0 {
			line += " (代指: " + strings.Join(e.Aliases, "/") + ")"
		}
		if e.Desc != "" {
			line += " — " + e.Desc
		}
		out += "  - " + line + "\n"
	}
	return out
}

// LearnEntity 记录一个新实体（用户说"X是Y"时调用）。R9-D8 知己多源：
// sources 记录认知来源（voice-local=本地对话提取 / external-hub=外部模型中心 /
// longwall=长城长期记忆库），缺省 voice-local。
func (h *HarnessHooks) LearnEntity(canonical, desc string, aliases []string, sources ...string) {
	if h == nil || h.z == nil || h.z.Entities == nil {
		return
	}
	src := "voice-local"
	if len(sources) > 0 && sources[0] != "" {
		src = sources[0]
	}
	h.z.Entities.Upsert(Entity{
		Type:       EntityPerson,
		Canonical:  canonical,
		Aliases:    aliases,
		Desc:       desc,
		Confidence: 0.9,
		Source:     src,
	})
}

// RecentCandidates 返回最近提到的 topN 实体（用于代指 resolve）。
// 如果用户说"那个项目"，这些就是候选。
func (h *HarnessHooks) RecentCandidates(topN int) []string {
	if h == nil || h.z == nil || h.z.Entities == nil || h.z.Mentions == nil {
		return nil
	}
	var canonicals []string
	for _, e := range h.z.Entities.AllActive() {
		canonicals = append(canonicals, e.Canonical)
	}
	return h.z.Mentions.TopCandidates(canonicals, topN)
}

// RecordMention 记录一次实体提及。
func (h *HarnessHooks) RecordMention(canonical string) {
	if h == nil || h.z == nil || h.z.Mentions == nil {
		return
	}
	h.z.Mentions.Record(canonical)
}

// RecentDialogue 返回最近 N 轮用户输入，用于代指 resolve。
func (h *HarnessHooks) RecentDialogue(n int) string {
	if h == nil || h.z == nil || h.z.RawLog == nil {
		return ""
	}
	entries := h.z.RawLog.Recent(n)
	if len(entries) == 0 {
		return ""
	}
	out := ""
	for i, e := range entries {
		out += fmt.Sprintf("  [%d] 用户说: %s\n", i+1, truncateStr(e, 60))
	}
	return out
}

// RecentSTM 返回最近 N 条 self_model 记忆（用户偏好/事实）。
func (h *HarnessHooks) RecentSTM(n int) string {
	if h == nil || h.z == nil || h.z.Store == nil {
		return ""
	}
	selfs := h.z.Store.SelfModel("")
	if len(selfs) == 0 {
		return ""
	}
	out := ""
	count := 0
	for i := len(selfs) - 1; i >= 0 && count < n; i-- {
		s := selfs[i]
		if s.Status == StatusActive {
			out += "  - " + truncateStr(s.Text, 80) + "\n"
			count++
		}
	}
	return out
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
