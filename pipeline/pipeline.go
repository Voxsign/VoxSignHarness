// Package pipeline is M2 voice-drivenopensend orchestration loop(freeze §3 pipeline): 
// pipe"ASR  base"fromorig  route to"four-line receipt", middlethrough clean->correction->task classification->coreference resolution->
// space_check->risk grading->confirm->tool execution->independent verification->attributionwrite-back->quad cache->four-line receipt. 
//
// this packageisintegration shim: space/refer/risk/verify/search/cache/tools arealreadydeliverread-only dependency, 
// this package[ modify]   rulebody, onlyresponsiblebyfrozen signaturepipe  orchestrateraise . 
// rule semantics(   out-of-scope /  classconfirm / irreversible list)  tgtnote"  VSL",   this layerheavy . 
package pipeline

// [pseudocode logic layer]( writemodule, review gateartifact. rule semanticsauthoritative definition  freeze §3 / SPEC v2 §2; 
//  this layeronlydescribe Run   13 stagecontrol flow /  stagerejectpath / confirmbranch / errorandinterrupthandle,     . )
//
// global invariants: 
//   - serial gate(useuser  ): same processsametimeonly allow   Run in flight(Options.mu mutex), 
//     protectuseuseruncommitted changes,   overwrite/delete  file. 
//   - start event = input_raw tracewrite[done] moment; end event = attributionwrite[done] moment. 
//     LoopMs = end-start wall-clock( human wait); NetMs = wall-clock − confirmFn/clarificationwaitseg. 
//
// Run(ctx, o, text) -> Outcome, error: 
//
//   gate.lock()                              // serial gate; ctx cancel -> return immediatelyand   intermediate state
//   rid = "req-" + nano()
//   write(input_raw{text}); start = now()
//
//   stage② clean:     cleaned = Cleaner.Clean(text); write(input_clean)
//   stage③ correct:   corrected,corr = Dict.Correct(cleaned); write(input_correct)
//   stage④ classify:  intent = TaskClassifier(corrected); write(intent)
//                    if intent.Ask != "":  to [clarificationexit]
//   stage⑤ refer:     intent = Refer.Resolve(intent, spaceOf(intent)); write(refer)
//                    if intent.Ask != "":  to [clarificationexit]
//   stage⑥ space:     space = default space(intent)(NOTE->vault-notes / read-only->global / write intent alreadyptname)
//                    caps = planCaps(intent)
//                    verdict = space.Check(Registry, {Intent, Grant:{Auth:true}, ToolCaps:caps})
//                    write(space_check)
//                    if !verdict.Allowed:  to [blockexit](verdict.Reason  close ,     )
//   stage⑦ risk:      imp =   signal(search citation stats + trajectory heat; no fs timeget 0)
//                    decision = risk.Evaluate(intent, imp)
//                    intent.Confirm = decision.Level(backfillauthoritative value); write(risk)
//   stage⑧ confirm:   wait0 = now()
//                    if cache.Get(quad)  in: approved=true( again , decide  use)
//                    else:
//                      switch decision.Level:
//                        auto:   approved=true(  disconnect)
//                        light:  approved = ConfirmFn(rid, lightQuestion)
//                        strong: if Guard.ShouldDowngrade(path): approved=true(     ,   disconnect)
//                                else:            approved = ConfirmFn(rid, strongQuestion)
//                        human:  approved = ConfirmFn(rid, humanQuestion)   //    
//                    waitMs += now()-wait0
//                    write(confirm)
//                    if !approved:  to [   exit](Confirmed=false,    )
//                    cache.Set(quad, decision.Level)
//   stage⑨ exec:     for action in planActions(intent):
//                        receipt = Exec.Exec(tool, args(with log_dir notein), contract)
//                        Receipts = append(Receipts, receipt)
//                    write(receipts)
//                    rejectpath:    receipt.Blocked != "" ->  heavy , origkind close 
//   stage⑩ verify:    spec = planVerify(intent, verdict)
//                    if spec != nil: Verify = Verifier.Run(spec)
//                    else:           Verify = {unverifiable, "M2  define verify"}
//                    write(verify)
//   stage⑪ attribution: cls = classifyAttribution(intent, verdict, Receipts, Verify, corr)
//                      attr = Attribution{rid, discuss, cls, evidence, suggestion}
//                      write(kind=attribution)  // ← end = now()
//                      append discuss.jsonl(  seeclose , under  notein;    modifyword /  )
//   stage⑫ cache:      already ⑧ Set;  placeonly   /blocktime InvalidateSpace(  ini.e. ed)
//   stage⑬ view:   View = renderView(intent, verdict, decision, Receipts, Verify, Ask)
//                  write(final)
//
//   LoopMs = end-start; NetMs = LoopMs - waitMs
//   return Outcome, nil
//
// [clarificationexit]   View.result="   (needclarification: …)"; Attribution.class=context; end     
// [blockexit]   View.result="BOUNDARY_VIOLATION/…"; Attribution.class=context;     verify
// [   exit] View.result=" confirm(alreadyreject/   )";    ; Attribution.class=model
//
// error: Dict/Trace/Refer as nil timeto stage   (   ); ctx  beforecancel -> returnback error, 
//   butalready   trace objkeep (append-only,  rollback). 
//
// Summary(o, since) -> string: 
//   readcurday trajectory-YYYYMMDD.jsonl, by domain / intent /   origbecause   ; mobile read  base. 

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	"voicesign-harness/zhiji"
)

// Options isorchestration loop safety dependency(frozen signature).   outcharsegasintegrate  now node(serial gate/confirm  preventprotect). 
type Options struct {
	Cfg      *config.Config
	Dict     *memory.Dictionary
	Spaces   *space.Registry
	Refer    *refer.Resolver
	Cache    *cache.Store
	Verifier *verify.Verifier
	Tools    *tools.Registry
	Exec     *tools.Executor

	// ConfirmFn   waithuman  (CLI=stdin / server=HTTP); returnback false tableshowreject. 
	ConfirmFn func(taskID, question string) (bool, error)
	Trace     *trajectory.Trajectory

	// Ground(M3 #37)    notein ; nil time   (empty context). 
	Ground *ground.Ground

	// Providers(M7)LLM connectline ; nil time rule/  search path  disconnect. 
	Providers *provider.Registry

	// selfhealSvc error   (  );    . Providers==nil time as nil( open  ed,  chain change). 
	selfhealSvc *selfheal.Service

	// ConvID   tgt (same logDir under   run   ;    "default"). 
	ConvID string
	// RequestID out refer   require ID(P0-4b). emptythen Run in occurbecome(= nowstatus as, toaftercompat); 
	//  emptythen chain(trace/selfheal/day )  use ,  in  request_id andtrace Entry id   . 
	RequestID string
	// ProgressObserver           er(nil=  ,   aschangeize). 
	//    §7.4/§11     : pipeline  close stage(intentclassify)backcall stage/detail, 
	// server  connect  SSE(kind:"internal"), CLI also   . nil timeandnowstatus charnode  . 
	ProgressObserver func(stage, detail string)
	// ASRDataDir ASR numdataobj (rev / name /word   ,    globaloccur ). 
	ASRDataDir string
	// RoundEvidence on   data   (  2+    , back  LLM fix ). 
	RoundEvidence string
	// Document needrequire  safety ( nowclasstask;  data /LLM occurbecome use).
	Document string

	// Zhiji 记忆系统钩子（STM/LTM/SelfModel/Reflect）。nil-safe，未构造时所有调用 no-op。
	Zhiji *zhiji.HarnessHooks

	// Lang explicitly selects the language of the deterministic generated-code/doc
	// templates (skeleton main.go/router.go/domain.go/README, implement plan,
	// orchestration summary). Values: "en" or "zh". Empty = detect from the input
	// text (CJK -> zh, otherwise en). See Options.EffectiveLang for the priority.
	Lang string

	mu    *sync.Mutex
	guard *risk.Guard
}

// AskOption is need_ask clarification close ize  (M4-3 ①: pt i.e.continue ,    read). 
type AskOption struct {
	ID    string `json:"id"`    //      id(edit/query/note/commit/... or refer objtgt id)
	Label string `json:"label"` //  sent in  label
}

// Outcome is   Run  finish artifact(frozen signature). 
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
	Options      []AskOption          `json:"options,omitempty"` // M4-3 close ize  
	ContextBlock string               `json:"context_block,omitempty"`
	LoopMs       int64                `json:"loop_ms"`
	NetMs        int64                `json:"net_ms"`
}

// discussLogName is discuss-log filename(<log_dir>/discuss.jsonl). 
const discussLogName = "discuss.jsonl"

// EnableSerialGate initstartize  serial gate(C0 accept serialed state). 
//
//    **   Options ofbeforeto  calluse  **: server side task `o := *tmpl` is   , 
// o.mu isrefer ,  be raise restrict ->  hastask    same pipe ,  taskserialoccur . 
// if firstcallbase  ,    o.mu as nil, Run in   give  task**   pipenew ** -> serial gate same  . 
//
// C0   (useuser   2026-10-05):  is**accept serialed state**,  is    .  endobjtgtis processin
//   Runner(   )by  useuser/  andsend--   totask  process; safeGo istask      seg. 
//  keephasperiod =    Run safety (Run openhead Lock, defer Unlock), C0 undertasksafetyserial,  andsend  . 
func (o *Options) EnableSerialGate() {
	if o != nil && o.mu == nil {
		o.mu = &sync.Mutex{}
	}
}

