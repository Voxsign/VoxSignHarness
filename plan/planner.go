// planner.go -- rule  (VHS-PLAN-001 §4: PL-1..PL-5). 
//
// base  nowis**ruleform** :  line,   ity, onlyuse Manifest in     . 
//   calluse type -- PL/RV  dataneedrequire  now,  typeform planner dayoccur   , 
//   as**add **   data(     9  ). 
//
// PL-2(       reject)isrev     : list  has    =   to. 
package plan

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotImplemented keep give   now path(curbeforerule  already now,  againreturnback ). 
var ErrNotImplemented = errors.New("plan: not implemented")

// ErrUnachievable tableshow out   boundaryand rule numuse  --   e.g.  "  to". 
var ErrUnachievable = errors.New("plan: beyond capability boundary, cannot do")

// Step is  in   :     tolistin     , and num  . 
type Step struct {
	Tool      string            `json:"tool"`             //   is Manifest.Tools     name
	Caps      []string          `json:"caps"`             // useto  cap(e.g. git   commit)
	Params    map[string]string `json:"params"`           //    num(PL-3:   empty )
	Action    string            `json:"action"`           //      (  is"split  under" classempty )
	Output    string            `json:"output"`           // produceout (PL-3)
	Why       string            `json:"why,omitempty"`    // PM-5:      data(  is  )
	Domain    string            `json:"domain,omitempty"` // PM-4: objtgtdomain(base by AllowedSpaces   )
	DependsOn []int             `json:"depends_on,omitempty"`
}

// ConsideredItem is"  rule **    need **"(P2 firstdecide  :  has  face, W_*   out ). 
type ConsideredItem struct {
	Element  string `json:"element"`
	Source   string `json:"source"`   // WM-3:     has  
	Inferred bool   `json:"inferred"` //  disconnect  (but   )
}

// WMStats is       refertgt(WORKMEM-001 §3). 
type WMStats struct {
	Max       int      `json:"w_max"`                  //   rule sametime use need onboundary(  )
	Used      int      `json:"w_used"`                 //   useto
	Drop      int      `json:"w_drop"`                 // because   be   need num
	DropTrace []string `json:"w_drop_trace,omitempty"` // WM-2:       
	ChainLen  int      `json:"chain_len"`              //     to   
	// WMCAP   (VHS-WMCAP-001 C1/C2/WMC-5):   changeize   resolve . 
	Capacity    int     `json:"capacity,omitempty"`
	DemandFloor int     `json:"demand_floor,omitempty"`
	Familiarity float64 `json:"familiarity,omitempty"`
	Capped      bool    `json:"capped,omitempty"`
}

// Plan is  rule  close . 
type Plan struct {
	Goal    string   `json:"goal"`
	Steps   []Step   `json:"steps"`
	Refused bool     `json:"refused"`           // PL-2:    time  as true
	Missing []string `json:"missing,omitempty"` // PL-2: rejecttime   
	Reason  string   `json:"reason,omitempty"`
	Voided  bool     `json:"voided,omitempty"`  // RV-3: bedomain forbidreject ->     
	Changes []string `json:"changes,omitempty"` // RV-2:  ruletime    "change  "

	Considered     []ConsideredItem `json:"considered,omitempty"`      //   face(P2)
	WM             WMStats          `json:"wm,omitempty"`              //     W(P2)
	Source         string           `json:"source,omitempty"`          // rule | model | readonly-fallback(PM-5  resolve )
	Degraded       bool             `json:"degraded,omitempty"`        // PM-2:  type  already  
	DegradedReason string           `json:"degraded_reason,omitempty"` //   origbecause(    )
}

// Planner isrule  connect . 
type Planner interface {
	Plan(goal string, m Manifest) (Plan, error)
}

// LocalPlanner isruleformrule  : onlyuse Manifest in   ,  line  now. 
type LocalPlanner struct{}