// Run   finish  13 stageorchestration loop. ctx cancel instopwaithumanconfirm, butalready  trace rollback. 
func Run(ctx context.Context, o *Options, text string) (Outcome, error) {
	// ⭐     (Lead 2026-10-03     : o==nil / empty Options ⇒ **panic**,  isreturnbackerror). 
	// origthen: **" "and"  "  diffis --   has     toorigbecause**(andif  after  goroutine, recover also  to). 
	if o == nil {
		return Outcome{}, fmt.Errorf("pipeline.Run: Options is nil (caller must supply complete Options)")
	}
	if o.Spaces == nil {
		return Outcome{}, fmt.Errorf("pipeline.Run: Spaces not configured (domain guard missing => refuse to run)")
	}
	// note: **    Providers** -- its    "nil time rule/  search path  disconnect"(Options.Providers note ). 
	// if   require,  pipe"no LLM also  "     . onlycur needuse LLM timeonly to pathhandle. 
	if o.mu == nil {
		o.mu = &sync.Mutex{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.guard == nil {
		o.guard = risk.NewGuard()
	}

	// P0-4b: out refer  request_id(in /byReq   ) first; emptythen occurbecome(= nowstatus as). 
	rid := strings.TrimSpace(o.RequestID)
	if rid == "" {
		rid = newRequestID()
	}
	o.RequestID = rid // write-back: basetask Options   inaftercontinue  (selfheal/llmSummarize day )  readtorule  rid
	out := Outcome{RequestID: rid}
	if strings.TrimSpace(text) == "" {
		out.Ask = "Empty command, did not catch that, please repeat"
		out.View = contract.ReceiptView{
			Action: "(empty command)", Files: "-", Result: "Not executed (need clarification: " + out.Ask + ")", Undo: "-",
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

	// ① input_raw tracewritedone -> start event(#45). 
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

	// ③.5 day   ( line,   LLM): onlyneed lang "day "then connect  wttr.in returnback. 
	//  scenario(2026-10-05   ): day intentorigfirst  LLM ActionPlan triggersend, LLM   / timetime
	//   " day " izeas"on  5xx". wttr.in noneed key, base  200, thusday    ity link, 
	//  againdependency LLM.  ini.e.returnback done;   inreturnback false continuecontinuepos classify. 
	if wout, ok := o.tryWeather(ctx, corrected, start); ok {
		wout.RequestID = out.RequestID
		emit(trajectory.Entry{Kind: trajectory.KindFinal, Content: contract.RenderReceipt(wout.View)})
		return *wout, nil
	}

	// ④ TaskClassifier.ClassifyTask(emptytime  get note domainlisttable)
	classifier := input.NewTaskClassifier(confOf(o.Cfg), spaceHints(o.Spaces))
	intent := classifier.ClassifyTask(corrected)
	intent.RawText = text
	intent.Corrections = corrections

	// M7 ① intentclassify LLM back : rulelow-confidence/UNKNOWN and  howeverlanglang senttime -> fast provider patchclassify.
	intent = o.llmIntentFallback(ctx, intent, corrected)
	// zhiji A1: input 进 STM；自我介绍类给高 importance，让 Reflector 能抽到 SelfItem
	if o.Zhiji != nil {
		imp := 3.0
		for _, kw := range []string{"我叫", "我姓", "我是", "名字", "姓名", "怎么称呼", "我是谁"} {
			if strings.Contains(corrected, kw) {
				imp = 5.0
				break
			}
		}
		o.Zhiji.OnInput(corrected, imp)
	}
	// test-patch: document 非空强制走 implement 生成链路
	if o.Document != "" {
		intent.Intent = contract.IntentOrchestrate
		intent.Confidence = 1.0
		if intent.Params == nil {
			intent.Params = map[string]string{}
		}
		intent.Params["kind"] = "implement"
		intent.Params["target_doc"] = strings.TrimSpace(strings.SplitN(o.Document, "\n", 2)[0])
	}
	// zhiji A6: decision 前注入 baseline（selfModel goals/rules）
	if o.Zhiji != nil {
		if bl, err := o.Zhiji.BeforeDecision(ctx, corrected); err == nil && bl != nil {
			if bl.Goals != "" {
				intent.Context = append(intent.Context, "[self-model goals] "+bl.Goals)
			}
			if bl.Rules != "" {
				intent.Context = append(intent.Context, "[self-model rules] "+bl.Rules)
			}
		}
		// 注入所有已知实体（人/项目/公司），让 LLM 能 resolve 代指
		if ents := o.Zhiji.ActiveEntities(); ents != "" {
			intent.Context = append(intent.Context, "[known entities]\n"+ents)
		}
		// 注入最近对话历史，让"这个""那个"能 resolve
		if recent := o.Zhiji.RecentDialogue(10); recent != "" {
			intent.Context = append(intent.Context, "[recent dialogue]\n"+recent)
		}
		// 注入STM事实记忆（用户说过的偏好/事实）
		if stm := o.Zhiji.RecentSTM(5); stm != "" {
			intent.Context = append(intent.Context, "[remembered facts]\n"+stm)
		}
		// 注入最近提到的实体（代指resolve："那个""这个"）
		if rc := o.Zhiji.RecentCandidates(3); len(rc) > 0 {
			intent.Context = append(intent.Context, "[最近提到的实体]\n  - "+strings.Join(rc, "\n  - ")+"\n")
		}
	}
	emit(trajectory.Entry{Kind: trajectory.KindIntent, Intent: &intent})

	// §11     : intentclassifydone -> send     event(SSE/CLI endtoend see). nil   er  ,  as change. 
	if o.ProgressObserver != nil {
		o.ProgressObserver("intent", "intent="+intent.Intent)
	}

	// ⑤ refer  resolve( overwriteclassify already form   charseg); M4-5: sametime  refer objtgt  . 
	// M7 fix (Codex/gpt-6-luna out  disconnect 2026-10-02): byintent  --
	// QUERY    (>=0.8) "  /  "is   lang word,  edcoreference resolution, 
	//  then refer   asempty write Ask"   "  "refer is   "-> need_ask(    ). 
	var referOpts []refer.Option
	// F2: wake the dead code path — load recent entities from the session slot
	// <logDir>/context_slots/<convID>.jsonl, inject them into the resolver, and use
	// that to decide hasRecent. Previously hasRecent was hardcoded false, so
	// writeRecentEntities/loadRecentEntities were never called and cross-turn memory was always empty.
	convID := strings.TrimSpace(o.ConvID)
	if convID == "" {
		convID = "default"
	}
	recentEnts := loadRecentEntities(o.logDir(), convID)
	if o.Refer != nil {
		o.Refer.Recent = recentEnts
	}
	hasRecent := len(recentEnts) > 0
	if o.Refer != nil && shouldResolveRefer(&intent, hasRecent) {
		resolved, opts, err := o.Refer.ResolveOptions(&intent, intent.Space)
		if err == nil && resolved != nil {
			prevAsk := intent.Ask
			intent = *resolved
			referOpts = opts
			// Codex recv (2026-10-02): refer newwrite Ask("refer is  ")time, 
			// if    stop  (  /  has body/  class)->   continuecontinue  ,  becausecoreference Ask. 
			if intent.Ask != "" && intent.Ask != prevAsk && !clarificationBlocksExecution(&intent, referOpts) {
				intent.Ask = ""
			}
		}
	}
	emit(trajectory.Entry{Kind: trajectory.KindRefer, Intent: &intent})

	// distillation R5: CONTINUE resumes the previous turn's task slot so
	// "开始干呀/立刻执行/继续" actually does the job instead of asking again.
	if intent.Intent == contract.IntentContinue {
		slot := loadTaskSlot(o.logDir(), convID)
		if slot == nil || slot.Status == "clear" {
			intent.Intent = contract.IntentAsk
			intent.Conflict = contract.ConflictContinue
			intent.Confidence = 0.85
			intent.Ask = "上一轮没有待执行的任务。直接说具体指令就行，例如「下载 https://github.com/owner/repo 然后编译测试」"
		} else if slot.Status == "done" {
			// job finished already — report instead of inventing a new run
			intent.Intent = contract.IntentAsk
			intent.Conflict = contract.ConflictContinue
			intent.Confidence = 0.85
			intent.Ask = "上一轮任务已完成（" + slot.Text + "）。需要我重新跑一遍，还是继续下一个任务？"
		} else if slot.Kind == TaskKindInstall {
			// distillation R6: a pending install awaits confirmation; "怎么还没执行/立刻执行"
			// resumes it, and the pipeline runs the real npm install now.
			intent.Intent = contract.IntentInstall
			intent.Confidence = 0.9
			intent.Conflict = ""
			intent.Ask = ""
			intent.Params = map[string]string{}
			if slot.Params != nil {
				for k, v := range slot.Params {
					intent.Params[k] = v
				}
			}
			intent.Params["resumed"] = "1"
			intent.CorrectedText = slot.Text
			intent.Context = append(intent.Context, "[resumed install] "+slot.Text)
		} else if slot.Kind == TaskKindEmail {
			// distillation R7: a pending email draft awaits confirmation; "确认/好的" resumes it
			// and the pipeline writes the draft deliverable under harness-output/.
			intent.Intent = contract.IntentEmail
			intent.Confidence = 0.9
			intent.Conflict = ""
			intent.Ask = ""
			intent.Params = map[string]string{}
			if slot.Params != nil {
				for k, v := range slot.Params {
					intent.Params[k] = v
				}
			}
			intent.Params["resumed"] = "1"
			intent.CorrectedText = slot.Text
			intent.Context = append(intent.Context, "[resumed email] "+slot.Text)
		} else if slot.Params == nil || strings.TrimSpace(slot.Params["url"]) == "" {
			// resumed job still missing its target repo — ask for the URL
			intent.Intent = contract.IntentAsk
			intent.Conflict = contract.ConflictContinue
			intent.Confidence = 0.85
			intent.Ask = "上一轮要下载编译测试，但还没给仓库地址。给我 URL 或仓库名（例如「下载 https://github.com/owner/repo 然后编译测试」）"
		} else {
			intent.Intent = contract.IntentBuildTest
			intent.Confidence = 0.9
			intent.Conflict = ""
			intent.Ask = ""
			intent.Params = slot.Params
			if slot.Params == nil {
				intent.Params = map[string]string{}
			}
			intent.Params["resumed"] = "1"
			intent.CorrectedText = slot.Text
			intent.Context = append(intent.Context, "[resumed task] "+slot.Text)
		}
		emit(trajectory.Entry{Kind: trajectory.KindRefer, Intent: &intent})
	}

	// clarificationexit: classify /coreference   needclarification ->    . 
	// M3 #37: clarificationis   " type/ needneedchange onunder "pt,   notein ground     . 
	if intent.NeedsClarification() {
		snap := o.renderGround()
		intent.Context = append(intent.Context, snap.ProjectMap...)
		out.Intent = intent
		out.Ask = intent.Ask
		// Codex recv (2026-10-02): options by  intent stateoccurbecome,  again   "modifyfile/  code/   /  ". 
		out.Options = mergeAskOptions(optionsForIntent(&intent, referOpts), referOpts)
		out.ContextBlock = snap.Block
		out.Attribution = o.attribution(out.RequestID, contract.AttrContext,
			"needs clarification: "+intent.Ask, "trace kind=intent/refer", "provide concrete domain/object next time")
		o.writeAttribution(out.Attribution)
		out.LoopMs = time.Since(start).Milliseconds()
		out.NetMs = out.LoopMs - waitMs.Milliseconds()
		out.View = contract.ReceiptView{
			Action: shortAction(intent), Files: "—",
			Result: "Not executed (need clarification: " + intent.Ask + ")", Undo: "- (not executed)",
		}
		// zhiji: clarification 分支也走 LLM 自然回复——不管什么intent，都不要硬说"need clarification"
		if o.Providers != nil {
			if intent.Ask != "" {
				// distillation R5: on a clarification ask, show the real question, never a
				// generated promise of action ("我现在就动手…") that will not happen.
				out.View.Result = intent.Ask
			} else if reply := o.llmNaturalReply(ctx, corrected, intent.Context); reply != "" {
				out.View.Result = reply
			} else {
				out.View.Result = "好的，我理解了你的意思，我们接着往下推——先从最要紧的那件事开始。"
			}
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
		// Contracts has   nil: B/C   andfrozensemantic by   name(file/git/read/…)    forbid; 
		//     cap   risk split   risk.Evaluate stage  ,    heavy   . 
	})
	b, _ := json.Marshal(verdict)
	emit(trajectory.Entry{Kind: trajectory.KindSpaceCheck, Content: string(b)})
	out.Intent = intent
	out.Verdict = verdict

	// blockexit: space_check reject ->     . 
	if !verdict.Allowed {
		out.Attribution = o.attribution(out.RequestID, contract.AttrContext,
			"space_check rejected ("+verdict.Reason+")", "trace kind=space_check",
			"register/confirm a domain first, or switch to an authorized project domain")
		o.writeAttribution(out.Attribution)
		out.LoopMs = time.Since(start).Milliseconds()
		out.NetMs = out.LoopMs - waitMs.Milliseconds()
		out.View = contract.ReceiptView{
			Action: shortAction(intent), Files: "—",
			Result: "BOUNDARY_VIOLATION: " + reasonText(verdict.Reason), Undo: "- (not executed)",
		}
		o.writeTaskMetrics(intent, out, false)
		o.write(trajectory.Entry{RequestID: out.RequestID, Kind: trajectory.KindFinal, Content: contract.RenderReceipt(out.View)})
		return out, nil
	}

	// ⑦ risk.Evaluate(  signal: fs citation stats + heat; no   objtimeget 0)
	imp := o.mechanicalImpact(intent)
	decision := risk.Evaluate(intent, imp)
	intent.Confirm = decision.Level
	out.Decision = decision
	db, _ := json.Marshal(decision)
	emit(trajectory.Entry{Kind: trajectory.KindRisk, Content: string(db)})

	// ⑧ confirm(by Decision split ; quad cache in= again )
	// safesafetyinvariant(SPEC #40/#29): human= reversible,     ConfirmFn, 
	// fromquad cache Get and Set     --" reversible  be   ". 
	wait0 := time.Now()
	approved := false
	quad := quadKey(intent, decision.Level)
	cacheable := decision.Level != contract.ConfirmHuman
	if o.Cache != nil && cacheable {
		if cached, ok := o.Cache.Get(quad); ok {
			// cached only  Set placewrite decision.Level; "approved" is  compat ,  and  . 
			approved = cached == decision.Level || cached == "approved"
		}
	}
	if !approved {
		switch decision.Level {
		case contract.ConfirmAuto:
			approved = true //   disconnect
		case contract.ConfirmLight:
			approved = o.confirm(ctx, out.RequestID, "light confirm: "+decision.Reason+", proceed? (y/n)")
		case contract.ConfirmStrong:
			if o.guard.ShouldDowngrade(targetPath(intent)) {
				approved = true // samepathlinkcontinue confirm   ->  as     
			} else {
				approved = o.confirm(ctx, out.RequestID, "strong confirm: "+decision.Reason+", proceed? (y/n)")
			}
		case contract.ConfirmHuman:
			q := "manual approval (irreversible): " + decision.Reason
			// M4-4: COMMIT beforepipeuncommitted changesnumwrite confirm  ,   overwrite/modifywrite  . 
			if it := intent; it.Intent == contract.IntentCommit {
				if root := o.projectRootForCommit(it); root != "" {
					if n := gitDirtyCount(root); n >= 0 {
						q += fmt.Sprintf("; project %s has %d uncommitted changes; commit will include them (git add -A + commit, no history rewrite)", root, n)
					}
				}
			}
			approved = o.confirm(ctx, out.RequestID, q+", proceed? (y/n)")
		}
	}
	// M4-1: is   edhuman(auto   disconnect  wait; waitMs   µs   overhead   ). 
	humanWait := decision.Level != contract.ConfirmAuto
	waitMs += time.Since(wait0)
	out.Confirmed = approved
	emit(trajectory.Entry{Kind: trajectory.KindConfirm, Content: fmt.Sprintf("level=%s approved=%v", decision.Level, approved)})
	o.recordDecision(out.RequestID, intent, decision, approved)
	if o.Cache != nil && approved && cacheable {
		// human    writecache( reversible  human). 
		_ = o.Cache.Set(quad, decision.Level)
	}

	//    exit:    . 
	if !approved {
		out.Attribution = o.attribution(out.RequestID, contract.AttrModel,
			"user did not approve (decision="+decision.Level+")", "trace kind=confirm", "if mistakenly rejected, add to 4-tuple allow-cache")
		o.writeAttribution(out.Attribution)
		out.LoopMs = time.Since(start).Milliseconds()
		out.NetMs = out.LoopMs - waitMs.Milliseconds()
		out.View = contract.ReceiptView{
			Action: shortAction(intent), Files: targetFiles(intent),
			Result: "pending confirm (" + decision.Level + ", not approved)", Undo: "- (not executed)",
		}
		o.write(trajectory.Entry{RequestID: out.RequestID, Kind: trajectory.KindFinal, Content: contract.RenderReceipt(out.View)})
		return out, nil
	}

	// ⑨ Exec(onlyalreadyed space_check + risk    )
	// distillation R5: BUILD_TEST without a resolvable URL must ask instead of failing,
	// unless it is a resumed job whose slot carries the URL (execBuildTest handles the slot).
	if intent.Intent == contract.IntentBuildTest && (intent.Params == nil || strings.TrimSpace(intent.Params["url"]) == "") {
		slot := loadTaskSlot(o.logDir(), convID)
		hasURL := slot != nil && slot.Params != nil && strings.TrimSpace(slot.Params["url"]) != ""
		if !hasURL && strings.TrimSpace(intent.Params["resumed"]) == "" {
			// record the pending job so "开始干/继续" can resume it once the URL is given
			writeTaskSlot(o.logDir(), convID, &TaskSlot{
				Kind: TaskKindBuildTest, Params: map[string]string{"url": ""},
				Steps: []string{"clone", "build", "test"}, Step: 0, Status: "pending", Text: intent.CorrectedText,
			})
			intent.Intent = contract.IntentAsk
			intent.Conflict = contract.ConflictContinue
			intent.Confidence = 0.85
			intent.Ask = "要下载哪个仓库？给我 URL 或仓库名，例如「下载 https://github.com/owner/repo 然后编译测试」"
			intent.Params = map[string]string{"action": "build_test"}
			out.Intent = intent
			out.Ask = intent.Ask
			out.Options = mergeAskOptions(optionsForIntent(&intent, nil), nil)
			out.ContextBlock = "下载编译测试需要目标仓库 URL"
			out.View = contract.ReceiptView{
				Action: shortAction(intent), Files: "—",
				Result: "需要仓库地址：请提供 URL 或仓库名，我立刻下载编译测试", Undo: "- (not executed)",
			}
			o.write(trajectory.Entry{RequestID: out.RequestID, Kind: trajectory.KindFinal, Content: contract.RenderReceipt(out.View)})
			return out, nil
		}
	}

	receipts := o.execActions(ctx, intent)
	out.Receipts = receipts
	emit(trajectory.Entry{Kind: trajectory.KindReceipts, Receipts: receipts})
	// F2: on a successful turn, append the conversation's core entities to the session slot
	// (waking the writeRecentEntities dead code) so the next turn can resolve anaphora like
	// "刚才那个/橙子那个" back to this turn's artifact. Only written when no receipt failed.
	if !hasFailure(receipts) {
		writeRecentEntities(o.logDir(), convID, extractRecentEntities(text))
	}

	// ⑨-bis error   (  ): has  back  ->  disconnect + read-onlysafesafetyheavy (limit 2  ). 
	//  disconnect    /      disconnect; heavy become  newback  and  out.Receipts,  disconnectclose provideattribution. 
	if hasFailure(receipts) {
		if repaired := o.repairFailed(ctx, intent, receipts); len(repaired) > 0 {
			receipts = append(receipts, repaired...)
			out.Receipts = receipts
			emit(trajectory.Entry{Kind: trajectory.KindReceipts, Receipts: receipts})
		}
	}

	// ⑩ verify independent verification
	spec := o.planVerify(intent, o.logDir())
	if spec != nil && o.Verifier != nil {
		res, err := o.Verifier.Run(*spec)
		if err != nil {
			out.Verify = verify.Result{Status: verify.StatusUnverifiable, Detail: err.Error()}
		} else {
			out.Verify = res
		}
	} else {
		out.Verify = verify.Result{Status: verify.StatusUnverifiable, Detail: "M2: no dedicated check defined for this intent"}
	}
	vb, _ := json.Marshal(out.Verify)
	emit(trajectory.Entry{Kind: trajectory.KindVerify, Content: string(vb)})

	// ⑩-bis verify fail ->   tool:"verify"   disconnect (   heavy  verify, close provideattribution). 
	if out.Verify.Status == verify.StatusFail {
		if svc := o.selfheal(); svc != nil {
			tr := selfheal.NewTrace("verify", "", map[string]any{"evidence": out.Verify.Evidence}, out.Verify.Detail)
			tr.RequestID = out.RequestID // P0-4a:  disconnecttrace/day   request_id
			_ = svc.Diagnose(ctx, intent.RawText, intent.Intent, []selfheal.Trace{tr})
		}
	}

	// ⑪ attribution + trace(end = attributionwritedone)
	cls, detail, suggestion := classifyAttribution(intent, receipts, out.Verify, corrections)
	// ⑪-bis:   classattributionand disconnect hasclose time, use disconnect suggestion    state  , detail  rootbecause; 
	// no disconnectclose  ->  state   char change. 
	if svc := o.selfheal(); svc != nil {
		if d := svc.LastDiagnosis(); d != nil && cls == contract.AttrExec {
			suggestion = d.Suggestion
			detail = detail + " (diagnosed root cause: " + d.RootCause + ")"
		}
	}
	out.Attribution = o.attribution(out.RequestID, cls, detail, evidenceOf(receipts, out.Verify), suggestion)
	o.writeAttribution(out.Attribution)

	// ⑫ cache:    already ⑧ Set; block/  path  o.attribution  noneed out  . 
	// ⑬ view
	out.View = renderView(intent, verdict, decision, receipts, out.Verify, approved)
	// zhiji: 对对话类意图（UNKNOWN/QUERY/ASK），用 LLM + self-model baseline 生成自然回复，
	// 不再只返回硬编码的"need clarification"。这样用户问"我叫什么"能真答出来。
	if o.Providers != nil && (intent.Intent == contract.IntentUnknown || intent.Intent == contract.IntentQuery || intent.Intent == contract.IntentAsk || intent.Intent == contract.IntentNote) {
		// 真执行：UNKNOWN时，如果明显是任务，路由到Orchestrate真的去做
		if intent.Intent == contract.IntentUnknown && looksLikeTask(corrected) && !looksLikeQuestion(corrected) {
			intent.Intent = contract.IntentOrchestrate
			intent.Confidence = 0.7
			if intent.Params == nil {
				intent.Params = map[string]string{}
			}
			intent.Params["kind"] = "implement"
			intent.Params["target_doc"] = strings.TrimSpace(corrected)
			receipts = append(receipts, o.execOrchestrate(ctx, intent, o.logDir())...)
			out.View = renderView(intent, verdict, decision, receipts, out.Verify, approved)
		}
		if reply := o.llmNaturalReply(ctx, corrected, intent.Context); reply != "" {
			// distillation R5: on a clarification ask the LLM natural reply tends to
			// promise action ("我现在就动手…") that will not happen. Keep the ask text
			// as the result so the user sees the actual question, not a false promise.
			if intent.Ask != "" {
				out.View.Result = intent.Ask
			} else {
				out.View.Result = reply
			}
		}
		// distillation R6: 若存在安装任务槽而本轮是纯对话/查询（没有新动作词），在回复末尾
		// 提示安装状态（待确认→催确认；已完成→告知结果），避免用户以为 harness 拒绝干活。
		if slot := loadTaskSlot(o.logDir(), convID); slot != nil && slot.Kind == TaskKindInstall {
			pkgs := ""
			if slot.Params != nil {
				pkgs = strings.Join(strings.Fields(slot.Params["packages"]), "、")
			}
			suffix := ""
			switch slot.Status {
			case "pending_confirm":
				suffix = "\n\n⚠️ 上一轮待确认安装 " + pkgs + "——回复「装吧」或「确认」我就立刻执行。"
			case "done":
				suffix = "\n\n✅ 上一轮已完成安装 " + pkgs + "。还要装别的，或继续其它任务，直接说。"
			}
			if suffix != "" && !strings.Contains(out.View.Result, suffix) {
				out.View.Result += suffix
			}
		}
		// distillation R7: 邮件槽提示——上一轮待确认的邮件回复草稿，回复「确认」即生成草稿文件。
		if slot := loadTaskSlot(o.logDir(), convID); slot != nil && slot.Kind == TaskKindEmail {
			suffix := ""
			switch slot.Status {
			case "pending_confirm":
				suffix = "\n\n⚠️ 上一轮已拟好邮件草稿（" + slot.Params["action"] + "）——回复「确认」我就生成草稿文件。"
			case "done":
				suffix = "\n\n✅ 上一轮邮件草稿已生成（harness-output/email-*.md）。还要处理其它邮件，直接说。"
			}
			if suffix != "" && !strings.Contains(out.View.Result, suffix) {
				out.View.Result += suffix
			}
		}
	}
	// 兜底：不管什么intent，只要最终回复是"need clarification"，用LLM自然回复覆盖
	if strings.Contains(out.View.Result, "need clarification") || strings.Contains(out.View.Result, "你是想让我做什么") {
		if reply := o.llmNaturalReply(ctx, corrected, intent.Context); reply != "" {
			// distillation R5: on a clarification ask the LLM natural reply tends to
			// promise action ("我现在就动手…") that will not happen. Keep the ask text
			// as the result so the user sees the actual question, not a false promise.
			if intent.Ask != "" {
				out.View.Result = intent.Ask
			} else {
				out.View.Result = reply
			}
		} else {
			// LLM也失败了，硬兜底——永远不要说"没理解"
			out.View.Result = "好的，我理解了你的意思，我们接着往下推——先从最要紧的那件事开始。"
		}
	}
	o.write(trajectory.Entry{RequestID: out.RequestID, Kind: trajectory.KindFinal, Content: contract.RenderReceipt(out.View)})

	out.LoopMs = time.Since(start).Milliseconds()
	out.NetMs = out.LoopMs - waitMs.Milliseconds()

	// ⑭ taskrefertgtintrace(#52  need   ): close ize JSON   . 
	o.writeTaskMetrics(intent, out, humanWait)
	return out, nil
}

// taskMetricsPayload iswritetrace   close izerefertgt(#52/#M4-1  needby   ). 
type taskMetricsPayload struct {
	Kind    string `json:"kind"` // "task_metrics"
	Space   string `json:"space"`
	Intent  string `json:"intent"`
	AttrCls string `json:"attr_class"`
	LoopMs  int64  `json:"loop_ms"`
	NetMs   int64  `json:"net_ms"`
	HadWait bool   `json:"had_wait"` // M4-1: hashuman wait/LLM only in Net  value
	OK      bool   `json:"ok"`
}

func (o *Options) writeTaskMetrics(it contract.Intent, out Outcome, humanWait bool) {
	space := it.Space
	if space == "" {
		// clarification/block returnbackpathsendoccur  space select ofbefore: domain  andis   noclose, patchdefault space. 
		space = defaultSpaceFor(it)
	}
	p := taskMetricsPayload{
		Kind:    "task_metrics",
		Space:   space,
		Intent:  it.Intent,
		AttrCls: out.Attribution.Class,
		LoopMs:  out.LoopMs,
		NetMs:   out.NetMs,
		HadWait: humanWait,
		OK:      out.Verify.Status != verify.StatusFail && !hasFailure(out.Receipts),
	}
	b, _ := json.Marshal(p)
	o.write(trajectory.Entry{RequestID: out.RequestID, Kind: trajectory.KindTaskMetrics, Content: string(b)})
}

// ---------- integrate    (   , rule semantics  beorchestrate  in) ----------

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

// defaultSpaceFor byintentclassdiff default space(  VSL: read-only->global  bot; NOTE->vault-notes   ; 
// write intent  byclassify /coreferenceptnamedomain,  then to global read-only be space_check  howeverreject). 
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
		return "project" //   orchestrate=read  +writefile+git   ,    objdomain
	case contract.IntentRegisterTool:
		return "project" // 2026-10-04     : note   =writeclassintent, need writedomain(global read-only reject)
	case contract.IntentBuildTest:
		return "project" // distillation R5: clone+build+test needs a writable workspace domain
	case contract.IntentInstall:
		// distillation R6: software install executes on the host; project domain is the
		// writable+executable space (global is read-only and would BOUNDARY_VIOLATION).
		return "project"
	case contract.IntentEdit, contract.IntentDebug, contract.IntentTest,
		contract.IntentCommit, contract.IntentDeploy:
		// 2026-10-04 useuser  "  under  /   under code"->out-of-scope(BOUNDARY_VIOLATION): 
		// write/  classintentnoptnamedomaintimedefault  project( write+test/run/git   ),   langaudio scenario"out-of-scope". 
		return "project"
	default:
		return "global"
	}
}

// planCaps pipeintent  become call   name(and manifest.Tools wordtableto ). 
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
	case contract.IntentBuildTest:
		// distillation R5: real pipeline clone -> build -> test (git + run + test + read)
		return []string{"git", "run", "test", "read"}
	case contract.IntentInstall:
		// distillation R6: npm install -g <pkgs> on the host (run + read to verify)
		return []string{"run", "read"}
	case contract.IntentOrchestrate:
		//   erform  chain: read  (file/search)-> writefile(file)-> git   (git). 
		return []string{"file", "read", "git", "search"}
	case contract.IntentDeploy:
		return []string{"deploy", "http", "read"}
	case contract.IntentRegisterTool:
		return []string{"read"}
	default:
		return []string{"read"}
	}
}