// objtgtclose word ->  need  .   i.e."  to"(  ,   ). 
var requirementRules = []struct {
	words []string
	tool  string
	auth  bool   // true =  reversible/occurproduce   ⇒    before(U1:   onlyisbase )
	kind  string //  close requireclasstype:    |  type(U3/C3)
}{
	{[]string{"部署", "上线", "deploy"}, "deploy", true, "工具"},
	{[]string{"发布到生产", "生产发布"}, "release", true, "工具"},
	{[]string{"删除", "删掉", "移除文件", "rm "}, "delete", true, "工具"},
	{[]string{"推到", "推送", "push", "远端分支"}, "push", false, "工具"},
	{[]string{"上架", "app store", "软件商店", "应用商店"}, "appstore", false, "工具"},
	{[]string{"邮件", "email", "发信"}, "email", false, "工具"},
	{[]string{"上传", "upload"}, "upload", false, "工具"},
	{[]string{"判断一下", "评估一下", "帮我判断", "帮我评估"}, "model-judgment", false, "模型"},
}

// bypassWords is"needrequire edconfirm"  lang state(F3:   novoice  beprotect  ). 
var bypassWords = []string{"不用确认", "不用我确认", "无须确认", "别问", "不用问", "直接提交", "自行决定", "不用请示"}

// protectedCapsIn  outobjtgt  and beprotect  ( reversible/need  ). 
func protectedCapsIn(goal string) []string {
	var out []string
	if containsAny(goal, "提交", "commit") {
		out = append(out, "commit")
	}
	if containsAny(goal, "部署", "上线", "发布", "deploy") {
		out = append(out, "deploy")
	}
	if containsAny(goal, "删除", "删掉") {
		out = append(out, "delete")
	}
	return out
}

// selfServiceWords is"read-only serveservice"objtgt state(SC-1:   has  ). 
var selfServiceWords = []string{"读一下", "看一下", "看看", "列出", "数一下", "几个", "跑一下", "查一下", "告诉我"}

// readonlyProbeSteps returnback**read-only  **  (F1  decide: reject  , but by   ). 
// only allow file.read / search.text / test.run / git.status; Action and Why   voicecalled"done/ become". 
func readonlyProbeSteps() []Step {
	return []Step{
		{
			Tool: "search", Caps: []string{"text"},
			Params: map[string]string{"pattern": ".", "path": "."},
			Action: "inspect related files and context (read-only probe)", Output: "matches (for judgment)",
			Why: "gather info to judge whether the goal is doable (read-only probe)",
		},
		{
			Tool: "git", Caps: []string{"status"},
			Params: map[string]string{"args": "status"},
			Action: "inspect workspace state (read-only probe)", Output: "git status output",
			Why: "gather current repo state for later judgment (read-only probe)",
		},
	}
}

// PlanWithMemory bygive   heavyrule (WM-1:     ,      body  ). 
// goal  e.g. "objtgt| body1| body2|…": '|' ofafterisbenotein    body. 
func (LocalPlanner) PlanWithMemory(goal string, m Manifest, capacity int) (Plan, error) {
	parts := strings.Split(goal, "|")
	w := &WorkingMemory{Capacity: capacity}
	for _, e := range parts[1:] {
		w.Remember("working_set", BoardItem{Element: strings.TrimSpace(e), Source: "caller"})
	}
	return LocalPlanner{}.planWithWorkingMemory(parts[0], m, w)
}

// PlanWithWorkingMemory is"     " rule in (WM-5     path): 
// has   ⇒   resolvecoreference,   chain;  empty ⇒ only clarification. 
func (LocalPlanner) PlanWithWorkingMemory(goal string, m Manifest, w *WorkingMemory) (Plan, error) {
	return LocalPlanner{}.planWithWorkingMemory(goal, m, w)
}