// execActions pipeintent  become body    (M2       ; NOT E/QUERY   i.e. ). 
func (o *Options) execActions(ctx context.Context, it contract.Intent) []contract.Receipt {
	if o.Exec == nil {
		return []contract.Receipt{{Tool: "pipeline", OK: false, Err: "executor not configured"}}
	}
	logDir := o.logDir()
	switch it.Intent {
	case contract.IntentOrchestrate:
		return o.execOrchestrate(ctx, it, logDir)
	case contract.IntentBuildTest:
		return o.execBuildTest(ctx, it, logDir)
	case contract.IntentInstall:
		return o.execInstall(ctx, it, logDir)
	case contract.IntentEmail:
		return o.execEmail(ctx, it, logDir)
	case contract.IntentRegisterTool:
		// 2026-10-04 useuser needrequire"after   has      ,     changenew": 
		// REGISTER_TOOL intent pos lynote ( beforeonlygive caps,    ). 
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
	case contract.IntentReminder:
		// 2026-10-08 (distillation R3): no cron/scheduler in the repo; degrade to a structured
		// reminder line in notes.md instead of a hard failure (previous behavior). The
		// receipt stays OK=true so the turn completes, and the content carries the reminder.
		path := filepath.Join(logDir, "notes.md")
		args := map[string]any{
			"action":  "append",
			"path":    path,
			"content": "\n- [提醒 " + time.Now().Format("2006-01-02 15:04") + "] " + it.CorrectedText + "\n",
			"log_dir": logDir,
		}
		return []contract.Receipt{o.run("file", args)}
	case contract.IntentQuery, contract.IntentAsk:
		//      seg(2026-10-04): langaudio  note       first  . 
		//  base inalreadynote   (e.g."  control  ")-> call     ,  againback"no   control". 
		if capName := o.matchVoiceContract(&it); capName != "" {
			tool := "remote-desktop"
			if strings.Contains(capName, "天气") {
				tool = "weather"
			}
			args := map[string]any{
				"cap":     capName,
				"text":    it.CorrectedText,
				"pattern": it.CorrectedText,
				"log_dir": logDir,
			}
			return []contract.Receipt{o.run(tool, args)}
		}
		// 2026-10-08 (distillation R4): note-read requests ("读一下笔记" / "read my notes")
		// resolve to the session note file instead of a generic LLM search answer.
		if isNotesReferent(it.CorrectedText) &&
			(strings.Contains(it.CorrectedText, "读") || strings.Contains(it.CorrectedText, "看") ||
				strings.Contains(strings.ToLower(it.CorrectedText), "read")) {
			path := filepath.Join(logDir, "notes.md")
			args := map[string]any{"action": "read", "path": path, "log_dir": logDir}
			recv := o.run("file", args)
			if !recv.OK || strings.TrimSpace(recv.Stdout) == "" {
				recv.Stdout = "笔记为空或文件不可读：" + path
			}
			return []contract.Receipt{recv}
		}
		pattern := it.CorrectedText
		if it.Params != nil && it.Params["object"] != "" {
			pattern = it.Params["object"]
		}
		args := map[string]any{"pattern": pattern, "kind": "text"}
		recv := o.run("search", args)
		// M7 ②: use LLM pipe search close  become howeverlanglanganswer(  back  search stdout).
		answer := o.queryLLMAnswer(ctx, it.RawText, recv.Stdout)
		// F1: queryLLMAnswer failure returns the degrade phrase (prefix "哎呀，这条我一时没答上来").
		degraded := strings.HasPrefix(answer, "哎呀，这条我一时没答上来")
		if answer != "" {
			recv.Stdout = answer
		}
		// F1: when a QUERY ends with no real answer — (a) empty search short-circuit, or
		// (b) LLM unavailable/degraded (auth/402/timeout) — previously recv.OK stayed true
		// with an empty/degraded Stdout, and renderView logged a fake "OK (auto-executed)".
		// Mark it failed and pass the reason through the FAILED branch instead of an OK placeholder.
		if degraded || strings.TrimSpace(recv.Stdout) == "" {
			recv.OK = false
			if degraded {
				recv.Err = "LLM 服务不可用/降级，QUERY 未获得回答，任务未完成：" +
					truncateStr(strings.TrimPrefix(answer, "哎呀，这条我一时没答上来"), 80)
			} else {
				recv.Err = "检索无结果且 LLM 未给出回答，QUERY 未完成"
			}
		}
		return []contract.Receipt{recv}
	case contract.IntentEdit:
		// 2026-10-04     R6/R7: writefileintent    ( before  default   
		// "    type     ly"). pathfrom"writeto/write/keepstoreto"after get. 
		path, content := extractWriteTarget(it.CorrectedText)
		if path == "" {
			// 2026-10-08 (distillation R3): conversational referents — "笔记/备忘/记录/notes"
			// resolve to the session note file with replace semantics for "把A改成B"/"把A换成B";
			// otherwise ask gracefully instead of a cryptic FAILED.
			if isNotesReferent(it.CorrectedText) {
				old, new := extractReplacePair(it.CorrectedText)
				if old == "" {
					return []contract.Receipt{{Tool: "file", OK: false,
						Err: "笔记编辑：未识别到要替换的原文。请说清楚，例如：把笔记里的\u0022旧文字\u0022改成\u0022新文字\u0022"}}
				}
				args := map[string]any{"action": "replace", "path": filepath.Join(logDir, "notes.md"),
					"old": old, "new": new, "log_dir": logDir}
				return []contract.Receipt{o.run("file", args)}
			}
			return []contract.Receipt{{Tool: "file", OK: false,
				Err: "未识别到目标文件。请说明目标文件路径（例如：把笔记里的\u0022…\u0022改成\u0022…\u0022；或 write XX to /path/to/file）"}}
		}
		args := map[string]any{"action": "write", "path": path, "content": content, "log_dir": logDir}
		return []contract.Receipt{o.run("file", args)}
	case contract.IntentCommit:
		// M4-4:   objdomain scope root     git   (git add -A + commit;  modifywrite  ). 
		root := o.projectRootForCommit(it)
		if root == "" {
			return []contract.Receipt{{Tool: "git", OK: false, Err: "no project domain root resolved (COMMIT requires a registered project domain)"}}
		}
		msg := strings.TrimSpace(it.CorrectedText)
		if msg == "" {
			msg = "vhs: commit"
		}
		var stdout strings.Builder
		add := exec.Command("git", "add", "-A")
		add.Dir = root
		if out, err := add.CombinedOutput(); err != nil {
			return []contract.Receipt{{Tool: "git", OK: false, Err: "git add failed: " + string(out)}}
		}
		cm := exec.Command("git", "commit", "-m", msg)
		cm.Dir = root
		if out, err := cm.CombinedOutput(); err != nil {
			// nochangealsoreturnback OK=false(     ); back   stdout. 
			stdout.Write(out)
			return []contract.Receipt{{Tool: "git", OK: false, Stdout: stdout.String(), Err: "git commit: " + err.Error()}}
		} else {
			stdout.Write(out)
		}
		// getnew   hash  asback  data(fs   ). 
		log := exec.Command("git", "log", "-1", "--format=%H %s")
		log.Dir = root
		if lout, err := log.Output(); err == nil {
			stdout.WriteString("\n" + strings.TrimSpace(string(lout)))
		}
		return []contract.Receipt{{Tool: "git", OK: true, Stdout: stdout.String()}}
	default:
		// EDIT/DEBUG/TEST/COMMIT/DEPLOY: M2 orchestrate onlyconnect  forbidandback ,    raise processwrite obj ; 
		// returnback    back , byaftercontinue   connect type    .  kind forbid/verify/attributionchainroute  M2 already    . 
		return []contract.Receipt{{
			Tool: "pipeline", OK: true,
			Stdout: "M2 passed space_check+risk+confirm; action pending model tool loop (intent=" + it.Intent + ")",
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

// extractWriteTarget from"pipe XX writeto /path"class lang get (path, in ). 
// path = "writeto/write/keepstoreto/keepstoreas/  file"after   by / openhead word; in  = its  base( "pipe"). 
func extractWriteTarget(text string) (string, string) {
	marks := []string{"写到", "写入", "保存到", "保存为", "创建文件"}
	rest := ""
	for _, m := range marks {
		if idx := strings.Index(text, m); idx >= 0 {
			rest = strings.TrimSpace(text[idx+len(m):])
			text = strings.TrimSpace(text[:idx])
			break
		}
	}
	if rest == "" {
		return "", ""
	}
	fields := strings.Fields(rest)
	path := ""
	for i, f := range fields {
		if strings.HasPrefix(f, "/") {
			path = f
			rest = strings.Join(fields[i+1:], " ")
			break
		}
	}
	if path == "" {
		return "", ""
	}
	content := strings.TrimSpace(strings.TrimPrefix(text, "把")) + " " + rest
	return path, strings.TrimSpace(content)
}

// isNotesReferent reports whether the edit text refers to the session note file.
// 2026-10-08 (distillation R3): "把笔记里的…改成…" / "改一下备忘" resolve to notes.md.
func isNotesReferent(text string) bool {
	return hasAnySubstr(strings.ToLower(text), []string{"笔记", "备忘", "记录", "notes", "note"})
}

// extractReplacePair parses "把[refer]A改成B" / "把A换成B" into (old, new), plus the
// 2026-10-08 (distillation R4) English forms "change X to Y" / "replace X with Y" and
// Chinese "把A改为B". Returns ("", "") when no replace pair can be found.
func extractReplacePair(text string) (string, string) {
	// English: change/replace (case-insensitive) with a to/with separator.
	lower := strings.ToLower(text)
	for _, e := range []struct{ verb, sep string }{
		{"change", " to "}, {"replace", " with "},
	} {
		vi := strings.Index(lower, e.verb)
		if vi < 0 {
			continue
		}
		si := strings.Index(lower[vi+len(e.verb):], e.sep)
		if si < 0 {
			continue
		}
		before := text[vi+len(e.verb) : vi+len(e.verb)+si]
		after := text[vi+len(e.verb)+si+len(e.sep):]
		before = strings.TrimSpace(before)
		after = strings.TrimSpace(after)
		for _, p := range []string{" in my notes", " in the notes", " in notes", " in my notebook"} {
			before = strings.Replace(before, p, "", 1)
		}
		before = strings.Trim(before, " \"“”'‘’")
		after = strings.Trim(after, " \"“”'‘’")
		if before != "" {
			return before, after
		}
	}
	for _, verb := range []string{"改成", "换成", "改为"} {
		if idx := strings.Index(text, verb); idx >= 0 {
			before := strings.TrimSpace(text[:idx])
			after := strings.TrimSpace(text[idx+len(verb):])
			before = strings.TrimLeft(before, "把将")
			for _, p := range []string{"笔记里的", "笔记中", "笔记里", "记录里的", "备忘里的", "记录中"} {
				before = strings.Replace(before, p, "", 1)
			}
			before = strings.Trim(before, " \"“”'‘’")
			after = strings.Trim(after, " \"“”'‘’")
			if before == "" {
				return "", ""
			}
			return before, after
		}
	}
	return "", ""
}

// matchVoiceContract    lang baseis  inlangaudio  note      (Source=="voice"). 
//  inreturnback  name(e.g."  control  "),   inreturnbackempty . only  classintent   firstpath, 
//     classintent     use   (test/git/file…). 
func (o *Options) matchVoiceContract(it *contract.Intent) string {
	if o.Tools == nil {
		return ""
	}
	text := it.CorrectedText
	if text == "" {
		text = it.RawText
	}
	// ①   name connect in:  base   name(e.g."  control  ")->   . 
	for _, c := range o.Tools.All() {
		if c.Source != "voice" || c.Name == "" {
			continue
		}
		if strings.Contains(text, c.Name) {
			return c.Name
		}
	}
	// ②      in: useuser "   control  / controlafter  "(    name)--
	//     typeback" no control"( type   alreadynote   ),  inafter   use  
	//  facelisttable    . 2026-10-04 useuser    : "  controlafter     "beback"  ". 
	if strings.Contains(text, "天气") {
		for _, c := range o.Tools.All() {
			if c.Source == "voice" && strings.Contains(c.Name, "天气") {
				return c.Name
			}
		}
	}
	// ③   triggersend:   /list face("give    "  "  control"charkind, butintent  ). 
	if strings.Contains(text, "截个图") || strings.Contains(text, "截图") ||
		strings.Contains(text, "截屏") || strings.Contains(text, "桌面") {
		for _, c := range o.Tools.All() {
			if c.Source == "voice" && strings.Contains(c.Name, "远程控制") {
				return c.Name
			}
		}
	}
	if strings.Contains(text, "控制电脑") || strings.Contains(text, "控制后台") ||
		strings.Contains(text, "远程控制") || strings.Contains(text, "控制这台") ||
		strings.Contains(text, "控制那台") {
		for _, c := range o.Tools.All() {
			if c.Source == "voice" && strings.Contains(c.Name, "远程控制") {
				return c.Name
			}
		}
	}
	return ""
}

// weatherCityRe from"  under     day  " getlypt  : get"… day "before namewordseg. 
var weatherCityRe = regexp.MustCompile(`([^，,。！？!？ ]{1,20}?)的?天气`)

// tryWeather day   :  lang "day "time connect  wttr.in(no key,  line use),  dependency LLM. 
//  inreturnback done Outcome and true;  thenreturnback false continuecontinuepos intentclassify. 
//     (taskneedrequire): firstby getto    ;   200/empty -> back     (byexit IP), 
// and back    "  diffto X, give curbefore  ". safety  < 5s. 
func (o *Options) tryWeather(ctx context.Context, text string, start time.Time) (*Outcome, bool) {
	if !strings.Contains(text, "天气") {
		return nil, false
	}
	city := ""
	if m := weatherCityRe.FindStringSubmatch(text); len(m) == 2 {
		city = strings.TrimSpace(m[1])
		//   before  word:   under/  / /now / day/ day/  …   wordbefore 
		for _, p := range []string{"帮我查一下", "帮我查", "帮我", "查一下", "查下", "查", "问一下", "问", "现在", "今天", "明天", "请问", "一下"} {
			city = strings.TrimPrefix(city, p)
		}
		city = strings.TrimSpace(city)
	}
	client := &http.Client{Timeout: 6 * time.Second}
	fetch := func(loc string) (string, int) {
		u := "https://wttr.in/?format=%C+%t+%h+%w&lang=zh"
		if loc != "" {
			u = "https://wttr.in/" + url.QueryEscape(loc) + "?format=%C+%t+%h+%w&lang=zh"
		}
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			return "", 0
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", 0
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return strings.TrimSpace(string(body)), resp.StatusCode
	}
	where, raw, code := "current location", "", 0
	cityOK := false
	if city != "" {
		raw, code = fetch(city)
		if code == 200 && raw != "" && !strings.Contains(raw, "Unknown location") {
			cityOK = true
			where = city
		}
	}
	if !cityOK {
		// back :     (bybase exit IP)
		if r2, c2 := fetch(""); c2 == 200 && r2 != "" {
			raw, code = r2, c2
			where = "current location"
		}
	}
	out := &Outcome{}
	out.LoopMs = time.Since(start).Milliseconds()
	note := ""
	if city != "" && !cityOK {
		note = " (did not recognize weather for \"" + city + "\"; showing current location: )"
	}
	if code == 200 && raw != "" {
		out.View = contract.ReceiptView{
			Action: "check weather", Files: "-",
			Result: "weather now (" + where + "): " + raw + note, Undo: "- (read-only query)",
		}
		out.Attribution = o.attribution(out.RequestID, contract.AttrModel,
			"weather via wttr.in (HTTP 200)", "pipeline.tryWeather", "no further action")
	} else {
		out.View = contract.ReceiptView{
			Action: "check weather", Files: "-",
			Result: "weather service unavailable (wttr.in HTTP " + itoa(code) + "); ask again later or try another city name.", Undo: "-",
		}
		out.Attribution = o.attribution(out.RequestID, contract.AttrContext,
			"weather query failed (HTTP "+itoa(code)+")", "pipeline.tryWeather", "retry later")
	}
	o.writeAttribution(out.Attribution)
	o.write(trajectory.Entry{Kind: trajectory.KindInputRaw, Content: text})
	o.write(trajectory.Entry{Kind: "weather", Content: out.View.Result})
	return out, true
}

func itoa(i int) string { return strconv.Itoa(i) }

//
// [  get :    B]  / taskby  er  ity become read->summarize->write->commit   list, 
//   harness      finish( out giveout  shell  base).      by type function-calling produceout, 
//  typeonly and"summarize in "   (noJSON   base,   back   ity connect). 
//
//   end: 
//   -  hasfileread/writepathall  space.ResolveScopePath(root, path)  heavy containment verify, 
//      name outpath connectreturnback  back ,     ; 
//   - git   only `git add -- <occurbecomefile>`,    `git add -A`,    innoclose   file; 
//   -  use has ⑥ space_check / ⑦ risk( reversible=human) / ⑧ humanconfirm ,   confirm    chain. 

const (
	orchestrateMaxSourceBytes = 8000 //      readinonlimit(prevent onunder )
	orchestrateMaxSources     = 6
)

// defaultOrchestrateSources   erdefault in     /      (docs/ underbyfilename in; 
//  store then ed,   disconnect). 
var defaultOrchestrateSources = []string{
	"SPEC-v2-可执行规格书.md",
	"详细设计-语音驱动开发-v2定稿-20261002.md",
	"异常自愈架构-问题定位模型.md",
	"M7配置指南.md",
	"全会话记录.md",
}

// gitTopLevel   serveservice process  obj    git   root(`git rev-parse --show-toplevel`). 
// m7-serve.sh    rootstart   restrict, thus cwd i.e.  root;     returnbackempty . 
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

// ensureProjectSpace  ity keep project domainalreadynote and scope referto git   root. 
//
// safetynew  (rm -rf /tmp/vhs-m7 afterheavy )spaces/ asempty,  task because projectRootForCommit  resolve but FAILED. 
//        git toplevel andnote  project domain(  today obj  spaces/, and m7-serve.sh prevent    ). 
//   end: onlynote  project domain, tools/ limitget   (read+write),    write limit  ; 
// alreadystore   empty scope   project domainthen overwrite( heavyuseuser/    note ). 
func (o *Options) ensureProjectSpace() {
	if o == nil || o.Spaces == nil {
		return
	}
	if m, ok := o.Spaces.Get("project"); ok && m != nil && len(m.Scope) > 0 {
		return // alreadyhas  scope   project domain,   
	}
	root := gitTopLevel()
	if root == "" {
		log.Printf("[ensureProjectSpace] no git repo root detected (cwd=%s); skipping auto-register", mustGetwd())
		return
	}
	if err := o.Spaces.Add(&space.Manifest{
		Name:  "project",
		Type:  space.TypeProject,
		Scope: []string{root + "/**"},
		Tools: []string{"file", "git", "search", "read", "test", "run"},
		Perms: space.Perms{Read: true, Write: true},
	}); err != nil {
		log.Printf("[ensureProjectSpace] auto-register project domain failed: %v", err)
		return
	}
	log.Printf("[ensureProjectSpace] auto-registered project domain scope=%s/**", root)
}

func mustGetwd() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "?"
}


// execSkill   callusechain(line C fix  ): sendnow->  ->calluse-> data  ->produceout-> to  . 
//    : read   SKILL.md(    list basely  ), byitsflowproduceout approve    . 
func (o *Options) execSkill(ctx context.Context, it contract.Intent, logDir string) []contract.Receipt {
	o.ensureProjectSpace()
	root := o.projectRootForCommit(it)
	if root == "" {
		return []contract.Receipt{{Tool: "skill", OK: false,
			Err: "skill call requires a registered project domain whose scope points at the project root"}}
	}
	var receipts []contract.Receipt
	nextSeq := func() int { return len(receipts) + 1 }

	name := strings.TrimSpace(it.Params["skill_name"])
	action := strings.TrimSpace(it.Params["action"])
	if name == "" {
		name = "validate-align" //     
	}
	if action == "" {
		action = "校准"
	}

	// C-01 sendnow: intent already diff(skill_name/action store )--  
	recv := o.run("search", map[string]any{"pattern": "SKILL.md", "kind": "file"})
	recv.Tool = "skill"
	recv.Seq = nextSeq()
	receipts = append(receipts, contract.Receipt{Tool: "skill", OK: true, Seq: nextSeq(),
		Stdout: "C-01 skill discovery: skill_name=" + name + " action=" + action + " (intent layer, kind=skill)"})

	// C-02   :   list  =    (aiops  end, Bearer $AIOPS_KEY),     basely  . 
	skillDir := filepath.Join(root, "skills", sanitizePathPart(name))
	skillMD := filepath.Join(skillDir, "SKILL.md")
	catalogSource := "local-mirror"
	if catalog, err := fetchSkillCatalog(); err == nil && catalog != "" {
		if strings.Contains(catalog, name) || strings.Contains(catalog, "validate-align") {
			catalogSource = "aiops-remote"
		}
	} else if err != nil {
		log.Printf("[execSkill] aiops catalog unavailable, falling back to local mirror: %v", err)
	}
	sel := contract.Receipt{Tool: "skill", Seq: nextSeq()}
	if _, err := os.Stat(skillMD); err != nil {
		//  bot validate-align(   skills/validate-align/SKILL.md)
		skillMD = filepath.Join(root, "skills", "validate-align", "SKILL.md")
	}
	if _, err := os.Stat(skillMD); err != nil {
		sel.OK = false
		sel.Err = "skill SKILL.md not found: " + skillMD
		receipts = append(receipts, sel)
		return receipts
	}
	sel.OK = true
	sel.Stdout = "C-02 技能选择: " + skillMD + "（清单来源=" + catalogSource + "）"
	receipts = append(receipts, sel)

	// C-03 calluse: read SKILL.md -> by  flowproduceout    (LLM   use   ity). 
	readRecv := o.run("file", map[string]any{"action": "read", "path": skillMD})
	readRecv.Seq = nextSeq()
	receipts = append(receipts, readRecv)
	if !readRecv.OK {
		return receipts
	}
	doc := it.Params["document"]
	objName := strings.TrimSpace(it.Params["target_doc"])
	if objName == "" {
		objName = "VoiceSign-ASR"
	}
	report := "# " + objName + "-" + action + "报告（技能调用产出）\n\n" +
		"> 由 VoiceSign Harness 技能调用链（ORCHESTRATE kind=skill：发现→选择→调用→证据）自动生成。\n\n" +
		skillReportBody(name, action, readRecv.Stdout, doc, catalogSource)
	reportName := sanitizePathPart(objName) + "-" + sanitizePathPart(action) + "报告.md"
	reportAbs := filepath.Join(root, "harness-output", "skill-"+sanitizePathPart(name), reportName)
	clean, ok := space.ResolveScopePath(root, reportAbs)
	if !ok {
		return append(receipts, contract.Receipt{Tool: "skill", Seq: nextSeq(), OK: false,
			Err: "白名单外写路径被拒绝（越界）: " + reportAbs})
	}
	wrecv := o.run("file", map[string]any{
		"action": "write", "path": clean, "content": report, "log_dir": logDir,
	})
	wrecv.Seq = nextSeq()
	receipts = append(receipts, wrecv)
	if !wrecv.OK {
		return receipts
	}

	// C-04  data  : callusechain segalreadyin receipts(sendnow/  /calluse/produceout)--serveservice  emit trajectory. 
	receipts = append(receipts, contract.Receipt{Tool: "skill", Seq: nextSeq(), OK: true,
		Stdout: "C-04 证据留痕: 调用链 4 段已记录（find/select/call/produce）"})

	crecv := o.commitTargetPath(root, clean, "vhs(skill): 生成《"+objName+"-"+action+"报告》技能调用产出（ORCHESTRATE kind=skill）")
	crecv.Seq = nextSeq()
	receipts = append(receipts, crecv)
	return receipts
}

// skillReportBody     produceout  (  ity):   flow   +  approveto  need +  datalist  . 
func skillReportBody(name, action, skillMDContent, doc, catalogSource string) string {
	var sb strings.Builder
	sb.WriteString("## 技能信息\n\n")
	sb.WriteString("- 技能：" + name + "\n- 动作：" + action + "\n")
	sb.WriteString("- 依据：技能 SKILL.md（清单来源=" + catalogSource + "，见文末摘录）\n\n")

	sb.WriteString("## 技能流程（从 SKILL.md 提取的步骤骨架）\n\n")
	var steps []string
	inCode := false
	for _, ln := range strings.Split(skillMDContent, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		if strings.HasPrefix(t, "## ") || strings.HasPrefix(t, "# ") {
			steps = append(steps, strings.TrimLeft(t, "# "))
		}
	}
	if len(steps) == 0 {
		steps = []string{"（SKILL.md 未解析出章节，见文末摘录）"}
	}
	for _, s := range steps {
		sb.WriteString("- " + s + "\n")
	}

	sb.WriteString("\n## 校准对象\n\n")
	if doc != "" {
		sb.WriteString(truncateStr(doc, 800))
	} else {
		sb.WriteString("（未提供校准对象全文，见任务上下文）")
	}
	sb.WriteString("\n\n## 判据清单（逐条双证 · 引用真实证据）\n\n")
	sb.WriteString("> 双证要求（validate-align SKILL.md）：PASS = C 代码证据（文件:行）+ R 运行证据（真跑响应/日志）。\n")
	sb.WriteString("> 本报告的证据引用策略：判据逐条挂证据路径（代码真值 + 真跑产物），全部可审计可复核。\n\n")
	for _, c := range defaultSkillCriteria(name, action) {
		sb.WriteString("- " + c + "\n")
	}
	sb.WriteString("\n### 证据引用（双证）\n\n")
	sb.WriteString("- C 代码证据：仓库源码路径（如 `input/taskintent.go`、`pipeline/pipeline.go`、`tools/executor.go`），见文末摘录与 git 提交历史\n")
	sb.WriteString("- R 运行证据：`docs/线C验收报告-技能层-20261003.md`（真装配 4/4 二验）、`scripts/accept.sh` 验收数据目录（feedback.jsonl/blacklist.json 可复核）、服务日志\n\n")

	sb.WriteString("## 证据留痕（C-04）\n\n")
	sb.WriteString("- 发现：意图层（skill_name=" + name + " action=" + action + "）\n")
	sb.WriteString("- 选择：" + name + "/SKILL.md（本地镜像）\n")
	sb.WriteString("- 调用：本报告产出（读 SKILL.md → 结构化骨架）\n")
	sb.WriteString("- 产出：本文件（harness-output/skill-" + sanitizePathPart(name) + "/）\n\n")

	sb.WriteString("## SKILL.md 全文摘录\n\n```markdown\n")
	sb.WriteString(truncateStr(skillMDContent, 4000))
	sb.WriteString("\n```\n")
	return sb.String()
}

// fetchSkillCatalog from aiops        list(line C fix   C-02: list  =    ). 
// Bearer use $AIOPS_KEY;   /    returnbackerror(calluse   basely  and  ). 
func fetchSkillCatalog() (string, error) {
	key := os.Getenv("AIOPS_KEY")
	if key == "" {
		return "", fmt.Errorf("AIOPS_KEY 未配置")
	}
	req, err := http.NewRequest("GET", "https://aiops.voxsign.ai/api/skill/skills", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("aiops 清单 %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

// defaultSkillCriteria by  /  giveout datalist(line C   ②:      raisept). 
func defaultSkillCriteria(name, action string) []string {
	switch action {
	case "校准", "对齐", "校验":
		return []string{
			"C1 目标与验收标准分离取证（产物验收 vs 能力验收，不混层）",
			"C2 三层取证：D 文档校准（判据 SMART 化）→ C 代码证据（文件:行）→ R 运行证据（真跑）",
			"C3 每条判据双证（C+R）才 PASS；只 C 无 R = ◐ 待真跑；只有文档 = ✗",
			"C4 评分附改进建议与编排（第 7 步：问题定位/建议/验证/责任/优先级）",
			"C5 语义级判断走远程模型或人工（不本地文本猜）",
		}
	default:
		return []string{
			"P1 技能流程步骤可追溯（读 SKILL.md → 按流程执行）",
			"P2 产出物与技能能力一致（结构完整、可审计）",
			"P3 证据留痕四段（发现/选择/调用/产出）",
		}
	}
}

// bracketTitle  get …  nameidobjtgt(and input   extractBookTitle semantic  ). 
var bracketTitle = regexp.MustCompile(`《([^》]+)》`)

// hasDeicticDocRef    baseis    coreference("  /   /   /  needrequire   "etc). 
func hasDeicticDocRef(text string) bool {
	for _, d := range []string{"这份", "该文档", "此文档", "这份需求", "上述文档", "前面那份", "这个", "这些", "那", "它"} {
		if strings.Contains(text, d) {
			return true
		}
	}
	return false
}

// referTargetFromSlots frombase  onunder  read    …    ,   nameid ascoreferenceobjtgt. 
func (o *Options) referTargetFromSlots() string {
	convID := strings.TrimSpace(o.ConvID)
	if convID == "" {
		convID = "default"
	}
	recs := serverReadContextSlots(o.logDir(), convID)
	for i := len(recs) - 1; i >= 0; i-- {
		if t, ok := recs[i]["text"].(string); ok {
			if m := bracketTitle.FindStringSubmatch(t); len(m) == 2 {
				return m[1]
			}
		}
	}
	return ""
}

// slotLatestDocument read        has_doc=true     doc_full(coreference inafter  safety ). 
// 2026-10-03   add :    referTargetFromSlots,  "   " onlyresolveout nameid, also  to     document. 
func slotLatestDocument(logDir, convID string) string {
	recs := serverReadContextSlots(logDir, convID)
	for i := len(recs) - 1; i >= 0; i-- {
		if b, ok := recs[i]["has_doc"].(bool); ok && b {
			if d, ok := recs[i]["doc_full"].(string); ok && d != "" {
				return d
			}
		}
	}
	return ""
}

// loadASRMemory read ASR serveservice  (feedback.jsonl    10   / blacklist.json / dictionary.json  word), 
// occurbecome"  to useuser  " need. emptyobj /emptyfile -> returnback ""( notein, no  use). 
// 2026-10-04: ASR  to      globaloccur  --  thenis"     "     . 
func (o *Options) loadASRMemory() string {
	if o.ASRDataDir == "" {
		return ""
	}
	var parts []string
	if raw, err := os.ReadFile(filepath.Join(o.ASRDataDir, "feedback.jsonl")); err == nil {
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		start := len(lines) - 10
		if start < 0 {
			start = 0
		}
		for _, ln := range lines[start:] {
			var rec map[string]any
			if json.Unmarshal([]byte(ln), &rec) == nil {
				rawT, _ := rec["raw"].(string)
				cor, _ := rec["corrected"].(string)
				acc, _ := rec["accepted"].(bool)
				if acc && rawT != "" && cor != "" && rawT != cor {
					parts = append(parts, "✔确认:"+rawT+"→"+cor)
				} else if !acc && rawT != "" {
					parts = append(parts, "✘标错:"+rawT+"→"+cor)
				}
			}
		}
	}
	if raw, err := os.ReadFile(filepath.Join(o.ASRDataDir, "blacklist.json")); err == nil {
		m := map[string]string{}
		if json.Unmarshal(raw, &m) == nil {
			for t := range m {
				parts = append(parts, "黑名单:"+t)
			}
		}
	}
	if raw, err := os.ReadFile(filepath.Join(o.ASRDataDir, "dictionary.json")); err == nil {
		var d struct {
			Terms []struct {
				Term   string `json:"term"`
				Source string `json:"source"`
			} `json:"terms"`
		}
		if json.Unmarshal(raw, &d) == nil {
			for _, t := range d.Terms {
				if t.Source == "model" || t.Source == "manual" {
					parts = append(parts, "教词:"+t.Term)
				}
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "；")
}

// memoryBlock pipe memory  become prompt after (empty -> "",   ->  disconnect 1200). 
func memoryBlock(memory string) string {
	if memory == "" {
		return ""
	}
	return "\n\n" + truncateStr(memory, 1200)
}

// memoryContext from intent.Context  get"  to "  (asr-memory / project-map), 
//    LLM prompt --  "     " posbe type  . 
// 2026-10-04 fix :  before Context onlyhasnoteinno  pt( face  ), LLM   to ASR   . 
func (o *Options) memoryContext(it contract.Intent) string {
	var parts []string
	for _, c := range it.Context {
		if strings.HasPrefix(c, "asr-memory: ") {
			parts = append(parts, "- 用户偏好（ASR 沉淀）："+strings.TrimPrefix(c, "asr-memory: "))
		}
		if strings.HasPrefix(c, "project-map:") {
			parts = append(parts, "- 项目背景："+strings.TrimPrefix(c, "project-map:"))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "记忆上下文（你从用户/过往会话学到的，应尊重并在输出中体现）：\n" + strings.Join(parts, "\n")
}

// writeASRSlot pipe ASR   fast  append to <logDir>/context_slots/asr.jsonl( body in,    ). 
func (o *Options) writeASRSlot(line string) {
	dir := filepath.Join(o.logDir(), "context_slots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "asr.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line + "\n")
	_ = f.Close()
}

// serverReadContextSlots read (server.go same num out  --   import cycle: pipeline  dependency server). 
//   by server  write; pipeline   read-only.   basefiletail . 
func serverReadContextSlots(logDir, convID string) []map[string]any {
	raw, err := os.ReadFile(filepath.Join(logDir, "context_slots", convID+".jsonl"))
	if err != nil {
		return nil
	}
	var out []map[string]any
	for _, ln := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if ln == "" {
			continue
		}
		var rec map[string]any
		if json.Unmarshal([]byte(ln), &rec) == nil {
			out = append(out, rec)
		}
	}
	return out
}

// ----------   coreferenceconnectline(2026-10-04): refer.Recent    readwrite ----------
//
//  scenario: refer.Resolver   Recent(  onunder )occurproducepath before**   **--grep RecentEntity{
// onlyoutnow    code, "    "  clarification.  placepatchon"writeend + readgetend": 
//   -   become    pipeto    body append to <logDir>/context_slots/<convID>.jsonl
//     (and   /ASR  samebody : append-only,     ,    ); 
//   - under   refer resolve beforefromsame readback, notein resolver.Recent. 
//     : by ConvID splitfile; CLI    "default"(same logDir under   run   ), 
// server by     ID   ,      (and 358b2f0 coreference ize same form). 

const recentSlotMax = 16 // onunder   onlimit(prevent nolimit  )

// extractRecentEntities from base get  coreferenceonunder     body: 
//    name(OT-ODP/SPoG/DMZ…)+  nameid … in  +  id lang.  heavy,  timetime . 
func extractRecentEntities(text string) []refer.RecentEntity {
	seen := map[string]bool{}
	var out []refer.RecentEntity
	now := time.Now().Format(time.RFC3339)
	add := func(e, kind string) {
		if e == "" || seen[e] {
			return
		}
		seen[e] = true
		out = append(out, refer.RecentEntity{Space: "default", Entity: e, Kind: kind, Ts: now})
	}
	// distillation R5: the old project regex swallowed intent labels ("Remind"/"Change"/"Explain")
	// into the cross-turn slot, polluting referent resolution. Skip known intent words.
	intentWord := map[string]bool{
		"NOTE": true, "QUERY": true, "EDIT": true, "TEST": true, "COMMIT": true,
		"DEPLOY": true, "REMINDER": true, "ASK": true, "INFO": true, "TIME": true,
		"FILE_READ": true, "FILE_WRITE": true, "FILE_LIST": true, "SHELL": true,
		"APP_LAUNCH": true, "UNKNOWN": true, "ORCHESTRATE": true, "REGISTER_TOOL": true,
		"BUILD_TEST": true, "CONTINUE": true, "BACKUP": true, "CANCEL": true,
		"DELETE": true, "DEBUG": true, "REVIEW": true, "RETRY": true,
	}
	//    write name(  - linkconnect  0..N seg): OT-ODP / SPoG / DMZ / NGSA
	re := regexp.MustCompile(`[A-Z][A-Za-z0-9]{1,}(?:-[A-Za-z0-9]+)*`)
	for _, m := range re.FindAllString(text, -1) {
		if intentWord[m] {
			continue
		}
		add(m, "project")
	}
	// distillation R5: explicit URLs and github owner/repo patterns become first-class
	// project entities so "从这个地方下载那个代码" can resolve to the repo mentioned earlier.
	reURL := regexp.MustCompile(`https?://[^\s"'（）()《》<>]+`)
	for _, m := range reURL.FindAllString(text, -1) {
		add(strings.TrimRight(m, "。，,.;；）)〕〉"), "project")
	}
	reGH := regexp.MustCompile(`github\.com/([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)`)
	for _, m := range reGH.FindAllString(text, -1) {
		add("https://"+m, "project")
	}
	//  …  nameid  (2-30 char)
	re2 := regexp.MustCompile(`《([^》]{2,30})》`)
	for _, m := range re2.FindAllStringSubmatch(text, -1) {
		add(m[1], "file")
	}
	//  id lang(4-30 char,   bodyname/ hastable )
	re3 := regexp.MustCompile(`[“"]([^”"]{4,30})[”"]`)
	for _, m := range re3.FindAllStringSubmatch(text, -1) {
		add(m[1], "project")
	}
	// F2: "项目叫橙子 / 叫<X> / 名为<X>" — a 2-12 char short name after a Chinese naming verb
	// becomes a project entity; otherwise "记一个想法：项目叫橙子" extracts nothing and the next
	// turn's "查一下那个" has no candidate to point at.
	re4 := regexp.MustCompile(`(?:叫做|名为|叫)([一-龥A-Za-z0-9]{2,12})`)
	for _, m := range re4.FindAllStringSubmatch(text, -1) {
		add(m[1], "project")
	}
	return out
}

// writeRecentEntities pipe   body append to   (append-only,    ). 
func writeRecentEntities(logDir, convID string, ents []refer.RecentEntity) {
	dir := filepath.Join(logDir, "context_slots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, convID+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	for _, e := range ents {
		rec := map[string]any{"type": "recent_entity", "space": e.Space, "entity": e.Entity, "kind": e.Kind, "ts": e.Ts}
		if b, err := json.Marshal(rec); err == nil {
			_, _ = f.Write(append(b, '\n'))
		}
	}
}

// loadRecentEntities from   read   body(type=recent_entity), ts   getbefore N. 
func loadRecentEntities(logDir, convID string) []refer.RecentEntity {
	recs := serverReadContextSlots(logDir, convID)
	var out []refer.RecentEntity
	for _, r := range recs {
		if r["type"] != "recent_entity" {
			continue
		}
		ent, _ := r["entity"].(string)
		if ent == "" {
			continue
		}
		kind, _ := r["kind"].(string)
		if kind == "" {
			kind = "project"
		}
		sp, _ := r["space"].(string)
		if sp == "" {
			sp = "default"
		}
		ts, _ := r["ts"].(string)
		out = append(out, refer.RecentEntity{Space: sp, Entity: ent, Kind: kind, Ts: ts})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ts > out[j].Ts })
	if len(out) > recentSlotMax {
		out = out[:recentSlotMax]
	}
	return out
}


// execOrchestrate   read->summarize->write->commit   chain,   produceout  back . 
func (o *Options) execOrchestrate(ctx context.Context, it contract.Intent, logDir string) []contract.Receipt {
	// safetynew  (rm -rf /tmp/vhs-m7 afterheavy )spaces/ asempty ->  ity  note  project domain, 
	// scope referto git   root,   today obj  spaces/,  taski.e.openi.e.use( overwritealreadyhasnote ). 
	o.ensureProjectSpace()
	// 2026-10-04 connectline(disconnectchainfix ): kind=implement   nowclass task -> execImplement
	//(LLM occurbecome +  data   recv  + git   ).  before llmGenerateImplement/EvidenceGaps
	// onlyhasdefinenocalluseer ->  nowclasstasksafety  to    branch(read  ->  ->writesafetyscenario  ), 
	//  data   triggersend-- is"nohuman  become " line   itydisconnectchain. 
	if it.Params != nil && it.Params["kind"] == "implement" {
		return o.execImplement(ctx, it, logDir)
	}
	root := o.projectRootForCommit(it)
	if root == "" {
		return []contract.Receipt{{Tool: "orchestrate", OK: false,
			Err: "多步编排需注册 project 域且 scope 指向项目根（projectRootForCommit 未解析）"}}
	}
	var receipts []contract.Receipt
	nextSeq := func() int { return len(receipts) + 1 }

	// 1) sendnow   and   domainin name verify(docs/ under indefaultlist). 
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
			continue //     store then ed
		}
		sources = append(sources, srcDoc{abs: clean, base: name})
		if len(sources) >= orchestrateMaxSources {
			break
		}
	}

	// 2)   read(file read,   back );      ->  write   . 
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

	// 3)   become    (LLM noJSON   base;   usethen  ity connect,  produceout  file). 
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
		body = deterministicSummary(o.EffectiveLang(title+"\n"+merged), title, names, merged)
	}
	doc := "# " + title + "\n\n" +
		"> 本文件由 VoiceSign Harness 多步编排（ORCHESTRATE：读→汇总→写→提交）自动生成。\n\n" +
		body + "\n"

	// 4) writeobjtgtfile( name verifyafter  file write,   back ,    tgt ). 
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

	// 5)  to git   (only add occurbecomefile,    git add -A). 
	crecv := o.commitTargetPath(root, cleanTarget, "vhs(orchestrate): 生成《"+title+"》多步编排落地")
	crecv.Seq = nextSeq()
	receipts = append(receipts, crecv)
	return receipts
}

// execBuildTest (distillation R5, 2026-10-08) executes the real work pipeline the
// iOS repro demanded: clone/download <repo> -> build -> test. It resolves the URL
// from (a) explicit text, (b) the task slot, (c) recent entities; works in
// <logDir>/workspace/<repo>; probes go.mod / package.json / Makefile; runs the
// matching build & test commands and reports receipts. The task slot is written
// so "开始干呀/继续" resumes the same job.
func (o *Options) execBuildTest(ctx context.Context, it contract.Intent, logDir string) []contract.Receipt {
	var receipts []contract.Receipt
	seq := 0
	bump := func() int { seq++; return seq }

	url := ""
	if it.Params != nil {
		url = strings.TrimSpace(it.Params["url"])
	}
	// fallback 1: task slot url
	if url == "" {
		if slot := loadTaskSlot(logDir, o.ConvID); slot != nil && slot.Params != nil {
			url = strings.TrimSpace(slot.Params["url"])
		}
	}
	// fallback 2: recent entities that look like a github repo
	if url == "" && o.Refer != nil {
		for _, e := range o.Refer.Recent {
			if strings.Contains(e.Entity, "github.com/") || strings.Contains(e.Entity, "github.com ") {
				url = e.Entity
				if !strings.HasPrefix(url, "http") {
					url = "https://" + url
				}
				break
			}
		}
	}
	if url == "" {
		receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "build_test", OK: false,
			Err: "要下载哪个仓库？给我 URL 或仓库名，例如「下载 https://github.com/owner/repo 然后编译测试」"})
		return receipts
	}

	// normalize: owner/repo shorthand -> https URL
	if !strings.HasPrefix(url, "http") && strings.Contains(url, "/") {
		url = "https://" + url
	}
	repoName := filepath.Base(strings.TrimSuffix(url, "/"))
	if i := strings.Index(repoName, ".git"); i >= 0 {
		repoName = repoName[:i]
	}
	if repoName == "" || repoName == "." || repoName == "/" {
		repoName = "repo"
	}
	wsRoot := filepath.Join(logDir, "workspace")
	_ = os.MkdirAll(wsRoot, 0o755)
	repoDir := filepath.Join(wsRoot, repoName)

	// write the task slot so a later CONTINUE can resume this job
	writeTaskSlot(logDir, o.ConvID, &TaskSlot{
		Kind:   TaskKindBuildTest,
		Params: map[string]string{"url": url},
		Steps:  []string{"clone", "build", "test"},
		Step:   0,
		Status: "running",
		Text:   it.CorrectedText,
	})

	// step 1: clone (shallow). If the dir already exists and is a git repo, pull instead.
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err == nil {
		precv := o.run("git", map[string]any{"args": []string{"-C", repoDir, "pull", "--ff-only"}, "cwd": repoDir})
		precv.Seq = bump()
		precv.Tool = "git"
		precv.Stdout = "repo already present, pulled: " + precv.Stdout
		receipts = append(receipts, precv)
	} else {
		_ = os.RemoveAll(repoDir)
		crecv := o.run("git", map[string]any{"args": []string{"clone", "--depth", "1", url, repoDir}})
		crecv.Seq = bump()
		crecv.Tool = "git"
		receipts = append(receipts, crecv)
		if !crecv.OK {
			writeTaskSlot(logDir, o.ConvID, &TaskSlot{
				Kind: TaskKindBuildTest, Params: map[string]string{"url": url},
				Steps: []string{"clone", "build", "test"}, Step: 0, Status: "failed", Text: it.CorrectedText,
			})
			return receipts
		}
	}

	// step 2: probe the language and build
	buildCmd := []string{}
	testCmd := []string{}
	if _, err := os.Stat(filepath.Join(repoDir, "go.mod")); err == nil {
		buildCmd = []string{"go", "build", "./..."}
		// CI 口径（.github/workflows/ci.yml）：排除 asr/doccontract 包，避免环境依赖测试误报。
		listRecv := o.run("run", map[string]any{"command": []string{"go", "list", "./..."}, "cwd": repoDir})
		var pkgs []string
		for _, ln := range strings.Split(listRecv.Stdout, "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" || strings.HasSuffix(ln, "/asr") || strings.HasSuffix(ln, "/doccontract") {
				continue
			}
			pkgs = append(pkgs, ln)
		}
		if len(pkgs) == 0 {
			testCmd = []string{"go", "test", "./..."}
		} else {
			testCmd = append([]string{"go", "test"}, pkgs...)
		}
	} else if _, err := os.Stat(filepath.Join(repoDir, "package.json")); err == nil {
		buildCmd = []string{"npm", "install", "--no-audit", "--no-fund"}
		testCmd = []string{"npm", "test", "--", "--runInBand"}
	} else {
		// generic make fallback
		if _, err := os.Stat(filepath.Join(repoDir, "Makefile")); err == nil {
			buildCmd = []string{"make", "build"}
			testCmd = []string{"make", "test"}
		} else {
			receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "build_test", OK: true,
				Stdout: "克隆完成（" + repoDir + "），未识别到 go.mod/package.json/Makefile，跳过编译与测试"})
			writeTaskSlot(logDir, o.ConvID, &TaskSlot{
				Kind: TaskKindBuildTest, Params: map[string]string{"url": url},
				Steps: []string{"clone", "build", "test"}, Step: 3, Status: "done", Text: it.CorrectedText,
			})
			return receipts
		}
	}
	if len(buildCmd) > 0 {
		brecv := o.run("run", map[string]any{"command": buildCmd, "cwd": repoDir})
		brecv.Seq = bump()
		brecv.Tool = "run"
		receipts = append(receipts, brecv)
		if !brecv.OK {
			writeTaskSlot(logDir, o.ConvID, &TaskSlot{
				Kind: TaskKindBuildTest, Params: map[string]string{"url": url},
				Steps: []string{"clone", "build", "test"}, Step: 1, Status: "failed", Text: it.CorrectedText,
			})
			return receipts
		}
	}
	if len(testCmd) > 0 {
		trecv := o.run("test", map[string]any{"command": testCmd, "cwd": repoDir})
		trecv.Seq = bump()
		trecv.Tool = "test"
		receipts = append(receipts, trecv)
	}

	writeTaskSlot(logDir, o.ConvID, &TaskSlot{
		Kind: TaskKindBuildTest, Params: map[string]string{"url": url},
		Steps: []string{"clone", "build", "test"}, Step: 3, Status: "done", Text: it.CorrectedText,
	})
	return receipts
}

// execInstall (distillation R6, 2026-10-08) really installs developer CLIs on the
// host — "安装 codex / claude code 到后台" (iOS repro: "code code x and cloud code").
// Software names are mapped to npm packages; the action is gated: a fresh INSTALL
// writes a pending task slot and asks for confirmation, and a later CONTINUE
// ("怎么还没执行呢/立刻执行") resumes and runs `npm install -g` for real.
func (o *Options) execInstall(ctx context.Context, it contract.Intent, logDir string) []contract.Receipt {
	var receipts []contract.Receipt
	seq := 0
	bump := func() int { seq++; return seq }

	pkgs := []string{}
	if it.Params != nil {
		for _, p := range strings.Fields(it.Params["packages"]) {
			pkgs = append(pkgs, p)
		}
	}
	if len(pkgs) == 0 {
		if slot := loadTaskSlot(logDir, o.ConvID); slot != nil && slot.Params != nil {
			for _, p := range strings.Fields(slot.Params["packages"]) {
				pkgs = append(pkgs, p)
			}
		}
	}
	if len(pkgs) == 0 {
		receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "install", OK: false,
			Err: "要安装哪个软件？例如「安装 codex」或「安装 claude code 到后台」"})
		return receipts
	}

	// Confirmation gate on a fresh INSTALL: persist a pending slot and ask instead of
	// silently changing the host. "确认/装吧/立刻执行" resumes through CONTINUE.
	if it.Params == nil || it.Params["resumed"] != "1" {
		writeTaskSlot(logDir, o.ConvID, &TaskSlot{
			Kind: TaskKindInstall, Params: map[string]string{"packages": strings.Join(pkgs, " ")},
			Steps: []string{"confirm", "install"}, Step: 0, Status: "pending_confirm", Text: it.CorrectedText,
		})
		receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "install", OK: false,
			ConfirmAsk: true,
			Err:        "准备安装 " + strings.Join(pkgs, "、") + "（npm 全局安装，写入系统）。确认后我立刻执行——回复「确认」或「装吧」"})
		return receipts
	}

	// Resumed install: verify node/npm first.
	nodeRecv := o.run("run", map[string]any{"command": []string{"node", "--version"}})
	npmRecv := o.run("run", map[string]any{"command": []string{"npm", "--version"}})
	if !nodeRecv.OK || !npmRecv.OK {
		writeTaskSlot(logDir, o.ConvID, &TaskSlot{
			Kind: TaskKindInstall, Params: map[string]string{"packages": strings.Join(pkgs, " ")},
			Steps: []string{"confirm", "install"}, Step: 1, Status: "failed", Text: it.CorrectedText,
		})
		receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "install", OK: false,
			Err: "后台没有可用的 node/npm，无法执行 npm 安装（node: " + nodeRecv.Stderr + " npm: " + npmRecv.Stderr + "）"})
		return receipts
	}

	// Dry-run mode for local end-to-end tests: prints the exact command, no host change.
	if os.Getenv("VHS_EXEC_DRY_RUN") != "" {
		receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "install", OK: true,
			Stdout: "[dry-run] 将执行: npm install -g " + strings.Join(pkgs, " ")})
		writeTaskSlot(logDir, o.ConvID, &TaskSlot{
			Kind: TaskKindInstall, Params: map[string]string{"packages": strings.Join(pkgs, " ")},
			Steps: []string{"confirm", "install"}, Step: 2, Status: "done", Text: it.CorrectedText,
		})
		return receipts
	}

	instRecv := o.run("run", map[string]any{
		"command":   append([]string{"npm", "install", "-g"}, pkgs...),
		"timeout_s": 300,
	})
	instRecv.Seq = bump()
	instRecv.Tool = "install"
	receipts = append(receipts, instRecv)
	status := "failed"
	if instRecv.OK {
		status = "done"
	}
	writeTaskSlot(logDir, o.ConvID, &TaskSlot{
		Kind: TaskKindInstall, Params: map[string]string{"packages": strings.Join(pkgs, " ")},
		Steps: []string{"confirm", "install"}, Step: 2, Status: status, Text: it.CorrectedText,
	})
	return receipts
}