func (LocalPlanner) planWithWorkingMemory(goal string, m Manifest, w *WorkingMemory) (Plan, error) {
	if w == nil || len(w.BoundedWorkingSet()) == 0 {
		cons, wm := observe(goal, m, nil, nil)
		return Plan{
			Goal: goal, Source: "rule", Refused: true,
			Missing:    []string{"person: goal contains anaphora or needs context, but working memory is empty; please specify the object"},
			Reason:     "working memory empty: cannot resolve anaphora, no guessing",
			Considered: cons, WM: wm,
		}, nil
	}
	//  end has** decide **:  in endi.e.reject(   ed). 
	for _, c := range w.Constraints {
		if strings.Contains(goal, c.Element) {
			cons, wm := observe(goal, m, nil, nil)
			return Plan{Goal: goal, Source: "rule", Refused: true,
				Missing: []string{"person: this action hits constraint \"" + c.Element + "\" (source=" + c.Source + "); requires human ruling"},
				Reason:  "constraint board veto", Considered: cons, WM: wm}, nil
		}
	}
	//  decide  empty ⇒ firstclarification(  ). 
	if len(w.OpenItems) > 0 {
		cons, wm := observe(goal, m, nil, nil)
		return Plan{Goal: goal, Source: "rule", Refused: true,
			Missing:    []string{"person: open item unresolved -- " + w.OpenItems[0].Element},
			Reason:     "open-items board non-empty; ask first before acting",
			Considered: cons, WM: wm}, nil
	}
	// needrequireunderlimit = objtgt ** form to**  bodynum(  all   ⇒ 1); 
	// providegive =    body .  ersplitopen,   onlyhas  ( then   use,  formempty ). 
	if w.DemandFloor <= 0 {
		w.DemandFloor = 1
		for _, it := range w.WorkingSet {
			if strings.Contains(goal, it.Element) {
				w.DemandFloor++
			}
		}
	}
	//    body   chain :   be    body ⇒ read + modify, endtail  . 
	var steps []Step
	for range w.BoundedWorkingSet() {
		steps = append(steps,
			Step{Tool: "file", Caps: []string{"read"}, Params: map[string]string{"path": "<实体>"}, Action: "read current entity content", Output: "content"},
			Step{Tool: "file", Caps: []string{"write"}, DependsOn: []int{len(steps) - 1}, Params: map[string]string{"path": "<实体>", "content": "<修改后>"}, Action: "write modification", Output: "modified file"},
		)
	}
	steps = append(steps, Step{Tool: "git", Caps: []string{"commit"}, DependsOn: []int{len(steps) - 1},
		Params: map[string]string{"args": "commit", "message": "<说明>"}, Action: "commit changes", Output: "commit hash"})
	if missing := missingTools(m, steps); len(missing) > 0 {
		cons, wm := observe(goal, m, steps, nil)
		return Plan{Goal: goal, Source: "rule", Refused: true,
			Missing: []string{"gateway: " + strings.Join(missing, ",") + " not in capability manifest"}, Considered: cons, WM: wm}, nil
	}
	cons, wm := observe(goal, m, steps, nil)
	ct := w.CapacityFor()
	wm.Capacity, wm.DemandFloor, wm.Familiarity, wm.Capped = ct.Capacity, ct.DemandFloor, ct.Familiarity, ct.Capped
	plan := Plan{Goal: goal, Source: "rule", Steps: steps, Considered: cons, WM: wm}
	plan.WM.Drop = len(w.DropTrace)
	plan.WM.DropTrace = append([]string(nil), w.DropTrace...)
	if plan.WM.Drop > 0 {
		plan.Degraded = true
		plan.DegradedReason = "working memory full, dropped " + strings.Join(w.DropTrace, ",") + " (traced, not silent)"
	}
	return plan, nil
}

// wmCapacity is  rule "   " need numonboundary(WORKMEM-001:   hasboundary,  need  ). 
const wmCapacity = 8

// observe   base rule     need (WM-2/WM-3), and out    W. 
func observe(goal string, m Manifest, steps []Step, gaps []string) ([]ConsideredItem, WMStats) {
	var items []ConsideredItem
	for _, r := range requirementRules {
		if containsAny(goal, r.words...) {
			for _, w := range r.words {
				if strings.Contains(goal, strings.ToLower(w)) {
					items = append(items, ConsideredItem{Element: "goal:" + w, Source: "plan/planner.go:requirementRules", Inferred: false})
					break
				}
			}
		}
	}
	for _, c := range m.Tools {
		if len(steps) > 0 {
			for _, s := range steps {
				if s.Tool == c.Name {
					items = append(items, ConsideredItem{Element: "capability:" + c.Name, Source: "tools/registry.go", Inferred: false})
					break
				}
			}
		}
	}
	for _, g := range gaps {
		items = append(items, ConsideredItem{Element: "gap:" + g, Source: "plan/planner.go:gapRequirements", Inferred: true})
	}
	wm := WMStats{Max: wmCapacity, ChainLen: len(steps)}
	if len(items) > wmCapacity {
		for _, it := range items[wmCapacity:] {
			wm.DropTrace = append(wm.DropTrace, it.Element) // WM-2:     forbidstop
		}
		items = items[:wmCapacity]
	}
	wm.Used = len(items)
	wm.Drop = len(wm.DropTrace)
	return items, wm
}

// Plan byruleproduceout    ity  . 
//
// classify( class): reachable / self_service / partial / capability_gap / authorization_gap /   diff. 
//   diff ⇒ Refused + **read-only  **(Source=readonly-probe, and  voicecalledoverwriteobjtgt). 
func (LocalPlanner) Plan(goal string, m Manifest) (Plan, error) {
	g := strings.ToLower(strings.TrimSpace(goal))
	if g == "" {
		return Plan{Goal: goal, Source: "rule", Refused: true, Missing: []string{"no external dependency: goal is empty"},
			Reason: "goal is empty, no assumptions made"}, nil
	}
	// authorization_gap(F3): needrequire edconfirm + beprotect   ⇒ rejectandrefer . 
	if containsAny(g, bypassWords...) {
		if caps := protectedCapsIn(g); len(caps) > 0 {
			return Plan{Goal: goal, Source: "rule", Refused: true,
				Missing: []string{"person: protected action (" + strings.Join(caps, ",") + ") irreversible/needs authorization; must be human-confirmed; planner must not self-authorize"},
				Reason:  "goal asks to bypass confirmation; refusing silent execution"}, nil
		}
	}

	gaps := gapRequirements(g, m)
	steps, kind := classify(g)

	switch {
	case len(gaps) > 0 && kind == "":
		// capability_gap: already diff      ⇒ reject + refer owner, **    **
		//(SC-2/PL-2 needrequire: rejecttime  give  ; read-only  onlyuseat"  diff"objtgt, seeunder). 
		cons, wm := observe(goal, m, nil, gaps)
		return Plan{Goal: goal, Source: "rule", Refused: true, Missing: gaps, Considered: cons, WM: wm,
			Reason: "goal needs out-of-manifest capability (manifest is the boundary); will not fabricate an executable plan"}, nil
	case len(gaps) > 0 && kind != "":
		// partial: onlyrule   before , tail   Missing(   body  , also  bodyreject)
		if missing := missingTools(m, steps); len(missing) > 0 {
			return Plan{Goal: goal, Source: "rule", Refused: true,
				Missing: append(gaps, "gateway: "+strings.Join(missing, ",")+" not in capability manifest"), Reason: "even the reachable prefix needs out-of-manifest capability"}, nil
		}
		cons, wm := observe(goal, m, steps, gaps)
		return Plan{Goal: goal, Source: "rule", Steps: steps, Missing: gaps, Considered: cons, WM: wm,
			Degraded: true, DegradedReason: "goal partially unreachable: planning only the reachable prefix, not the full goal"}, nil
	case kind != "":
		if missing := missingTools(m, steps); len(missing) > 0 {
			return Plan{Goal: goal, Source: "rule", Refused: true,
				Missing: []string{"gateway: " + strings.Join(missing, ",") + " not in capability manifest"}, Reason: "plan needs out-of-manifest capability"}, nil
		}
		cons, wm := observe(goal, m, steps, nil)
		return Plan{Goal: goal, Source: "rule", Steps: steps, Considered: cons, WM: wm}, nil
	default:
		//   diffobjtgt: Refused(reject  )+ read-only  (   )+  form  . 
		probe := readonlyProbeSteps()
		if missing := missingTools(m, probe); len(missing) > 0 {
			probe = nil
		}
		cons, wm := observe(goal, m, probe, nil)
		return Plan{
			Goal: goal, Refused: true, Steps: probe, Source: "readonly-probe", Considered: cons, WM: wm,
			Missing:  []string{"gateway: cannot determine required capability (goal shape unsupported); will not fabricate a plan; request a feature if truly needed"},
			Reason:   "cannot determine required capability; will not fabricate a plan",
			Degraded: true, DegradedReason: "goal unrecognized: read-only probe only, does not claim to cover the goal",
		}, nil
	}
}