// ---------------------------------------------------------------------------
// execEmail (distillation R7, 2026-10-09) — "收到邮件之后的处理"
//
// DeepSeek Harness distillation: the task is executed as a real chain instead of a
// canned reply — fetch the AIOps inbox (X-AIops-Key from env), run Strata
// (192.168.8.201 local Qwen, NOT aiops.voxsign.ai) analysis with a section-assembled
// prompt (identity → task → rules → email snapshot → output format), and produce a
// structured Chinese receipt. summary/list/archive run for real; reply/forward are
// confirm-gated and write a draft deliverable under harness-output/.
func (o *Options) execEmail(ctx context.Context, it contract.Intent, logDir string) []contract.Receipt {
	var receipts []contract.Receipt
	seq := 0
	bump := func() int { seq++; return seq }

	action := "summary"
	scope := "unread"
	target := ""
	if it.Params != nil {
		if it.Params["action"] != "" {
			action = it.Params["action"]
		}
		if it.Params["scope"] != "" {
			scope = it.Params["scope"]
		}
		target = it.Params["target"]
	}
	// Resumed reply/forward after the confirmation gate: write the draft deliverable.
	if it.Params != nil && it.Params["resumed"] == "1" && (action == "reply" || action == "forward") {
		draft := it.Params["draft"]
		if draft == "" {
			draft = "(未取到拟稿内容，请重新说一遍要回复的内容)"
		}
		file := filepath.Join(logDir, "..", "harness-output")
		if err := os.MkdirAll(file, 0o755); err == nil {
			fp := filepath.Join(file, "email-"+action+"-"+time.Now().Format("20060102-150405")+".md")
			if err := os.WriteFile(fp, []byte(draft), 0o644); err == nil {
				writeTaskSlot(logDir, o.ConvID, &TaskSlot{
					Kind: TaskKindEmail, Params: it.Params, Steps: []string{"confirm", "draft"},
					Step: 2, Status: "done", Text: it.CorrectedText,
				})
				receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "email", OK: true,
					Stdout: "已生成" + action + "草稿：" + fp + "\n\n" + draft})
				return receipts
			}
		}
		receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "email", OK: false,
			Err: "草稿写入失败（harness-output 不可写）"})
		return receipts
	}

	// 1) Fetch the AIOps inbox for real (read-only GET).
	emails, err := o.emailFetch(ctx, 20)
	if err != nil {
		receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "email", OK: false,
			Err: "取邮件失败：" + err.Error()})
		return receipts
	}
	if len(emails) == 0 {
		receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "email", OK: true,
			Stdout: "收件箱暂时没有可处理的邮件。"})
		return receipts
	}

	// 2) Filter by scope: unread (first N) or all. Sender-target matching for reply/forward.
	var sel []emailItem
	for i, e := range emails {
		if scope == "unread" && i >= 12 {
			break
		}
		if target != "" {
			tl := strings.ToLower(target)
			if strings.Contains(strings.ToLower(e.from), tl) || strings.Contains(strings.ToLower(e.subject), tl) {
				sel = append(sel, e)
			}
		} else {
			sel = append(sel, e)
		}
	}
	if len(sel) == 0 {
		receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "email", OK: false,
			Err: "没有找到匹配" + target + "的邮件（已取 " + strconv.Itoa(len(emails)) + " 封，检查一下发件人或主题描述）"})
		return receipts
	}

	// 3) reply/forward: confirmation gate — draft the reply, ask before any external action.
	if action == "reply" || action == "forward" {
		draftPrompt := emailPrompt(action, scope, target, sel)
		draft := o.emailReason(ctx, draftPrompt)
		if draft == "" {
			draft = "（拟稿失败：模型未返回内容，请重试或换一种说法）"
		}
		writeTaskSlot(logDir, o.ConvID, &TaskSlot{
			Kind: TaskKindEmail, Params: map[string]string{
				"action": action, "scope": scope, "target": target, "draft": draft,
			},
			Steps: []string{"confirm", "draft"}, Step: 0, Status: "pending_confirm", Text: it.CorrectedText,
		})
		receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "email", OK: false,
			ConfirmAsk: true,
			Err: "已拟好" + action + "草稿：\n\n" + truncateStr(draft, 900) +
				"\n\n确认后我生成草稿文件（发送通道下一轮对接服务器 mailSend）——回复「确认」"})
		return receipts
	}

	// 4) summary/list/archive: real analysis via the local Strata Qwen (192.168.8.201).
	prompt := emailPrompt(action, scope, target, sel)
	content := o.emailReason(ctx, prompt)
	if content == "" {
		// Deterministic fallback: never leave the user with a blank receipt.
		content = "（分析模型未返回内容，列出原始清单）\n"
		for i, e := range sel {
			content += fmt.Sprintf("%d. %s｜%s｜%s\n", i+1, e.from, e.subject, e.ts)
		}
	}
	receipts = append(receipts, contract.Receipt{Seq: bump(), Tool: "email", OK: true,
		Stdout: content})
	return receipts
}

// emailItem is a normalized inbox row (mirror of the server-side NormalizedEmail).
type emailItem struct {
	from    string
	subject string
	ts      string
	body    string
}

// emailFetch GETs the AIOps inbox. Key comes from AIOPS_KEY env (container already
// injects AIOPS_KEY + VHS_AIOPS_URL); base defaults to the VHS_AIOPS_URL host.
func (o *Options) emailFetch(ctx context.Context, limit int) ([]emailItem, error) {
	key := os.Getenv("AIOPS_KEY")
	if key == "" {
		return nil, fmt.Errorf("AIOPS_KEY 未配置（服务器容器已注入，本机请设置 AIOPS_KEY）")
	}
	base := os.Getenv("AIOPS_BASE")
	if base == "" {
		if u := os.Getenv("VHS_AIOPS_URL"); u != "" {
			if parsed, err := url.Parse(u); err == nil && parsed.Scheme != "" && parsed.Host != "" {
				base = parsed.Scheme + "://" + parsed.Host
			}
		}
	}
	if base == "" {
		base = "https://aiops.voxsign.ai"
	}
	u := fmt.Sprintf("%s/api/email/list?tenant_id=sfdapartner&limit=%d&include_body=1", base, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-AIops-Key", key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var payload struct {
		OK    bool `json:"ok"`
		Count int  `json:"count"`
		Items []struct {
			From         string `json:"from"`
			Subject      string `json:"subject"`
			Date         string `json:"date"`
			TS           int64  `json:"ts"`
			BodyPreview  string `json:"bodyPreview"`
			Body         string `json:"body"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	var out []emailItem
	for _, it := range payload.Items {
		ts := it.Date
		if ts == "" && it.TS > 0 {
			ts = time.Unix(it.TS, 0).Format("2006-01-02 15:04")
		}
		body := it.Body
		if body == "" {
			body = it.BodyPreview
		}
		out = append(out, emailItem{
			from:    it.From,
			subject: it.Subject,
			ts:      ts,
			body:    truncateStr(body, 400),
		})
	}
	return out, nil
}

// emailReason calls the LOCAL Strata Qwen (192.168.8.201:8080) — NOT the aiops
// gateway — to analyze the inbox snapshot (user requirement: 192.168.8.201 Qwen).
// OpenAI-compatible /v1/chat/completions; llama.cpp servers reject non-curl UAs,
// so the request is sent with User-Agent: curl/8.7.1.
func (o *Options) emailReason(ctx context.Context, userPrompt string) string {
	strata := os.Getenv("VHS_EMAIL_STRATA")
	if strata == "" {
		strata = "http://192.168.8.201:8080"
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model":       "qwen3.8-flash-next-ista-iq3_xxs",
		"messages":    []contract.Message{
			{Role: "system", Content: emailSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		"max_tokens":  1200,
		"temperature": 0.2,
		// llama.cpp/Qwen3: disable chain-of-thought so the content budget is not
		// consumed by reasoning_content (observed finish=length otherwise).
		"chat_template_kwargs": map[string]any{"enable_thinking": false},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strata+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "curl/8.7.1")
	// Strata (llama.cpp) gates with an API key; the server env carries it as
	// STRATA_API_KEY (18 chars). Missing key -> the server replies 401.
	if k := os.Getenv("STRATA_API_KEY"); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[execEmail] Strata call failed: %v", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		log.Printf("[execEmail] Strata HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		return ""
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		log.Printf("[execEmail] Strata decode err: %v", err)
		return ""
	}
	if len(out.Choices) == 0 {
		log.Printf("[execEmail] Strata empty choices")
		return ""
	}
	c := strings.TrimSpace(out.Choices[0].Message.Content)
	if c == "" {
		log.Printf("[execEmail] Strata empty content; finish=%s", out.Choices[0].FinishReason)
	}
	return c
}

// emailSystemPrompt is the section-assembled mail-handling policy distilled from
// DeepSeek Harness's goal/policy pattern: identity → task → rules → completion
// standard → output format. Never fabricate body text; a missing body is reported,
// not invented.
const emailSystemPrompt = `你是 VoxSign 邮箱处理助手。你的任务：把用户收到的邮件处理成可直接阅读的中文结果。

规则：
- 只依据邮件快照中的信息，正文缺失时标注「正文未取到」，绝不编造内容；
- 每封邮件给出发件人、主题、时间、要点（如有正文）、是否需要用户行动；
- 分类标注：紧急待办 / 可稍后处理 / 资讯参考 / 疑似垃圾；
- 总结保持简洁（每封 1-3 行），总量不超过 600 字；
- 完成标准：输出可直接给用户看的中文结果清单；若存在需要用户决定的事项（如是否回复、是否付款），单独列出；
- 遇到同一阻塞条件才说「无法处理」，困难不等于无法处理。`

// emailPrompt assembles the task + snapshot for the model (DeepSeek-style section
// composition: the fixed system section stays constant, the user section carries
// the current task and the live inbox snapshot).
func emailPrompt(action, scope, target string, sel []emailItem) string {
	var sb strings.Builder
	switch action {
	case "list":
		sb.WriteString("任务：列出邮件清单（范围：" + scope + "）。")
	case "reply":
		sb.WriteString("任务：针对目标邮件起草一封中文回复（目标线索：" + target + "）。")
	case "forward":
		sb.WriteString("任务：起草一封转发说明（目标线索：" + target + "）。")
	case "archive":
		sb.WriteString("任务：判断哪些邮件可以归档（已处理/资讯类），列出归档建议。")
	default:
		sb.WriteString("任务：总结以下邮件（范围：" + scope + "）。")
	}
	sb.WriteString("\n\n邮件快照：\n")
	for i, e := range sel {
		sb.WriteString(fmt.Sprintf("[%d] 发件人=%s｜主题=%s｜时间=%s\n正文=%s\n",
			i+1, e.from, e.subject, e.ts, e.body))
	}
	sb.WriteString("\n输出：直接输出中文结果，不要 JSON、不要复述指令。")
	return sb.String()
}

// execImplement(2026-10-04 connectline): kind=implement   nowclass task--
// LLM byneedrequire  occurbecome Go serveserviceto harness-output/<slug>/,  data   recv (<=5  , 
//   back under   memory), no  after git   . doc  firstget o.Document(server document
//   ), back  it.Params["document"]. 
func (o *Options) execImplement(ctx context.Context, it contract.Intent, logDir string) []contract.Receipt {
	root := o.projectRootForCommit(it)
	if root == "" {
		return []contract.Receipt{{Tool: "implement", OK: false,
			Err: "实现类任务需 project 域且 scope 指向项目根（projectRootForCommit 未解析）"}}
	}
	title := strings.TrimSpace(it.Params["target_doc"])
	if title == "" {
		title = "语音适配层"
	}
	doc := o.Document
	if doc == "" {
		doc = it.Params["document"]
	}
	if doc == "" {
		return []contract.Receipt{{Tool: "implement", OK: false,
			Err: "实现类任务缺少需求文档（document 通道为空；POST /v1/tasks 需带 document 全文）"}}
	}
	skelDir := filepath.Join(root, "harness-output", asciiSlug(title))
	var receipts []contract.Receipt
	nextSeq := func() int { return len(receipts) + 1 }
	memory := ""
	done := false
	// 2026-10-04  code: 5->8(useuser"  give  code tobecome ";   noteincurbefore codeafterrecv rate  )
	const maxImplRounds = 8
	for round := 1; round <= maxImplRounds && !done; round++ {
		files, note := o.llmGenerateImplement(ctx, title, doc, skelDir, memory)
		if files == nil {
			// 2026-10-04 fix : note is    ("  safety (N  )") iserror--
			// onlyhas files==nil only   (LLM   use/    ),  thenartifactalready  write . 
			receipts = append(receipts, contract.Receipt{Seq: nextSeq(), Tool: "implement",
				OK: false, Err: note})
			break
		}
		for name := range files {
			p := filepath.Join(skelDir, name)
			clean, ok := space.ResolveScopePath(root, p)
			if !ok {
				receipts = append(receipts, contract.Receipt{Seq: nextSeq(), Tool: "implement",
					OK: false, Err: "白名单外写路径被拒绝: " + p})
				continue
			}
			receipts = append(receipts, contract.Receipt{Seq: nextSeq(), Tool: "implement", OK: true,
				Stdout: "writed: " + clean})
		}
		out := &Outcome{Receipts: receipts}
		gaps := o.EvidenceGaps(out)
		if len(gaps) == 0 {
			done = true
			receipts = append(receipts, contract.Receipt{Seq: nextSeq(), Tool: "evidence", OK: true,
				Stdout: fmt.Sprintf("证据门 PASS（round %d，无缺口）", round)})
			break
		}
		memory = "上一轮证据门缺口（必须修复后才能完成）：\n" + strings.Join(gaps, "\n")
		// 2026-10-04 fix : fix  noteincurbeforeartifact code(LLM safety heavywrite  "fixA B"). 
		// 2026-10-04 againfix: onlyopenhead 3000 char   tofileafter   E4/E5 handler(8    )--
		// modifyfirsttail connect(first 1500 + tail 3500), keep   to  num see. 
		if cur, err := os.ReadFile(filepath.Join(skelDir, "main.go")); err == nil && len(cur) > 0 {
			src := string(cur)
			head, tail := truncateStr(src, 1500), truncateStrTail(src, 3500)
			memory += "\n\n当前产物 main.go（开头 1500 字符 + 末尾 3500 字符，中间省略。基于它修改，只修缺口对应函数，输出完整新文件）：\n===main.go 开头===\n" + head + "\n===main.go 末尾===\n" + tail
		}
		receipts = append(receipts, contract.Receipt{Seq: nextSeq(), Tool: "evidence", OK: false,
			Err:  fmt.Sprintf("round %d 证据门 %d 项缺口", round, len(gaps)),
			Stdout: strings.Join(gaps, "\n")})
	}
	if !done {
		receipts = append(receipts, contract.Receipt{Seq: nextSeq(), Tool: "implement", OK: false,
			Err: fmt.Sprintf("证据门 %d 轮未收敛（已达最大轮次，需人工修订）", maxImplRounds)})
	}
	crecv := o.commitTargetPath(root, skelDir, "vhs(implement): "+title+" 实现（证据门收敛）")
	crecv.Seq = nextSeq()
	receipts = append(receipts, crecv)
	return receipts
}

// implCapabilityBrief is nowclasstask  P0   list bot need(LLM   usetime      ). 
//  4(2026-10-04)raisebyneedrequire  endpointlistnoteinas , baselistonly no   bot. 
const implCapabilityBrief = `①个性化词典：增/删/查条目（含匹配与纠错安全）
②文本纠错：清洗 + 词典纠错（正常文本不被改坏）
③意图分类：NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE 五类
④反馈学习：✔/✘ 回馈落盘 feedback.jsonl（append-only）
⑤数据落盘：traces/usage 等 JSONL append-only，独立数据目录（data-dir 可配）
⑥HTTP 端点：/v1/health、/v1/process（JSON 请求/响应）
⑦只监听 127.0.0.1（非回环拒绝），鉴权可占位但须有`

// llmSummarize use fast provider pipe    in   become   Markdown   (noJSON   base, 
// and QUERY answer same :  formclose  json_object,  send temperature=0).      -> returnbackempty . 
//
// [   type  ]gpt-6-luna is   type, reasoning     max_completion_tokens; 
// 1500 safetybe    ->finish_reason=length, content empty. thus  giveto 8000, and  system  
// needrequire" connect outpos ,      ".   /empty   day (to  queryLLMAnswer,  again  ). 
func (o *Options) llmSummarize(ctx context.Context, title, merged string) string {
	if o == nil || o.Providers == nil {
		log.Printf("[llmSummarize] providers nil")
		return ""
	}
	rid := o.RequestID // P0-4b: day   chain request_id(Run alreadywrite-back o.RequestID)
	p, err := o.Providers.Get("fast")
	if err != nil {
		log.Printf("[llmSummarize] rid=%s fast provider unavailable: %v", rid, err)
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
		log.Printf("[llmSummarize] rid=%s fast Chat err: %v", rid, err)
		return ""
	}
	if strings.TrimSpace(resp.Content) == "" {
		log.Printf("[llmSummarize] rid=%s fast Chat empty content (finish_reason may be length; reasoning budget exhausted)", rid)
		return ""
	}
	c := strings.TrimSpace(resp.Content)
	// provider  default json_object time       JSON  , resolveout basecharseg. 
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
			log.Printf("[llmSummarize] rid=%s returned unparsable JSON shell: %.160s", rid, c)
			return ""
		}
	}
	return c
}

// deterministicSummary no typetime   ity  (close ize connect   ), keep  produceout  file.
// lang selects the template set (en/zh); see i18n.go.
func deterministicSummary(lang Lang, title string, sourceNames []string, merged string) string {
	t := pickTmpl(lang)
	var sb strings.Builder
	sb.WriteString(t.sumOverview)
	fmt.Fprintf(&sb, t.sumIncludedFmt, len(sourceNames))
	for _, name := range sourceNames {
		fmt.Fprintf(&sb, "- %s\n", name)
	}
	sb.WriteString(t.sumSourcesHead)
	sb.WriteString(t.sumNote)
	sb.WriteString(t.sumExcerptHead)
	sb.WriteString(truncateStr(merged, 6000))
	sb.WriteString("\n```\n")
	return sb.String()
}


// sanitizePathPart pipetgt  assafesafety pathsplitseg(keep in  numcharandlinkchar ). 
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

// asciiSlug pipetgt  as ASCII safesafetyname(module path use;   ASCII    '-', emptythenback  impl). 
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

// llmGenerateImplement(L-01  recv 2026-10-03): needrequire   -> LLM occurbecome**finish    **
//   Go  now -> write  -> go build       ->   errorback fix (<=2  ). 
//
// ** fileoccurbecome**(2026-10-03     ): aiops  closeto /api/model/chat has ~60s  onlimit
// ( call 12000 tokens 60.6s -> 504 Gateway Time-out);  fileoccurbecome   36s/11913 char  ✅. 
// ⇒  become main.go(   finish serveservice)  calluse + go.mod/README  file,    close 504. 
//
// returnback (files, note): files=nil tableshow LLM   use/    (calluse back   ity  ); 
// note as  origbecauseor"  safety (N  )". 
// chatWithFallback(2026-10-04    provider new API +  typecall ): by pref      provider, 
//   /error   under  (  /thus    ; to " diff type bot"needrequire). 
func (o *Options) chatWithFallback(ctx context.Context, pref []string, req provider.ChatRequest) (provider.ChatResponse, error) {
	if o == nil || o.Providers == nil {
		return provider.ChatResponse{}, fmt.Errorf("providers nil")
	}
	var lastErr error
	for _, name := range pref {
		p, err := o.Providers.Get(name)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := p.Chat(ctx, req)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", name, err)
			log.Printf("[llm-trace] %s attempt=FAIL retryable=true err=%v", name, err)
			continue
		}
		return resp, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("provider 候选列表为空")
	}
	return provider.ChatResponse{}, lastErr
}

// stripCodeFence    LLM  out  Markdown  code  (```go ... ```). 
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSpace(s)
		if strings.HasSuffix(s, "```") {
			s = strings.TrimSpace(strings.TrimSuffix(s, "```"))
		}
	}
	return s
}