// classify byobjtgt statereturnback    andclassdiff("" =   diff). 
func classify(g string) ([]Step, string) {
	switch {
	case containsAny(g, "todo", "整理", "汇总", "文档"):
		return summarizeSteps(), "reachable"
	case containsAny(g, "改", "修改", "编辑", "替换") && containsAny(g, "提交", "commit"):
		return editThenCommitSteps(), "partial_or_reachable"
	case containsAny(g, selfServiceWords...):
		return selfServiceSteps(g), "self_service"
	default:
		return nil, ""
	}
}

// selfServiceSteps giveoutread-only serveservice  (SC-1). 
func selfServiceSteps(g string) []Step {
	if containsAny(g, "跑", "测试", "go test") {
		return []Step{{
			Tool: "test", Caps: []string{"run"}, Params: map[string]string{"command": "go test ./..."},
			Action: "run tests and inspect results (read-only)", Output: "test output",
			Why: "self-service: read current project test state",
		}}
	}
	return []Step{{
		Tool: "search", Caps: []string{"text"}, Params: map[string]string{"pattern": ".", "path": "."},
		Action: "inspect related file content (read-only)", Output: "matches",
		Why: "self-service: read project info",
	}}
}

// gapRequirements returnbackobjtgt    obj(C1–C4): 
//   -   class ` : `;   class ` close: `(andtgt  close requireclasstype:   / type); 
//   - same objtgt sametimehas class  , **   before**(C2); 
//   -  reversible/occurproduce       hashumanconfirmreferto(C4). 
func gapRequirements(goal string, m Manifest) []string {
	have := map[string]bool{}
	for _, c := range m.Tools {
		have[c.Name] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, r := range requirementRules {
		if !containsAny(goal, r.words...) || seen[r.tool] {
			continue
		}
		seen[r.tool] = true
		if r.auth {
			out = append(out, "person: "+r.tool+" is an irreversible/production-impacting action; requires human authorization (authorization is a prerequisite)")
		}
		if !have[r.tool] {
			out = append(out, "gateway: and there is currently no "+r.tool+" capability (request type="+r.kind+"; domain alias is not a tool contract)")
		}
	}
	return out
}

// unmetRequirement   objtgtis needrequirelist       (keep give typeform   use). 
func unmetRequirement(goal string, m Manifest) ([]string, bool) {
	gaps := gapRequirements(goal, m)
	return gaps, len(gaps) == 0
}

func missingTools(m Manifest, steps []Step) []string {
	have := map[string]bool{}
	for _, c := range m.Tools {
		have[c.Name] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range steps {
		if !have[s.Tool] && !seen[s.Tool] {
			seen[s.Tool] = true
			out = append(out, s.Tool)
		}
	}
	return out
}

func summarizeSteps() []Step {
	return []Step{
		{
			Tool: "search", Caps: []string{"text"},
			Params: map[string]string{"pattern": "TODO", "path": "."},
			Action: "search TODO markers in project", Output: "TODO list (file:line:content)",
		},
		{
			Tool: "file", Caps: []string{"write"}, DependsOn: []int{0},
			Params: map[string]string{"path": "docs/TODO-summary.md", "content": "# TODO summary (generated from search results)"},
			Action: "write TODO list into summary doc", Output: "docs/TODO-summary.md",
		},
	}
}

func editThenCommitSteps() []Step {
	return []Step{
		{
			Tool: "file", Caps: []string{"read"},
			Params: map[string]string{"path": "<待改文件>"},
			Action: "read original content of target file", Output: "target file content",
		},
		{
			Tool: "file", Caps: []string{"write"}, DependsOn: []int{0},
			Params: map[string]string{"path": "<待改文件>", "content": "<修改后内容>"},
			Action: "write modified content", Output: "modified file",
		},
		{
			Tool: "git", Caps: []string{"commit"}, DependsOn: []int{1},
			Params: map[string]string{"args": "commit", "message": "<提交说明>"},
			Action: "commit changes", Output: "commit hash",
		},
	}
}

func searchSteps() []Step {
	return []Step{{
		Tool: "search", Caps: []string{"text"},
		Params: map[string]string{"pattern": "TODO", "path": "."},
		Action: "search matching files", Output: "matching file list",
	}}
}

func containsAny(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, strings.ToLower(w)) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
//  rule(RV-1..RV-4)
// ---------------------------------------------------------------------------

// StepFailure is      /bereject describe(RV-1/RV-3   in). 
type StepFailure struct {
	StepIndex    int      `json:"step_index"`
	Tool         string   `json:"tool"`
	Caps         []string `json:"caps,omitempty"`
	Reason       string   `json:"reason"`
	DomainDenied bool     `json:"domain_denied,omitempty"` // RV-3: bedomain forbidreject
	DeniedDomain string   `json:"denied_domain,omitempty"`
}

// MaxReplans is rule numonlimit(RV-4:  nolimit rule). 
const MaxReplans = 3

// Replan      afterheavynewrule (ruleform,   ity). 
//
//   - attempt   MaxReplans -> ErrUnachievable(e.g.    to, RV-4)
//   - domain forbidreject ->     ,     ,       ed(RV-3)
//   - its    ->      and inread-only disconnect , Changes   changeize(RV-1/RV-2)
func Replan(orig Plan, f StepFailure, m Manifest, attempt int) (Plan, error) {
	if attempt > MaxReplans {
		return Plan{}, ErrUnachievable
	}
	out := orig
	if f.DomainDenied {
		out.Voided = true
		out.Changes = []string{fmt.Sprintf(
			"step %d (%s/%v) rejected by domain %q: plan voided, will not rephrase to bypass the guard",
			f.StepIndex, f.Tool, f.Caps, f.DeniedDomain)}
		out.Steps = dropStep(orig.Steps, f.StepIndex)
		out.Steps = ensureNonEmpty(out.Steps, m)
		return out, nil
	}
	out.Changes = []string{fmt.Sprintf(
		"step %d (%s) failed: %s -> remove failed step, insert read-only diagnostic step then decide",
		f.StepIndex, f.Tool, f.Reason)}
	out.Steps = dropStep(orig.Steps, f.StepIndex)
	if diag, ok := diagnosticStep(m); ok {
		out.Steps = insertAt(out.Steps, f.StepIndex, diag)
	}
	out.Steps = ensureNonEmpty(out.Steps, m)
	return out, nil
}

func dropStep(steps []Step, idx int) []Step {
	if idx < 0 || idx >= len(steps) {
		return append([]Step(nil), steps...)
	}
	out := make([]Step, 0, len(steps)-1)
	out = append(out, steps[:idx]...)
	out = append(out, steps[idx+1:]...)
	return out
}

func insertAt(steps []Step, idx int, s Step) []Step {
	if idx < 0 {
		idx = 0
	}
	if idx > len(steps) {
		idx = len(steps)
	}
	out := make([]Step, 0, len(steps)+1)
	out = append(out, steps[:idx]...)
	out = append(out, s)
	out = append(out, steps[idx:]...)
	return out
}

// diagnosticStep    **read-only** disconnect ( first git.status, its  search.text). 
func diagnosticStep(m Manifest) (Step, bool) {
	caps := map[string]map[string]bool{}
	for _, c := range m.Tools {
		cs := map[string]bool{}
		for _, cap := range c.Caps {
			cs[cap] = true
		}
		caps[c.Name] = cs
	}
	if caps["git"]["status"] {
		return Step{Tool: "git", Caps: []string{"status"}, Params: map[string]string{"args": "status"},
			Action: "read workspace state to locate failure cause", Output: "git status output"}, true
	}
	if caps["search"]["text"] {
		return Step{Tool: "search", Caps: []string{"text"}, Params: map[string]string{"pattern": ".", "path": "."},
			Action: "search related context to locate failure cause", Output: "matches"}, true
	}
	return Step{}, false
}

// ensureNonEmpty keep  ruleclose  isempty  (empty  no   , alsono   ). 
func ensureNonEmpty(steps []Step, m Manifest) []Step {
	if len(steps) > 0 {
		return steps
	}
	if s, ok := diagnosticStep(m); ok {
		return []Step{s}
	}
	return []Step{{
		Tool: "search", Caps: []string{"text"}, Params: map[string]string{"pattern": ".", "path": "."},
		Action: "read-only diagnosis (no better capability in manifest)", Output: "diagnostic output",
	}}
}