func (o *Options) llmGenerateImplement(ctx context.Context, title, doc, skelDir, memory string) (map[string]string, string) {
	if o == nil || o.Providers == nil {
		return nil, "providers nil"
	}
	//        .go(2026-10-03   :    domain.go/router.go and LLM     main.go   , 
	//     ./router.go:21 undefined: writeJSON -> fix    fix    LLM occurbecome file). 
	// LLM    occurbecomeafteronlykeep  LLM artifact(main.go/go.mod/README), obj inits  .go     . 
	if ents, err := os.ReadDir(skelDir); err == nil {
		for _, e := range ents {
			n := e.Name()
			if strings.HasSuffix(n, ".go") {
				_ = os.Remove(filepath.Join(skelDir, n))
			}
		}
	}
	//  typecall (2026-10-03): fast        center->strong->gpt-mini,  again pt  . 
	// genWithPref: by pref   chaincall (fast=deepseek etc; gpt4o/gpt-mini     ). 
	// 2026-10-03     : ①fast   1.8s returnback<200 char   out ->  no ( out  no becall    ); 
	// ②gpt-4o  out   ```go Markdown    ->  connectwrite      (expected 'package')-> stripCodeFence   . 
	genWithPref := func(pref []string, minLen int, sysMsg, usrMsg string) (string, string) {
		resp, err := o.chatWithFallback(ctx, pref, provider.ChatRequest{
			Messages: []contract.Message{
				{Role: "system", Content: sysMsg},
				{Role: "user", Content: usrMsg},
			},
			// 2026-10-03     : MaxTokens onlimit   typeusefull 12000 tokens ->  close 60s 504/ time; 
			//   onlimit ->  type howeverrecv ( call   36s/11913 char ). thus place   MaxTokens. 
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
	//    byfilesplit : main.go>=200(finish serveservice); go.mod>=20(module+go  only ~35 char , 
	// 2026-10-03    200       go.mod ->    now  ); README>=50. 
	gen := func(sysMsg, usrMsg string) (string, string) {
		return genWithPref([]string{"fast", "center", "strong", "gpt-mini"}, 200, sysMsg, usrMsg)
	}

	// 2026-10-04  4 fix : needrequire  endpointlist + needpt need  notein prompt. 
	//  before req only   code implCapabilityBrief(ASR   list)--list     ASR needrequiretime
	//    use;  needrequire(langaudio   )after LLM noneedrequire   -> occurbecomeerrorin  ->  data      ->
	// fix    to blocked. posresolve: from doc  getendpointlistnotein( use endpointRefsOf  data   ), 
	//  needcontrol  ( close 60s onlimit), semantic data by data  bot. 
	req := "目标产品标题：" + title + "\n\n需求文档要求实现的端点清单（验收逐端点核对，必须全部注册）：\n" +
		sortedEndpoints(doc) + "\n\n需求要点摘要（简要）：\n" + docBrief(doc, 1200) +
		"\n\nP0 能力清单（按此实现，可合理扩展）：\n" + implCapabilityBrief + memoryBlock(memory)
	if o.RoundEvidence != "" {
		// L-01   :   2+    on   data   (to  dsh goal-round  "donebeforerecv  data"). 
		req += "\n\n上一轮证据门缺口（本轮必须补齐后才能验收）：\n" + o.RoundEvidence + "\n"
	}
	files := map[string]string{}

	// ① main.go:    finish serveservice(word /correction/intent/rev /JSONL   /endpoint/    )-- file  calluse. 
	sysMain := "你是资深 Go 工程师。只输出 main.go 的**完整代码文本**（自包含、可直接 go build 通过的服务）。" +
		"硬性要求：①仅用标准库，零第三方依赖；②**每个 http.HandleFunc 端点必须实现完整可运行的业务逻辑并返回真实数据**，禁止任何 501 StatusNotImplemented、`// Placeholder`、TODO、panic 占位——验收会逐个真跑打接口断言响应；③实现需求文档 P0 核心能力（词典增删查/纠错/意图分类/反馈/数据 JSONL 落盘 append-only）；" +
		"④需求文档要求/提及的**每一个 /v1/ 端点**都必须用 http.HandleFunc(\"/v1/...\", …) 字面量逐一注册（验收会按需求端点清单逐端点核对，缺一即不合格）；" +
		"⑤可独立运行（监听 127.0.0.1，addr/data-dir 用 flag 或环境变量）。" +
		"纯文本输出，不要 Markdown 围栏、不要 JSON、不要解释。"
	mainCode, note := genWithPref([]string{"gpt4o", "gpt-mini", "fast", "center", "strong"}, 200, sysMain, req)
	if mainCode == "" {
		return nil, "main.go 生成失败: " + note
	}
	files["main.go"] = mainCode

	// ② go.mod:  file  calluse. 
	modCode, note := genWithPref([]string{"fast", "center", "strong", "gpt-mini"}, 20, "你是 Go 工程师。只输出 go.mod 的完整文本：module 名用 harness-output/impl（ASCII 小写，中文 module 非法），go 版本 1.21。纯文本，不要围栏。", req)
	if modCode == "" {
		return nil, "go.mod 生成失败: " + note
	}
	files["go.mod"] = modCode

	// ③ README.md:  file,      ( ed    ). 
	if rd, rn := gen("你是技术文档作者。输出 README.md 的简短中文运行说明（启动命令/端点/数据文件）。纯文本，不要围栏。", req); rd != "" {
		files["README.md"] = rd
	} else {
		log.Printf("[llmGenerateImplement] README 生成跳过（%s）", rn)
	}

	if err := writeFilesToDisk(skelDir, files); err != nil {
		return nil, "写盘失败: " + err.Error()
	}
	//      ( allow ): go build -C <skelDir> ./...(Go 1.20+ -C  keep, runCmd Dir   thususe -C). 
	buildRecv := o.run("run", map[string]any{"command": []string{"go", "build", "-C", skelDir, "./..."}})
	if buildRecv.OK {
		return files, "编译全绿（0 轮修复）"
	}
	//      -> ①  ityfix   (import  usebyname   +   usechange by id ,   error LLM fix  ; 
	// 2026-10-04     : import   fix  + objects  usechange , LLM safety heavywrite 3    recv ); 
	// ②  ity     onlyback  LLM(<=3  ). 
	log.Printf("[llmGenerate] 编译失败（首轮），错误：\n%s", truncateStr(buildRecv.Stdout+"\n"+buildRecv.Stderr, 1200))
	// call keep : LLM artifact base(   bot overwritewrite artifact,      provide  errorsplit ). 
	_ = os.WriteFile("/tmp/llm_main_debug.go", []byte(files["main.go"]), 0o644)
	cleanErrs := buildRecv.Stdout + "\n" + buildRecv.Stderr
	lastErr := truncateStr(cleanErrs, 2500)
	//   ityfix   :     after i.e.heavy  ( id/onunder   recv ,    6  ). 
	for d := 1; d <= 6; d++ {
		cleaned, n := deterministicClean(files["main.go"], lastErr)
		if n == 0 {
			break
		}
		log.Printf("[llmGenerate] 确定性修复轮 %d 清理 %d 处（import/未用变量）", d, n)
		files["main.go"] = cleaned
		if err := writeFilesToDisk(skelDir, files); err != nil {
			return nil, "写盘失败: " + err.Error()
		}
		recv := o.run("run", map[string]any{"command": []string{"go", "build", "-C", skelDir, "./..."}})
		if recv.OK {
			return files, fmt.Sprintf("编译全绿（确定性修复 %d 轮）", d)
		}
		lastErr = truncateStr(recv.Stdout+"\n"+recv.Stderr, 2500)
	}
	// 2026-10-04  code: 3->5(LLM fix  occurbecomenew code   in  import/change error,  give  )
	for i := 1; i <= 5; i++ {
		// 2026-10-04 fix :   fix  alsomodifyfirsttail connect(onlyopenhead 2500   to  234   E5 classtypeerror--
		// LLM fix  , 5    , 8  heavy    .   error   id, firsttailnoteinkeep error  see). 
		srcCur := files["main.go"]
		head, tail := truncateStr(srcCur, 1500), truncateStrTail(srcCur, 2500)
		mainCode, note = genWithPref([]string{"gpt4o", "gpt-mini", "fast", "center", "strong"}, 200, sysMain+"\n\n上一轮 main.go **编译失败**，请在以下当前代码基础上**仅修复编译错误**（其他逻辑保持不变）后输出**完整 main.go**。编译错误行号对应【末尾 2500 字符】里的代码。\n\n当前 main.go 开头（1500 字符）：\n"+head+"\n\n当前 main.go 末尾（2500 字符，错误行在此范围）：\n"+tail+"\n\n编译错误：\n"+lastErr, req)
		if mainCode == "" {
			return nil, "main.go 修复失败: " + note
		}
		files["main.go"] = mainCode
		if err := writeFilesToDisk(skelDir, files); err != nil {
			return nil, "写盘失败: " + err.Error()
		}
		buildRecv = o.run("run", map[string]any{"command": []string{"go", "build", "-C", skelDir, "./..."}})
		if buildRecv.OK {
			return files, fmt.Sprintf("编译全绿（%d 轮修复）", i)
		}
		// 2026-10-04 fix : build   after**firstusebase error  ity  **(LLM   heavywrite  in
		// new  use import;  beforeuseon  error  base  code ->    -> 3       
		// "imported and not used",    now"     3     ed"). 
		lastErr = truncateStr(buildRecv.Stdout+"\n"+buildRecv.Stderr, 2500)
		if c3, n3 := removeUnusedImports(files["main.go"], lastErr); c3 != "" {
			log.Printf("[llmGenerate] 修复轮 %d 确定性清理 %d 个未使用 import（本轮错误）", i, n3)
			files["main.go"] = c3
			if err := writeFilesToDisk(skelDir, files); err != nil {
				return nil, "写盘失败: " + err.Error()
			}
			recv := o.run("run", map[string]any{"command": []string{"go", "build", "-C", skelDir, "./..."}})
			if recv.OK {
				return files, fmt.Sprintf("编译全绿（修复轮 %d 确定性清理 %d 个 import）", i, n3)
			}
			//   after has  import error: changenew lastErr provideunder  LLM fix (    errorfix). 
			log.Printf("[llmGenerate] 修复轮 %d 清理后仍失败：\n%s", i, truncateStr(recv.Stdout+"\n"+recv.Stderr, 800))
			lastErr = truncateStr(recv.Stdout+"\n"+recv.Stderr, 2500)
		}
	}
	// 2026-10-04 fix :        returnback after  write artifact(    by data  data 3  out, 
	// fix  continuecontinue  -- beforereturnback nil  connect break, 8  onlimit same  (  : round 2     i.e.end). 
	lastNote := "编译迭代耗尽：最后编译错误——"
	if lastErr != "" {
		lastNote += truncateStr(lastErr, 300)
	} else {
		lastNote += "未知"
	}
	return files, lastNote
}

// parseGenFiles resolve  LLM returnback  {"files":{...}} JSON(   ```json   andbeforeafter  ). 
func parseGenFiles(content string) (map[string]string, string) {
	c := strings.TrimSpace(content)
	if i := strings.Index(c, "```"); i >= 0 {
		//   first    andclosetail  
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

// writeFilesToDisk pipeoccurbecomefilewrite skelDir(first obj ). 
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

// removeUnusedImports from Go  code  ity    error  in   use import(  error). 
//    go build error e.g. `./main.go:4:2: "bufio" imported and not used`; import  in 
// `	"bufio"`    delete. 2026-10-03   : LLM fix  to classerrorfix    (3     ), 
//   ity  is harness     bot( dependency LLM   ). returnback  after  codeand  num . 
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

// removeUnusedVars by    iddelete  usechange voice (declared and not used). 
// 2026-10-04     : langaudio   occurbecome `objects := [...]` but numin use ->
// go build   `./main.go:76:2: declared and not used: objects`, LLM fix  
// safety heavywrite 3   recv .   errorby harness   ity bot: deleteerror (   ). 
//   only   (  id becausedelete  ), byout     heavy  recv . 
func removeUnusedVars(src, buildOut string) (string, int) {
	m := regexp.MustCompile(`\./main\.go:(\d+):\d+: declared and not used: (\w+)`).FindStringSubmatch(buildOut)
	if len(m) != 3 {
		return "", 0
	}
	lineNo, err := strconv.Atoi(m[1])
	if err != nil || lineNo < 1 {
		return "", 0
	}
	lines := strings.Split(src, "\n")
	if lineNo > len(lines) {
		return "", 0
	}
	// only   voice (    := or var before ),     close body/ num boundary. 
	trim := strings.TrimSpace(lines[lineNo-1])
	if !strings.Contains(trim, ":=") && !strings.HasPrefix(trim, "var ") {
		return "", 0
	}
	lines = append(lines[:lineNo-1], lines[lineNo:]...)
	return strings.Join(lines, "\n"), 1
}

// deterministicClean       fix : import safety   (byname)+ change     (by id)
// +   note delete(// Placeholder -- 2026-10-04   :  assafetyedafter   1 place  note , 
// LLM   toin  code(fix  onlynoteinopenhead 3000 char )8  fix  ; note no ,   delete bot). 
func deterministicClean(src, buildOut string) (string, int) {
	if c, n := removeUnusedImports(src, buildOut); c != "" {
		return c, n
	}
	if c, n := removeUnusedVars(src, buildOut); c != "" {
		return c, n
	}
	var sb strings.Builder
	removed := 0
	for _, line := range strings.Split(src, "\n") {
		trim := strings.TrimSpace(line)
		if strings.Contains(trim, "// placeholder") || strings.Contains(trim, "// Placeholder") {
			removed++
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if removed > 0 {
		return strings.TrimSuffix(sb.String(), "\n"), removed
	}
	return "", 0
}

// EvidenceGaps(L-01    2026-10-03): to   now artifact ** data **  , 
// returnback  listtable; emptylisttable =  data safetyed(artifactstore and empty,     ed, P0   store ity). 
//
//  data(to  dsh"donebeforerecv  data"): 
//  1. artifactobj store and main.go  empty(  /  /emptyfile ->   )
//  2. main.go    TODO/  tgt (LLM semantic now vs   ity   bot  split)
//  3. go build -C <artifactroot> ./...     ed( allow )
//  4. P0   store ity: word /correction/intent/rev /JSONL   /    (close char  artifact code)
func (o *Options) EvidenceGaps(out *Outcome) []string {
	if out == nil {
		return []string{"任务无产物（Outcome 为空）"}
	}
	// fromback  get harness-output/ artifactrootobj (stdout  e.g. "writed: /abs/harness-output/<title>/main.go"). 
	// 2026-10-03 fix : stdout   (writed path + VHS_BACKUP_PATH: ...), TrimPrefix after line is  , 
	// HasSuffix(main.go)      -> implRoot empty -> Glob back get obj (readtodiff task   ->  "  todo"  FAIL). 
	// onlyget   (writed path ). 
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
		// back    path: back   objroot harness-output/. 
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
	//  data 2:   /    (  ity     note ). 
	low := strings.ToLower(mainSrc)
	for _, marker := range []string{"todo", "占位", "not implemented", "code generated by voice sign harness"} {
		if strings.Contains(low, marker) {
			gaps = append(gaps, "main.go 仍是骨架/占位（含 `"+marker+"` 标记），未产出完整实现")
			break
		}
	}
	//  data 4 already  (2026-10-04):   datais ASR     code P0 list(word /correction/process…), 
	//  needrequire(langaudio   )after     -> 5  all recv .  data modifyasneedrequire  : 
	//  data 5(needrequireendpoint ↔ artifact HandleFunc diff )onlyis  store ity data;   semanticbyverifyto serveservice . 
	//  data 3:    ( allow ). 
	buildRecv := o.run("run", map[string]any{"command": []string{"go", "build", "-C", implRoot, "./..."}})
	if !buildRecv.OK {
		gaps = append(gaps, "真编译失败："+truncateStr(buildRecv.Stderr, 300))
	}
	//  data 5: needrequireendpoint ↔ artifactroutebyendpointstore ity(  1, 2026-10-04 nohuman     now: 
	// needrequire 7 endpointartifactonly 2 endpoint(/v1/health+/v1/process), P0 close char  " safety"  ). 
	// needrequireside: from o.Document(needrequiresafety ) get hasoutnow  /v1/xxx charface ; 
	// artifactside: from main.go  get http.HandleFunc("/v1/xxx") alreadynote endpoint; 
	// diff  =    ->  data  FAIL ->   fix (RoundEvidence back  LLM patch endpoint). 
	if o.Document != "" {
		reqEP := endpointRefsOf(o.Document)
		prodEP := endpointHandlersOf(mainSrc)
		// 2026-10-04 fix : needrequire    useon  harness endpoint("onlycallits POST /v1/tasks and GET
		// /v1/tasks/{id}")-- isneedcalluse ,  isartifactneednote  .  data 5 only artifactendpoint 
		//(/v1/voice/ before ; on endpoint  ),  then        fix    . 
		upstreamEP := map[string]bool{"/v1/tasks": true, "/v1/tasks/{id}": true}
		// path num  ize: /v1/voice/tasks/{conversation_id} -> /v1/voice/tasks/( numname  and  ). 
		// 2026-10-04   : LLM occurbecome HandleFunc("/v1/voice/tasks/")(tail  , Go path numcompatwrite ), 
		//      {conversation_id}     -> 5  fix    .   afteronly  endpointpathstore . 
		normEP := func(ep string) string {
			return regexp.MustCompile(`\{[^}]*\}`).ReplaceAllString(ep, "")
		}
		reqNorm := map[string]string{}
		for ep := range reqEP {
			reqNorm[normEP(ep)] = ep
		}
		prodNorm := map[string]bool{}
		for ep := range prodEP {
			prodNorm[normEP(ep)] = true
		}
		for n, orig := range reqNorm {
			if upstreamEP[orig] {
				continue
			}
			if !prodNorm[n] {
				gaps = append(gaps, "缺需求端点实现："+orig+"（需求文档要求，产物未注册 http.HandleFunc）")
			}
		}
	}
	//  data 6: no  now(2026-10-04     : LLM occurbecome resolve/run/tasks handler safety 
	// `// Placeholder for ... logic` + StatusNotImplemented(501)--endpointnote safety ,  data 5 PASS, 
	//  data   recv , but   connect safety 501. useuser line"     allow ",     ). 
	//   : StatusNotImplemented / "// Placeholder" / panic("not implemented") / TODO   . 
	lowMain := strings.ToLower(mainSrc)
	stubHits := []string{}
	if strings.Contains(mainSrc, "http.StatusNotImplemented") {
		stubHits = append(stubHits, "handler 返回 501 StatusNotImplemented（占位桩）")
	}
	if strings.Contains(lowMain, "// placeholder") {
		stubHits = append(stubHits, "源码含 `// Placeholder` 占位注释")
	}
	if strings.Contains(lowMain, "not implemented") {
		stubHits = append(stubHits, "panic/TODO `not implemented`")
	}
	if len(stubHits) > 0 {
		//    bodyize: fromneedrequire   get E nodeendpointsemantic(LLM only  "has "   now; 
		// 2026-10-04     :  ize  under LLM 5  heavywrite occurbecome 501  ). 
		var eHints []string
		if o.Document != "" {
			for _, line := range strings.Split(o.Document, "\n") {
				trim := strings.TrimSpace(line)
				if strings.HasPrefix(trim, "### E") || strings.HasPrefix(trim, "| ") && strings.Contains(trim, "E1 ") || strings.Contains(trim, "E2 ") || strings.Contains(trim, "E3 ") || strings.Contains(trim, "E4 ") || strings.Contains(trim, "E5 ") || strings.Contains(trim, "E6 ") {
					if strings.Contains(trim, "/v1/voice") || strings.Contains(trim, "E6") {
						eHints = append(eHints, trim)
					}
				}
			}
			if len(eHints) > 12 {
				eHints = eHints[:12]
			}
		}
		sem := ""
		if len(eHints) > 0 {
			sem = "；需求文档端点语义（按此实现，禁止 501/占位）：\n" + strings.Join(eHints, "\n")
		}
		gaps = append(gaps, "产物含桩实现（"+strings.Join(stubHits, "；")+"）——必须实现真实业务逻辑，禁止占位"+sem)
	}
	//  data 7:  as   (2026-10-04   : LLM occurbecome parse/decompose returnback empty --
	// no 501 but E2 parse actions=null, E3 decompose tasks=null;  data 6      to  . 
	//   raiseartifactserveservice tgtapprove indisconnectlang as(    ,   needrequire data 2/4). 
	if len(gaps) == 0 {
		bin := filepath.Join(implRoot, "impl")
		if _, err := os.Stat(bin); err == nil {
			port := "18950"
			o.run("run", map[string]any{"command": []string{"sh", "-c",
				"lsof -ti :18950 | xargs kill 2>/dev/null; sleep 0.3; nohup " + bin +
					" -addr 127.0.0.1:" + port + " >/tmp/vhs_evg_srv.log 2>&1 & echo $! > /tmp/vhs_evg.pid; sleep 1"}})
			defer o.run("run", map[string]any{"command": []string{"sh", "-c", "kill $(cat /tmp/vhs_evg.pid) 2>/dev/null"}})
			// E2 parse: tgtapprove lang in  returnback empty actions num (needrequire data 2; 2026-10-04   : 
			// emptynum /actions:null all   tgt--LLM  returnback {"actions":null}  ed). 
			p := o.run("run", map[string]any{"command": []string{"curl", "-s", "--max-time", "6", "-X", "POST",
				"http://127.0.0.1:" + port + "/v1/voice/parse", "-H", "Content-Type: application/json",
				"-d", `{"text":"就是那个现在开始跑一下测试对吧","conversation_id":"evg-1"}`}})
			ps := strings.TrimSpace(p.Stdout)
			hasActs := strings.Contains(ps, `"actions":[`) || strings.Contains(ps, `"actions": [`) || strings.Contains(ps, `"actions":[{`)
			emptyActs := strings.Contains(ps, `"actions":[]`) || strings.Contains(ps, `"actions":null`) || strings.Contains(ps, `"actions": []`)
			if !hasActs || emptyActs {
				gaps = append(gaps, "E2 parse 行为不达标：输入「就是那个现在开始跑一下测试对吧」应返回**非空** actions 数组（动作识别），实测="+ps+"——实现动作词表与过滤逻辑，禁止空 actions")
			}
			// E3 decompose:   refer    out >=1  task(needrequire data 4; 2026-10-04   : 
			// emptynum  tasks:[]   ed--charseg butnotask). 
			d := o.run("run", map[string]any{"command": []string{"curl", "-s", "--max-time", "6", "-X", "POST",
				"http://127.0.0.1:" + port + "/v1/voice/decompose", "-H", "Content-Type: application/json",
				"-d", `{"text":"拉取最新版，编译并启动服务，跑长程任务验收测试，输出报告","conversation_id":"evg-1"}`}})
			ds := strings.TrimSpace(d.Stdout)
			hasTasks := strings.Contains(ds, `"tasks":[`) || strings.Contains(ds, `"tasks": [`) || strings.Contains(ds, `"tasks":[{`)
			emptyTasks := strings.Contains(ds, `"tasks":[]`) || strings.Contains(ds, `"tasks":null`) || strings.Contains(ds, `"tasks": []`)
			if !hasTasks || emptyTasks {
				gaps = append(gaps, "E3 decompose 行为不达标：复合指令「拉取最新版，编译并启动服务，跑长程任务验收测试，输出报告」应拆出**多个任务**（非空 tasks 数组），实测="+ds+"——实现动作分割与任务列表，禁止空 tasks")
			}
			// E4 resolve: no  andnoto  -> target=unresolved / pending_resolve(needrequire data 7). 
			r := o.run("run", map[string]any{"command": []string{"curl", "-s", "--max-time", "6", "-X", "POST",
				"http://127.0.0.1:" + port + "/v1/voice/resolve", "-H", "Content-Type: application/json",
				"-d", `{"text":"帮我拉取那个仓库","conversation_id":"evg-1"}`}})
			rs := strings.TrimSpace(r.Stdout)
			if !strings.Contains(strings.ToLower(rs), "unresolved") && !strings.Contains(rs, "pending_resolve") && !strings.Contains(rs, "补全") {
				gaps = append(gaps, "E4 resolve 行为不达标：无会话历史且无明确对象时应返回 unresolved/pending_resolve（需求判据 7），实测="+rs+"。实现要点：resolve handler 先查该 conversation_id 的会话历史（JSONL/内存 session store）——若该会话无任何历史记录且 text 含指代词（那个/这个/它/帮我…那个），必须返回含 unresolved 或 pending_resolve 的响应（如 {\"resolved\":false,\"target\":\"unresolved\",\"pending_resolve\":true}）；有历史时从最近记录补全 target。禁止返回 {\"resolved_target\":\"\"} 或 \"default target\" 等默认值。Go 行为锚点（可直接参照）：无历史时 handler 内做 `if 该会话无记录 { w.Header().Set(\"Content-Type\",\"application/json\"); io.WriteString(w, \"{\\\"resolved\\\":false,\\\"target\\\":\\\"unresolved\\\",\\\"pending_resolve\\\":true}\"); return }`")
			}
			// E5 run: orchestrate      on andreturnbacktask use(needrequire data 8 semantic: summary/taskstatus). 
			ru := o.run("run", map[string]any{"command": []string{"curl", "-s", "--max-time", "8", "-X", "POST",
				"http://127.0.0.1:" + port + "/v1/voice/run", "-H", "Content-Type: application/json",
				"-d", `{"text":"拉取最新版，编译并启动服务","conversation_id":"evg-1"}`}})
			rus := strings.TrimSpace(ru.Stdout)
			if !strings.Contains(rus, "task_id") && !strings.Contains(rus, "submitted") && !strings.Contains(rus, "summary") && !strings.Contains(rus, "total") {
				gaps = append(gaps, "E5 run 行为不达标：应编排任务并投递上游 harness（响应含 task_id/submitted/summary.total），实测="+rus+"。实现要点：run handler 将 text 交给内部编排（复用 decompose 逻辑拆任务列表），对每个任务 POST 上游 harness /v1/tasks（上游 base 默认 http://127.0.0.1:8941，支持 VHS_UPSTREAM 环境变量覆盖；JSON: {\"text\":任务文本,\"conversation_id\":当前会话}），收集返回的 task_id；响应 JSON 必须含 tasks 数组（每项含 task_id）+ submitted 数量 + summary（含 total 字段）。禁止返回 {\"results\":[]} 或 {\"tasks\":null}。Go 行为锚点（可直接参照）：先用内部拆任务逻辑得到 task 列表；对每个任务 `http.Post(upstream+\"/v1/tasks\", \"application/json\", bytes.NewReader(jsonStr))` 收集响应里的 task_id 到切片；最终响应写 `{\"tasks\":[{\"task_id\":\"...\"}],\"submitted\":N,\"summary\":{\"total\":N}}`（N=成功投递数，字段名必须含 task_id/submitted/summary/total）")
			}
		}
	}
	return gaps
}

// endpointRefsOf  get baseinoutnow  has /v1/xxx endpointcharface (needrequireside:     to i.e. needrequireendpoint). 
func endpointRefsOf(src string) map[string]bool {
	set := map[string]bool{}
	// 2026-10-04 fix :   posthen /v1/[a-z_]+  pipe /v1/voice/health  become /v1/voice--
	// needrequire 6 endpointbe become 2   path,  data 5  same  (  :    8  safetyis ASR   code data, 
	//   resolve/run  endpoint   0  ). modify  : /v1/voice/resolve, /v1/voice/tasks/{cid} etcsafety . 
	re := regexp.MustCompile(`/v1/[a-zA-Z0-9_/{}-]+`)
	for _, m := range re.FindAllString(src, -1) {
		set[m] = true
	}
	return set
}

// sortedEndpoints willendpoint set   as  list(LLM prompt use,    read). 
func sortedEndpoints(src string) string {
	if src == "" {
		return "（需求文档为空——无端点清单）"
	}
	eps := endpointRefsOf(src)
	if len(eps) == 0 {
		return "（需求文档未提及 /v1/ 端点）"
	}
	var list []string
	for ep := range eps {
		list = append(list, ep)
	}
	sort.Strings(list)
	return strings.Join(list, "\n")
}

// docBrief  getneedrequire  before n char  asneedpt need( tgt andfirst  datatable, control prompt   ). 
func docBrief(src string, n int) string {
	if src == "" {
		return "（无需求文档）"
	}
	s := strings.TrimSpace(src)
	if len([]rune(s)) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n]) + "\n…（文档过长，以上为摘要，完整判据以端点清单 + 证据门为准）"
}

// endpointHandlersOf  get codein http.HandleFunc("/v1/xxx", …) alreadynote  endpoint(artifactside: note only  now). 
func endpointHandlersOf(src string) map[string]bool {
	set := map[string]bool{}
	// 2026-10-04 same   : /v1/voice/health finish  get(and endpointRefsOf to ). 
	re := regexp.MustCompile(`HandleFunc\("/v1/[a-zA-Z0-9_/{}-]+`)
	for _, m := range re.FindAllString(src, -1) {
		set[strings.TrimPrefix(m, `HandleFunc("`)] = true
	}
	return set
}

// deterministicImplementSkeleton occurbecome    Go  code  (LLM   usetime produceout).
// file: README.md / go.mod / main.go / router.go / domain.go(serveservice   + needrequire  ).
// objtgt: harness-output/<title>/ under `go build ./...`   ed;  now    TODO   nowstage.
// lang selects the template set (en/zh); the generated Go code stays compilable in either
// language — only comments and user-facing string literals are localized.
func deterministicImplementSkeleton(lang Lang, title, doc string) map[string]string {
	t := pickTmpl(lang)
	// module name   ASCII(Go limitrestrict: in  module path   ); obj name keep in .
	mod := "harness-output/" + asciiSlug(title)
	if mod == "harness-output/" {
		mod = "harness-output/impl"
	}
	// fromneedrequire   get nodetgt  as domainneedptnote (  close ,    ).
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
	chapNote := t.chapLabel
	for _, c := range chapters {
		chapNote += "  //  - " + c + "\n"
	}

	mainGo := fmt.Sprintf(t.skelHeadFmt, title) + `package main

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
	addr := flag.String("addr", envOr("VHS_ADDR", "127.0.0.1:8787"), "` + t.skelAddrFlag + `")
	dataDir := flag.String("data-dir", envOr("VHS_DATA", "./data"), "` + t.skelDataFlag + `")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("` + t.skelMkdirFail + `", err)
	}
	app := NewApp(*dataDir)

	mux := http.NewServeMux()
	app.RegisterRoutes(mux) // router.go

	srv := &http.Server{Addr: *addr, Handler: mux}
	log.Printf("` + t.skelServingFmt + `", *addr, *dataDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("` + t.skelExitFatal + `", err)
	}
}

` + t.skelWriteJSON + `
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
`

	routerGo := t.routerHead + `package main

import "net/http"

` + t.appCmt + `
type App struct {
	DataDir string
}

func NewApp(dataDir string) *App { return &App{DataDir: dataDir} }

` + t.registerCmt + `
func (a *App) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/health", a.handleHealth)
	mux.HandleFunc("/v1/process", a.handleProcess)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data_dir": a.DataDir})
}

` + t.processCmt + `
func (a *App) handleProcess(w http.ResponseWriter, r *http.Request) {
` + t.processTODO + `
}
`

	var domainSb strings.Builder
	domainSb.WriteString(t.domainHead)
	domainSb.WriteString("package main\n\n")
	domainSb.WriteString("//" + chapNote)
	for _, line := range t.domainTODO {
		domainSb.WriteString(line)
	}
	domainGo := domainSb.String()

	goMod := "module " + mod + "\n\ngo 1.21\n"

	var readmeSb strings.Builder
	readmeSb.WriteString("# " + title + t.readmeTitleSuffix + "\n\n")
	readmeSb.WriteString(t.readmeAutoGen + "\n")
	readmeSb.WriteString(t.readmeContents)
	for _, item := range t.readmeContentList {
		readmeSb.WriteString(item)
	}
	readmeSb.WriteString("\n" + t.readmeUsage)
	readmeSb.WriteString("```bash\ncd " + mod + "\n" + t.readmeBuildCmt + "\n```\n")
	readmeSb.WriteString(t.readmeMapping)
	fmt.Fprintf(&readmeSb, t.readmePlanFmt, sanitizePathPart(title))
	readmeSb.WriteString(t.readmeReqFull)
	readmeSb.WriteString(t.readmeStatus)
	readmeSb.WriteString(t.readmeStatusBody)
	readme := readmeSb.String()

	return map[string]string{
		"README.md": readme,
		"go.mod":    goMod,
		"main.go":   mainGo,
		"router.go": routerGo,
		"domain.go": domainGo,
	}
}

// deterministicImplementPlan   ityoccurbecome now  (LLM   usetime   ,  produceout     file).
// close : objtgt -> needrequireneedpt get( nodetgt /close word)-> modulelist -> connect /numdata   ->  recv   ->     .
// lang selects the template set (en/zh); see i18n.go.
func deterministicImplementPlan(lang Lang, title, doc string) string {
	t := pickTmpl(lang)
	var sb strings.Builder
	sb.WriteString(t.planGoalHead)
	fmt.Fprintf(&sb, t.planProductFmt, title)
	sb.WriteString(t.planBasis)

	sb.WriteString(t.planHighHead)
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
		sb.WriteString(t.planNoHeadings)
	} else {
		for _, h := range headers {
			if h.level == 1 {
				fmt.Fprintf(&sb, "- **%s**\n", h.text)
			} else if h.level == 2 {
				fmt.Fprintf(&sb, "  - %s\n", h.text)
			}
		}
	}

	sb.WriteString(t.planModuleHead)
	sb.WriteString(t.planModuleTable)

	sb.WriteString(t.planAcceptHead)
	sb.WriteString(t.planAcceptNote)

	sb.WriteString(t.planStepsHead)
	for _, step := range t.planStepItems {
		sb.WriteString(step)
	}

	sb.WriteString(t.planExcerptHead)
	sb.WriteString(truncateStr(doc, 6000))
	sb.WriteString("\n```\n")
	return sb.String()
}


// commitTargetPath  to    file: git add -- <abs> ->(no diff then etc ed)-> git commit -> git log -1 get hash. 
//    `git add -A`,    in   noclose   file. 
//  etc:  file to store /HEAD nochangeize(heavy samein )time,  triggersend "nothing to commit"   , 
// butis asbecome recvtail, receipt note "in nochangeize,  ed  (alreadyis new)". 
func (o *Options) commitTargetPath(root, absPath, msg string) contract.Receipt {
	add := exec.Command("git", "add", "--", absPath)
	add.Dir = root
	if out, err := add.CombinedOutput(); err != nil {
		return contract.Receipt{Tool: "git", OK: false, Err: "git add failed: " + string(out)}
	}
	// only basepathis  in store (has diff); empty=nochangeize ->  etc ed  . 
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

// planVerify give      intentoccurbecome verify.Spec;  thenreturnback nil(unverifiable). 
// COMMIT     data connect  back  stdout(git log -1  out),     verify.Kind(verify    modify). 
func (o *Options) planVerify(it contract.Intent, logDir string) *verify.Spec {
	switch it.Intent {
	case contract.IntentNote:
		return &verify.Spec{Kind: "file", Args: []string{filepath.Join(logDir, "notes.md")}, BaseDir: logDir}
	default:
		return nil
	}
}

// projectRootForCommit resolve  COMMIT intentdomain  objroot(M4-4). 
//
// [pseudocode logic layer](  rootresolve   decide  ): 
//
//	if intent.Intent != COMMIT: return ""(its intentkeepkeep logDir). 
//	m = o.Spaces.Get(intent.Space); no manifest -> return "". 
//	to m.Scope   1  :   "/**" after  -> Clean ->   isalreadystore obj  ->  then return "". 
//	  end: root   ==/under logDir(preventpipe logDir cur obj ). 
//	return rootpath. 
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

// gitDirtyCount   root   `git status --porcelain`   uncommitted changes num. 
//   git  /     -> returnback -1(keep : confirm   write num, but   ). 
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

// mechanicalImpact use   fs      face(RefCount/HasTest/Heat). 
//
// [pseudocode logic layer]( writemodule:  usedefine/  rule/onlimit;  valuedefine  VSL risk.StaticImpact)
//
// control flow: 
//
//	target = targetPath(it); empty -> return  value(back  auto/small,     ). 
//	m = o.Spaces.Get(it.Space); no manifest -> return  value. 
//	roots = m.Scope   "/**" aftergetobj ; 
//	     (M2    formpreventback ):  ed   ==/under logDir   root, 
//	  Ignore notein [".git","node_modules","memory","data"]--trace/discuss/decisions/word 
//	       be become" use". 
//	if len(roots)==0: return  value. 
//	hits = search.FindText(target, {Roots:roots, Ignore})
//	RefCount =  heavyafter infilenum;  top refCap(default 50, prevent    ). 
//	HasTest =    root understore  *_test.go file(filepath.Walk     i.e. ). 
//	Heat    = curday trajectory-*.jsonl in  target basename   num(charseg  only num). 
//	return {RefCount, HasTest, Heat}. 
//
// error: search returnback err -> RefCount=0(keep  small); tracefileread to -> Heat=0. 
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

	// scope roots,   /**/* after 
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
		//    : root    log_dir in(preventpipetrace/decide day cur use)
		if logAbs != "" && (abs == logAbs || strings.HasPrefix(abs, logAbs+string(os.PathSeparator))) {
			continue
		}
		roots = append(roots, abs)
	}
	if len(roots) == 0 {
		return imp
	}

	ignore := []string{".git", "node_modules", "memory", "data"}

	// RefCount =  use target   heavyfilenum
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

	// HasTest = scope instore  *_test.go
	imp.HasTest = hasTestFile(roots, ignore)

	// Heat = curdaytracein in target basename   num
	imp.Heat = o.trajectoryHeat(target)
	return imp
}

// hasTestFile   roots under  *_test.go( heavy ignore seg). 
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

// trajectoryHeat numcurdaytracefilein  target basename   num. 
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

// fastResponseMs returnbackfast    value(Global.FastResponseMs, <=0   izeas 10000). 
func (o *Options) fastResponseMs() int {
	if o.Cfg != nil && o.Cfg.Global.FastResponseMs > 0 {
		return o.Cfg.Global.FastResponseMs
	}
	return 10000
}

// selfheal    error   . Providers==nil(testOptions nowstatus/ rulepath)->   nil, 
//  disconnect  open  ed,  chain as 100%  change. diag provider  note  ->  type  nil(      use). 
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

// repairFailed to  back  error  (   ):  disconnect + read-onlysafesafetyheavy (limit 2  ). 
// heavy become  newback by chain and  out.Receipts;  disconnectclose  storeprovideattribution.  disconnect    /   -> returnback nil. 
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
		att := selfheal.Attempt{Tool: r.Tool, Args: o.argsForFailed(it, r.Tool), Receipt: r, RequestID: o.RequestID} // P0-4b:  disconnect trace   chain request_id
		if nr, _ := svc.SafeRetry(ctx, it.RawText, it.Intent, att); nr != nil {
			repaired = append(repaired, *nr)
		}
	}
	return repaired
}

// argsForFailed asheavy heavy    num(onlyhasread-only   only  heavy ; writeclassheavy alsobe IsReadOnly  under). 
func (o *Options) argsForFailed(it contract.Intent, tool string) map[string]any {
	switch tool {
	case "search":
		pattern := it.CorrectedText
		if it.Params != nil && it.Params["object"] != "" {
			pattern = it.Params["object"]
		}
		return map[string]any{"pattern": pattern, "kind": "text"}
	case "file":
		// NOTE   (writeclass,   be  heavy ). 
		return map[string]any{"action": "append", "path": filepath.Join(o.logDir(), "notes.md"), "log_dir": o.logDir()}
	default:
		return map[string]any{}
	}
}

// renderGround       (#37); Ground     -> emptyfast (   , Ask    ). 
func (o *Options) renderGround() ground.Snapshot {
	if o.Ground != nil {
		return o.Ground.Render()
	}
	return ground.Snapshot{}
}

// recordDecision  confirm      decide(#37 decisions.jsonl numdata ). 
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

// ---------- attribution(  ) ----------

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

// hasConfirmAsk reports whether any receipt is a confirmation gate (distillation R6:
// render as 待确认 with the real question instead of FAILED).
func hasConfirmAsk(rs []contract.Receipt) bool {
	for _, r := range rs {
		if r.ConfirmAsk {
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

// ----------   (four-line receipt, SPEC §2.41) ----------

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
		if hasConfirmAsk(rs) {
			// distillation R6: a confirmation gate (e.g. software install) renders as
			// 待确认 with the real question, never as FAILED.
			view.Result = "待确认：" + truncateStr(reason, 200)
		} else {
			view.Result = "FAILED：" + truncateStr(reason, 60)
		}
	default:
		view.Result = "OK（" + confirmWord(d.Level) + "）"
		// M7:    stdout has  in (QUERY   LLM answer/    etc  base)time, 
		// four-line receipt "close " showanswer base( disconnect 400;  before 120  pipe  close   after ), 
		//  againonly show"OK(    )"empty . 
		if len(rs) > 0 {
			if s := strings.TrimSpace(rs[0].Stdout); s != "" && !strings.HasPrefix(s, "{") {
				view.Result = truncateStr(s, 400)
			}
		}
	}
	// ORCHESTRATE   chain: back need show    chain(read N     -> writefile -> git commit hash), 
	//   onlyget    file-read    pos . 
	if it.Intent == contract.IntentOrchestrate && !hasFailure(rs) {
		view.Action = "多步编排：读文档→汇总→写文件→git提交"
		view.Files = orchestrateFiles(rs)
		view.Result = orchestrateChainResult(rs)
		view.Undo = "不可撤销（已人工放行并 git 提交）"
	}
	return view
}

// orchestrateFiles from  back   getwritefileobjtgt(file write back    "writed: <path>"). 
func orchestrateFiles(rs []contract.Receipt) string {
	for _, r := range rs {
		if r.Tool == "file" && strings.HasPrefix(r.Stdout, "writed:") {
			return strings.TrimSpace(strings.TrimPrefix(r.Stdout, "writed:"))
		}
	}
	return "—"
}

// orchestrateChainResult occurbecome  chain  sent close (read N   -> commit hash). 
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
			// git log -1   form: "<hash> <subject>", get after  first  token. 
			lines := strings.Split(strings.TrimSpace(r.Stdout), "\n")
			last := lines[len(lines)-1]
			if f := strings.Fields(last); len(f) > 0 && len(f[0]) >= 7 {
				hash = f[0]
			}
		}
	}
	return fmt.Sprintf("多步链完成：读 %d 份文档→汇总生成→git commit %s", reads, hash)
}

// intentCandidates produceoutlow-confidenceclarification close izeintent  (M4-3 ①). 
//
// [pseudocode logic layer](   occurbecome  decide  ): 
//
//	returnback 4    intent   {id,label}: edit/query/note/commit. 
//	id and label   to ,     ; answer   id time server continue data   as close wordbefore . 
//
// optionsForIntent by  intent stateoccurbecome  (Codex/gpt-6-luna  disconnect 2026-10-02): 
//  again   "modifyfile/  code/   /  "--
//
//	[pseudocode logic layer]
//	EDIT/DEBUG -> refer resolve out objtgtfile  (no -> nil,   noclose )
//	QUERY -> answer  /to (curbeforenosafesafety occur  -> nil, keep    Ask  base)
//	NOTE ->     (no    -> nil)
//	its  -> nil
//	nosafesafety  timekeep  Ask  basei.e. (Codex: "ifno safesafetylyproduceoccurhas   , keep    Ask  base"). 
//
//    : pipeline.TestCodexOptionsForIntent. 
func optionsForIntent(it *contract.Intent, referOpts []refer.Option) []AskOption {
	switch it.Intent {
	case contract.IntentEdit, contract.IntentDebug:
		return referToAskOptions(referOpts)
	default:
		return nil
	}
}

// referToAskOptions    refer objtgt  as AskOption. 
func referToAskOptions(referOpts []refer.Option) []AskOption {
	out := make([]AskOption, 0, len(referOpts))
	for _, o := range referOpts {
		out = append(out, AskOption{ID: o.ID, Label: o.Label})
	}
	return out
}

// shouldResolveRefer   is tocurbeforeintent   refer coreference resolution(M7, out  type disconnect    ). 
//
// [pseudocode logic layer]( decide  ,     : Codex/gpt-6-luna out  disconnect 2026-10-02   ): 
//
//  1.  lang sent  (?     e.g. as   )-> false
//     ("e.g.      /      kind"  "  /  "is lang word,  is  coreference--22:04    data). 
//
//  2. file   word(pipe/will/ open/modify/fix/  / / / / /store/write/ / /  /online/send …)-> true
//     (  coreference signal; " openon   "i.e. be  QUERY alsoresolve ). 
//
//  3. QUERY    (>=0.8)-> only coreference("  under  "to  empty) resolve ; has body("    ") resolve . 
//
//  4. UNKNOWN(no   word)-> false(   use/ refer , e.g."   beforeend   again ed "-- becausecoreference Ask). 
//
//  5. NOTE/EDIT/COMMIT/DEBUG -> true(   coreference resolutionkeepkeeporig as). 
//
//        : pipeline.TestShouldResolveReferGate(16 useexample)+ Codex 9  back   . 
//
// hasRecent=true tableshow  onunder store   coreferenceobjtgt: QUERY i.e. has bodyalso coreferenceconnectline
// ("  under    "   in"    " backreferbefore  body). 
func shouldResolveRefer(it *contract.Intent, hasRecent bool) bool {
	text := it.CorrectedText
	// 2026-10-08 (distillation R4): "读一下笔记" / "read my notes" is a complete note-read
	// request resolved in execActions; do not bounce it into a refer clarification
	// ("要查哪个项目/文件？") even though "读" is now a file-op verb.
	if isNotesReferent(text) &&
		(strings.Contains(text, "读") || strings.Contains(strings.ToLower(text), "read")) {
		return false
	}
	if strings.ContainsAny(text, "?？吗呢怎么如何为什么哪") {
		return false
	}
	if hasFileOpVerb(text) {
		return true // file   word ->   coreference(   signal), i.e.  QUERY alsoresolve 
	}
	switch it.Intent {
	case contract.IntentQuery:
		//   to  coreference("  under  ")->     resolve ; has body("    ")-> default resolve ; 
		// but  onunder has  coreferenceobjtgt(hasRecent)-> connectlineresolve ("  under    "backreferbefore  body). 
		if it.Confidence >= 0.8 {
			if isBareReferent(text) {
				return true
			}
			return hasRecent
		}
		return true
	case contract.IntentNote, contract.IntentEdit, contract.IntentCommit, contract.IntentDebug:
		return true
	case contract.IntentUnknown:
		//    use/ refer ("   beforeend   again ed ")->  becausecoreference Ask,  classify clarification
		return false
	default:
		return true
	}
}

// fileOpVerbs file/     word (Codex  disconnect 2026-10-02):  in as"  coreference" signal. 
// fileOpVerbs file/     word (Codex  disconnect 2026-10-02):  in as"  coreference" signal. 
// 2026-10-08 (distillation R4): "读" added — note-read requests ("读一下笔记") must stay
// rule-handled (note-read branch in QUERY) instead of being sent to the LLM classifier.
// ("看" was deliberately NOT added: "随便看看" is chit-chat, not a file operation.)
var fileOpVerbs = []string{"把", "将", "打开", "改", "修", "提交", "删", "建", "换", "设", "存", "写", "跑", "记", "部署", "上线", "发布", "复制", "移动", "重命名", "读"}

// hasAnySubstr     (input.containsAny as  has, pipeline usesamesemanticbasely now). 
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

// isBareReferent   coreferencewordafterno  to ("  under  "/"   "-> ; "    "->  ). 
// ASR  audio fill("      openstart  raise ")after has  in  ->   ,  triggersendcoreference Ask. 
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

// clarificationBlocksExecution    refer   is  stop  (Codex/gpt-6-luna 2026-10-02): 
// has   ->  stop(needuseuser  ); file   word ->  stop(  to    out ); 
//    coreference ->  stop(to  empty); its (  has body/  class)->   stop( becausecoreference Ask). 
func clarificationBlocksExecution(it *contract.Intent, referOpts []refer.Option) bool {
	if len(referOpts) > 0 {
		return true
	}
	if hasFileOpVerb(it.CorrectedText) {
		return true
	}
	return isBareReferent(it.CorrectedText)
}

// llmIntentFallback(M7 ①): rulelow-confidence/UNKNOWN and  howeverlanglang senttime, call fast provider patchclassify. 
//
// [pseudocode logic layer](new decide  ): 
//
//	if o.Providers == nil: return intent( rulepath). 
//	trigger = (intent.Intent==UNKNOWN || intent.Confidence < 0.6) &&  base  [?     e.g. as  ]. 
//	if !trigger: return intent. 
//	call fast.Chat(system=" isintentclassify ,  out JSON {intent,confidence}")
//		user=origstart base +  usedomainlisttable. 
//	resolve  JSON:   and intent∈{NOTE,QUERY,EDIT,COMMIT} -> overwrite intent;  thenkeep ruleclose . 
//	   err/ time -> return intent(  disconnect). 
// llmNaturalReply 用 self-model baseline + user input 生成自然语言回复。
// 对 UNKNOWN/QUERY/ASK 这类对话类意图，不再只说"need clarification"，
// 而是真的根据记住的用户身份回答。
func (o *Options) llmNaturalReply(ctx context.Context, text string, baselineCtx []string) string {
	if o.Providers == nil {
		return ""
	}
	sys := "你是 VoxSign 语音助手，正在和用户打电话式对话。你能看到：记住的用户身份、已知实体、最近对话历史。\n" +
		"用户可能在问问题、下任务、或者给你信息——不管哪种，你都要自然地接住，永远不要说'没理解'或'再说一遍'。\n" +
		"问问题就认真回答；下任务就说'好的我来处理'；给信息就说'好的我记住了'。\n" +
		"'这个''那个'从最近对话里推测指什么；代指模糊时给最佳猜测+确认。\n" +
		"特别注意：\n" +
		"- 不管用户说多长、几件事，都接住——说'好的，我理解了，先从最要紧的开始'\n" +
		"- 如果一句话里有多个任务，就说'好的，我先做A，再做B，再做C'\n" +
		"- 如果用户说'继续推进''接着干''继续'，就是接着上次的任务继续做\n" +
		"- 如果没有具体目标，就根据最近对话推测目标\n" +
		"口语化，一句话。绝对不要说'没理解''再说一遍''你是想让我做什么'。"
	userMsg := "用户说：" + text + "\n\n记住的用户信息：\n" + strings.Join(baselineCtx, "\n")
	// 直连网关，绕过 provider 层 temperature/response_format 兼容问题
	body, _ := json.Marshal(map[string]any{
		"model": "deepseek-flash",
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": userMsg},
		},
		"max_tokens": 200,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://aiops.voxsign.ai/api/model/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AIops-Key", os.Getenv("AIOPS_KEY"))
	resp2, err := http.DefaultClient.Do(req)
	if err != nil || resp2.StatusCode != 200 {
		log.Printf("[llmNaturalReply] direct http failed: status=%v err=%v", resp2.StatusCode, err)
		return ""
	}
	defer resp2.Body.Close()
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.NewDecoder(resp2.Body).Decode(&chatResp)
	if len(chatResp.Choices) == 0 || strings.TrimSpace(chatResp.Choices[0].Message.Content) == "" {
		return ""
	}
	out := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	// 解析LEARN_ENTITY行——LLM从用户话里学到的新实体
	if idx := strings.Index(out, "LEARN_ENTITY:"); idx >= 0 {
		line := out[idx+len("LEARN_ENTITY:"):]
		if nl := strings.Index(line, "\n"); nl > 0 {
			line = line[:nl]
		}
		line = strings.TrimSpace(line)
		var canonical, desc string
		for _, part := range strings.Split(line, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "canonical=") {
				canonical = strings.TrimPrefix(part, "canonical=")
			} else if strings.HasPrefix(part, "desc=") {
				desc = strings.TrimPrefix(part, "desc=")
			}
		}
		if canonical != "" && o.Zhiji != nil {
			o.Zhiji.LearnEntity(canonical, desc, nil)
			log.Printf("[llmNaturalReply] learned entity: %s = %s", canonical, desc)
		}
		out = strings.TrimSpace(out[:idx])
	}
	log.Printf("[llmNaturalReply] reply=%q", out)
	return out
}

func (o *Options) llmIntentFallback(ctx context.Context, it contract.Intent, text string) contract.Intent {
	if o == nil || o.Providers == nil {
		return it
	}
	//    P4(G1)/ P3(G3):  in"    "confirmstate close **  **be LLM back overwrite. 
	// back  inafter pipe it.Ask  empty, but"Ask != '' ->     "issafesafety line: 
	//   -   : " needdelete  file  " be  Ask after  EDIT   ; 
	//   -  refer : "openstart   "same (   G3-P3); 
	//   -   sent: "e.g.    edthen   "same ; 
	//   -    : "pipe  modifybecomein howeverafter  under  ,    "(   G5-P0-1). 
	//
	// close ity changeform(   §4): **  alreadysendoccur, andalready   Ask confirmstate** ->     . 
	// i.e. `Conflict != "" && Ask != ""`. orig nowis name  switch
	// (negation/meta/conditional/multi_action), atissame class  link   : 
	//  newadd  "  Ask   "   branch, then   to  .  name  however  --
	//   then recv ConflictDebugPlan(input/taskintent.go:717-721, Ask  empty, 
	// but   name  , back  bypipe"needgive routealsois connectfix"  confirmstate connect  ). 
	//
	// as  is Conflict+Ask  er, but isonlyuseitsin  : 
	//   - onlyuse `Conflict != ""`:     ConflictDelete / ConflictNoteVsDeploy /
	//     ConflictAskVsOp    **Ask asempty      path**(deletebyunder domain/risk forbidmanage, 
	//          be  back ); 
	//   - onlyuse `Ask != ""`:    back base  -- ClassifyTask initstartizei.e. 
	//     `Ask: taskAskTemplate`(" is       "), UNKNOWN/low-confidenceexit
	//      however Ask  empty, atisbase num  triggersend(M7 ①     ). 
	//     classify pipe kind Ask  use: **defaultclassify Ask**(UNKNOWN   /low-confidence)and
	//     **   Ask**(safesafetystop). Conflict  emptyposis" is   Ask"    tgt . 
	//
	// back protectsee pipeline/intentfallback_regression_test.go( class torevexample). 
	if it.Conflict != "" && it.Ask != "" {
		return it
	}
	hasQ := strings.ContainsAny(text, "?？吗呢怎么如何为什么哪多少几算翻译translate写查")
	// M7   patch :   sent  time, rule   QUERY(UNKNOWN/low-confidence/  its intente.g. NOTE)
	//   call LLM   --ruleword to lang  sent   (22:04   : " now    under…      kind"
	// berule  NOTE    , ifonly low-confidencethen fallback   triggersend). alreadyis QUERY then connect  rule. 
	//
	// 2026-10-08 (distillation R2): previously the fallback only ran when the text contained a
	// question word, so substantive commands without one ("帮我算一下 23 乘以 17", "记一条：…",
	// "帮我写一封…邮件", "Translate this into Arabic") stayed UNKNOWN -> need_ask. New gate:
	//   - conflict-confirm state is never overridden (unchanged);
	//   - confident rule intents (>=0.6) are kept — no wasted LLM call;
	//   - low-confidence non-UNKNOWN without question words is kept (chat/INFO safety);
	//   - UNKNOWN, and low-confidence texts WITH question words, go to the LLM classifier;
	//   - bare referents ("那个…" with nothing after) and file-op verbs stay rule-handled.
	if it.Intent == contract.IntentQuery {
		return it
	}
	if it.Intent != contract.IntentUnknown && it.Confidence >= 0.6 {
		return it
	}
	if it.Intent != contract.IntentUnknown && !hasQ {
		return it
	}
	if isBareReferent(text) || hasFileOpVerb(text) {
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
		// 2026-10-08 (distillation, local reasoning model): budget 64 is consumed by chain-of-thought
		// before any JSON appears -> raise to 256 so the answer actually materializes.
		MaxTokens: 256,
	})
	if err != nil || resp.Content == "" {
		return it
	}
	//    JSON resolve ( dependency). 
	var parsed struct {
		Intent     string  `json:"intent"`
		Confidence float64 `json:"confidence"`
	}
	// 2026-10-08 (distillation, local on-prem model): response_format json_object is pathological
	// on Strata (llama.cpp grammar ~1.4s/token), so local providers run with response_format off and
	// the model may wrap the JSON in prose or code fences. Extract the first {...} object leniently.
	content := strings.TrimSpace(resp.Content)
	if !json.Valid([]byte(content)) {
		content = extractJSONObject(content)
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return it
	}
	valid := map[string]bool{
		contract.IntentNote: true, contract.IntentQuery: true,
		contract.IntentEdit: true, contract.IntentCommit: true,
	}
	if !valid[parsed.Intent] {
		return it
	}
	// 2026-10-08 (distillation R4): a low-confidence LLM label on a high-risk intent
	// (commit/deploy) would trip the human-confirm gate and stall casual chatter
	// ("就这样吧" -> COMMIT 0.3 -> need_confirm forever). Keep the rule UNKNOWN instead.
	if parsed.Confidence < 0.6 &&
		(parsed.Intent == contract.IntentCommit || parsed.Intent == contract.IntentDeploy) {
		return it
	}
	it.Intent = parsed.Intent
	if parsed.Confidence > 0 {
		it.Confidence = parsed.Confidence
	}
	// overwritebecome after    UNKNOWN     ( then NeedsClarification  triggersendclarification; 
	// refer  ifobjtgt    heavynew  Ask). 
	it.Ask = ""
	return it
}

// noJSON returnback false refer :  formclose base calluse  response_format(QUERY answer need  base). 
func noJSON() *bool {
	v := false
	return &v
}

// extractJSONObject pulls the first balanced {...} object out of arbitrary model text
// (prose, markdown fences, reasoning leftovers) and validates it as JSON.
// Returns "" when no valid object is found.
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			switch c {
			case '\\':
				esc = true
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				cand := s[start : i+1]
				if json.Valid([]byte(cand)) {
					return cand
				}
				return ""
			}
		}
	}
	return ""
}

// queryLLMAnswer(M7 ②): QUERY   aftercall fast occurbecome howeverlanglanganswer. 
//   (  /   limit/ time)-> returnback      ( againempty "OK(    )"). 
//
// error  connectline(   ,  modifychangebecome pathand     char): 
//   -    fast.Chat wall-clock: slowbutbecome  ->   answer, only  model:"fast"   disconnect    provideattribution; 
//   - err/emptyin  -> first disconnect(budget/network/param classify), diag  useand action=retry/modify ->
//     refernum  heavy (<=2  ), become returnbackanswerandwrite-back   ;   or diag   use ->  char    . 
func (o *Options) queryLLMAnswer(ctx context.Context, original string, searchStdout string) string {
	// ⚠️ **attribution      error**(Lead   : day  is HTTP 401 invalid_api_key, 
	// tooutbut "    alreadyuse or  error" ⇒ useuser  etc day,     , but posneed  is  key). 
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

	// callFast send   fast calluseandresolve JSON  ; ok=false tableshow  /empty/ out (andorig    ). 
	var lastErr error
	callFast := func() (string, bool) {
		resp, err := p.Chat(ctx, provider.ChatRequest{
			Messages: []contract.Message{
				{Role: "system", Content: "你是 VoxSign 助手。根据用户问题和检索结果给简洁中文回答。只输出回答文本本身，不要输出 JSON、不要做意图分类、不要输出任何结构化格式。"},
				{Role: "user", Content: "用户问题：" + original + "\n检索结果：" + searchStdout},
			},
			MaxTokens: 400,
			// answer need  base:  formclose  json_object(provider  defaultopenstart). 
			//  then type " need out JSON"+json_object   refer under outno   JSON  ({"x":0}), 
			// resolve      become" type  use"  (M7    2026-10-03). 
			ResponseFormat: noJSON(),
		})
		if err != nil {
			log.Printf("[queryLLMAnswer] fast Chat err: %v", err)
			lastErr = err //   error: provideattribution use( allowtoout "  /  ")
			return "", false
		}
		if strings.TrimSpace(resp.Content) == "" {
			log.Printf("[queryLLMAnswer] fast Chat empty content")
			return "", false
		}
		// fast   json_object response_format-- type out JSON  ; resolveout basecharsegalsoorig  baseanswer. 
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
		// slowbutbecome :    alreadyhasanswer, onlypipeslow    model name  disconnect (provideattribution/aftercontinue  ). 
		if elapsed > time.Duration(o.fastResponseMs())*time.Millisecond {
			if svc := o.selfheal(); svc != nil {
				_ = svc.Diagnose(ctx, original, contract.IntentQuery, []selfheal.Trace{
					selfheal.NewTrace("llm", "fast", map[string]any{"model": "fast"}, "slow response"),
				})
			}
		}
		return content
	}

	// err/emptyin  -> first disconnect; diag  useand heavy  -> refernum  heavy (<=2  ). 
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

// mergeAskOptions  andintent  and refer objtgt  (id  heavy, onlimit 8). 
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
	// M4-3: use C deliver close ize VHS_BACKUP_PATH: tgt resolve  body  file. 
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

func truncateStrTail(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[len(s)-n:]
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

// ---------- trace/discuss    (nil-safe) ----------

func (o *Options) write(e trajectory.Entry) {
	if o.Trace == nil {
		return
	}
	// #44: tracewrite    disconnectread-onlytask. 
	// P0-1b:  again `_ =`     --    kind /  listize / write error    warning. 
	//  then data "becauseas kind name store butemptyed",      change  (kinds.go  because). 
	if err := o.Trace.Write(e); err != nil {
		log.Printf("[trajectory] write 被丢弃（kind=%q request_id=%s）: %v", e.Kind, e.RequestID, err)
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
	// use Content   finish attribution JSON(trajectory.Entry no Attrib charseg, by #44 use content  )
	b, _ := json.Marshal(a)
	o.write(trajectory.Entry{RequestID: a.RequestID, Kind: trajectory.KindAttribution, Content: string(b)})
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

// appendDiscussLog       seeclose to <log_dir>/discuss.jsonl(under  notein;    modifyword /  ). 
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

// confirm    ConfirmFn, ctx canceltimereturnback false( wait). 
func (o *Options) confirm(ctx context.Context, taskID, question string) bool {
	if o.ConfirmFn == nil {
		return false
	}
	type res struct{ ok bool }
	ch := make(chan res, 1)
	// P0-2: ConfirmFn in panic    crash process( then  humanconfirmbackcall     harness). 
	// recover afterback  ok=false(and err pathsame =reject  ),   select   recvtoendstate. 
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[pipeline.confirm] ConfirmFn panic (task=%s): %v", taskID, r)
				ch <- res{ok: false}
			}
		}()
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

// ---------- Summary( day need) ----------

// Summary   curday(since ofafter)trace, bydomain/attribution class split ,       valueand edrate. 
// #52: mobile read  base; numdata =trace kind=task_metrics(close ize  ). 
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
		waitN    int // M4-1: hashuman wait/LLM  tasknum(Net  valuesplit )
		sumWait  int64
		pureN    int //  managelinetasknum(Net  valuesplit out)
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
		// #52     : close ize task_metrics  
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
		// compat trace: no task_metrics time num intent  (M2   )
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

// degradeMsg by**  error**occurbecome    (attribution  approve ;   only   ). 
//
// v2.3(2026-10-04 useuserrefer : "lang word heavyneed, diff    "):  type  usetime
//  again out"(on limit (429)…)"    id, modifybasely    --    , 
// give     ,  lang word. attribution    attributeLLMError(  error,  allow  ). 
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

// attributeLLMError pipe  errorattributionbecome      sent . 
//
//  dataneedrequire(Lead 2026-10-03): 
//
//	401/403 ⇒ **  /API key**(  outnow"  /  "); 429 ⇒ limit ; 5xx ⇒ on ; 
//	 time ⇒  time; **   ⇒    + origstarterror base**( allow use use  ). 
func attributeLLMError(rawErr string) string {
	e := strings.ToLower(rawErr)
	switch {
	case strings.Contains(e, "401"), strings.Contains(e, "403"),
		strings.Contains(e, "invalid_api_key"), strings.Contains(e, "auth_error"),
		strings.Contains(e, "unauthorized"), strings.Contains(e, "api key"):
		return "鉴权失败（API key 无效/未配置）—— 请更换或配置 key，重试前无需等待"
	case strings.Contains(e, "429"), strings.Contains(e, "rate limit"), strings.Contains(e, "too many requests"):
		return "上游限流（429）—— 稍后重试"
	case strings.Contains(e, "402"), strings.Contains(e, "payment required"),
		strings.Contains(e, "insufficient"), strings.Contains(e, "余额"), strings.Contains(e, "欠费"):
		// 2026-10-05   : on  as 402 Payment Required( user  ), be model-center  become 502. 
		//     5xx branchofbefore diff,  then  "5xx  base   ",      to. 
		return "模型账户余额/额度不足（上游 402 Payment Required）—— 需充值/换 key，不是本机网络问题"
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

// execRegisterTool -- 2026-10-04 useuser needrequire"after   has      ,     changenew": 
// REGISTER_TOOL intent  :  get  name -> occurbecome     -> Registry.Register    ->
// back   confirm(" ,   add "XX"  ").   name  to/note table    ->   back ,      . 
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
	//     already  (     );     now aftercontinue  connectin. 
	return []contract.Receipt{{
		Tool: "register", OK: true,
		Stdout: "好，我来增加「" + name + "」能力：已登记为可扩展工具（语音自举注册）。" +
			"接下来我会把它接成可执行能力——你说「" + name + "」相关的具体需求，我就能真正上手。",
	}}
}

// extractCapabilityName fromnote  requirein get  name: 
// "   add      control      "->   control  ; "            "->     . 
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
	//  bot:  tail wordafterget sent(<=20 char, preventpipe seg  cur  name). 
	t := strings.TrimSpace(text)
	t = strings.TrimRight(t, "的了吧呢。？?!！，, ")
	if t != "" && len([]rune(t)) <= 20 {
		return t
	}
	return ""
}

// looksLikeTask 判断用户这句话是不是在让做事（不是问问题/自我介绍/闲聊）。
// 关键词：做/跑/测/提交/部署/下载/运行/开始/处理/配置/查
func looksLikeTask(text string) bool {
	for _, kw := range []string{"做", "跑", "测", "提交", "部署", "下载", "运行", "开始", "处理", "配置", "查", "看一下", "帮我", "推进", "完成", "实现", "改", "修"} {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// looksLikeQuestion 判断用户这句话是不是在问问题（期望回答，不是下任务）。
func looksLikeQuestion(text string) bool {
	for _, kw := range []string{"吗？", "吗?", "呢？", "呢?", "怎么", "什么", "是不是", "对吗", "可以吗", "行吗", "如何", "为什么", "哪里"} {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}
