// task.go -- `POST /v1/task`: **onlyrule ,    **(Lead  decide 2026-10-03). 
//
//  boundary( allow ): 
//
//	· onlyrule ,     --      execute:false,  face  tgtnote; 
//	· document only rule  in, **   to  **(base num write  file); 
//	· returnback:    /    /  data / **rejectorigbecause** / **   ** / Source / Degraded / trace; 
//	· **domain forbid rule period disconnect**(    / domaincur reject, but isetcto  ). 
package asr

import (
	"net/http"
	"strconv"
	"strings"

	"voicesign-harness/modelcenter"
	"voicesign-harness/plan"
	"voicesign-harness/space"
	"voicesign-harness/tools"
)

// taskPlanStep isgive face    (read-onlyfast ). 
type taskPlanStep struct {
	Index     int      `json:"index"`
	Tool      string   `json:"tool"`
	Caps      []string `json:"caps"`
	Action    string   `json:"action"`
	Output    string   `json:"output"`
	Why       string   `json:"why,omitempty"`
	Domain    string   `json:"domain,omitempty"`
	DependsOn []int    `json:"depends_on,omitempty"`
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var req struct {
		Task     string `json:"task"`
		Document string `json:"document"` // **only rule  in**,    
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Task == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "task 不得为空"})
		return
	}
	//   listby code valueasapprove(contracts obj get  DataDir;   thenusein default). 
	tr, err := tools.LoadContracts(s.DataDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "工具注册表: " + err.Error()})
		return
	}
	sr, err := space.Load(s.DataDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "域注册表: " + err.Error()})
		return
	}
	m := plan.ExportManifest(tr, sr)
	// ② **     beread**: close  + close word  considered / WM(** pipesafety    showword**). 
	goal := req.Task // note : goal ** again**write"    N char"( is" raise read")
	// ① **endpoint      PlanWithL2**(Lead   :  before  code LocalPlanner ⇒ L2 is  code). 
	cfgPath := s.L2ConfigPath
	if cfgPath == "" {
		cfgPath = "config/plan.json"
	}
	modelID := s.L2ModelID
	if s.Models != nil {
		// ** and**:   (useway)->   (  )->  type id; resolve    fail-closed. 
		id, err := s.Models.ResolveModel(modelcenter.ChannelPlan)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": "plan 通道解析失败（fail-closed）: " + err.Error(),
			})
			return
		}
		modelID = id
	} else if modelID == "" {
		modelID = plan.L2ModelID(cfgPath)
	}
	p, err := plan.PlanWithL2(r.Context(), goal, m, s.PlanModel, modelID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	doc := summarizeDocument(req.Document)
	if doc.TooLarge {
		p.Degraded = true
		p.DegradedReason = "文档过大（>" + strconv.Itoa(maxDocRunes) + " 字），已截断摘要，未全文参与规划"
	}
	if doc.Read {
		// ① close   considered
		p.Considered = append([]plan.ConsideredItem{{
			Element: doc.Describe(), Source: "<document>", Inferred: false,
		}}, p.Considered...)
		// ③ taskand   close word in see
		for _, kw := range doc.HitKeywords(req.Task) {
			p.Considered = append(p.Considered, plan.ConsideredItem{
				Element: "document_hit:" + kw, Source: "<document>", Inferred: true,
			})
		}
		// ②    body    body  path: w_used because  butchange(   then  )
		added := 0
		for _, e := range doc.Entities() {
			if p.WM.Used+added >= p.WM.Max {
				p.WM.Drop++
				p.WM.DropTrace = append(p.WM.DropTrace, "document:"+e)
				continue
			}
			added++
			p.Considered = append(p.Considered, plan.ConsideredItem{
				Element: "document_entity:" + e, Source: "<document>", Inferred: true,
			})
		}
		p.WM.Used += added

		// #2-c ① **      **:  objnum ⇒ approvehandle  (ruleform      ). 
		// ② revexamplepreventline: approvenum**onlyby objnum**decide  ⇒ in etc but   same    to same steps. 
		driver := doc.Items
		driverName := "条目"
		if doc.TODOs > driver {
			driver, driverName = doc.TODOs, "TODO"
		}
		if driver == doc.Items && doc.TODOs == doc.Items {
			driverName = "条目/TODO"
		}
		batches := (driver + batchSizeItems - 1) / batchSizeItems
		_ = driverName
		if batches > 1 {
			p.Steps = expandBatchedSteps(p.Steps, batches)
			p.WM.ChainLen = len(p.Steps) // ③  openafter  and steps   ( then  numcharis  )
			p.Considered = append(p.Considered, plan.ConsideredItem{
				Element: "document_batches: " + strconv.Itoa(batches) + "（" + driverName +
					" " + strconv.Itoa(driver) + " 条 / 每批 " + strconv.Itoa(batchSizeItems) + "）",
				Source: "<document>", Inferred: true,
			})
		}
	}
	steps := make([]taskPlanStep, 0, len(p.Steps))
	for i, st := range p.Steps {
		steps = append(steps, taskPlanStep{
			Index: i, Tool: st.Tool, Caps: st.Caps, Action: st.Action, Output: st.Output,
			Why: st.Why, Domain: st.Domain, DependsOn: st.DependsOn,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"execute":       false,
		"l2_enabled":    plan.L2Enabled(modelID) && s.PlanModel != nil, //   but    ⇒ e.g.   false
		"l2_model":      modelID,
		"l2_note":       l2Note(s), // ⚠️ base onlyrule ,    
		"document_read": doc.Read,  // empty  / read ⇒ false + note( allow  cur has)
		"document_note": doc.Note,
		"document": map[string]any{
			"lines": doc.Lines, "items": doc.Items, "todos": doc.TODOs,
			"keywords": doc.Keywords, "too_large": doc.TooLarge,
		},
		"goal": p.Goal,
		"plan": map[string]any{
			"steps": steps, "refused": p.Refused, "reason": p.Reason,
			"missing": p.Missing, // rejectorigbecause + **   **(owner before )
			"source":  p.Source, "degraded": p.Degraded, "degraded_reason": p.DegradedReason,
			"considered": p.Considered, "wm": p.WM,
		},
		"note": "本轮只规划，不执行；document 仅作规划输入，不落盘。",
	})
}

func itoaLen(s string) int { return len([]rune(s)) }

// l2Note   "as     use typeform"(  but   time,     ). 
func l2Note(s *Server) string {
	if s.PlanModel == nil {
		return "L2 未装配（无模型客户端）⇒ 走规则式"
	}
	return ""
}

// batchSizeItems is  "approvehandle"  overwrite    objnum. 
// ⚠️ **UNVALIDATED**( get  10,  tgt ). 
const batchSizeItems = 10

// expandBatchedSteps pipe   ** after  writefile  **byapprovenum open(its    change). 
// ruleformrule   table    thento  : **approvenumby   objnumdecide **. 
func expandBatchedSteps(steps []plan.Step, batches int) []plan.Step {
	idx := -1
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Tool == "file" && containsCap(steps[i].Caps, "write") {
			idx = i
			break
		}
	}
	if idx < 0 || batches < 2 {
		return steps
	}
	orig := steps[idx]
	out := append([]plan.Step(nil), steps[:idx]...)
	prev := idx - 1
	for b := 1; b <= batches; b++ {
		step := orig
		if b > 1 {
			step.DependsOn = []int{prev}
		}
		step.Output = orig.Output + "（第 " + strconv.Itoa(b) + "/" + strconv.Itoa(batches) + " 批）"
		step.Action = orig.Action + "（第 " + strconv.Itoa(b) + "/" + strconv.Itoa(batches) + " 批）"
		out = append(out, step)
		prev = len(out) - 1
	}
	out = append(out, steps[idx+1:]...)
	return out
}

func containsCap(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

// maxDocRunes is   andrule  onlimit(**UNVALIDATED**;  limit disconnect needandtgt  ). 
const maxDocRunes = 200000

// docSummary is   ** need**(close  + close word),  issafety . 
//
//   : ** allowpipe       showword**(L3: onlynotein close seg); 
// also** allowonly     "in alreadysplit "**( thenis  )--underface  numallfromin  out . 
type docSummary struct {
	Read                bool
	Lines, Items, TODOs int
	Keywords, hit       []string
	Note                string
	TooLarge            bool
}

func (d docSummary) Describe() string {
	return "document: " + strconv.Itoa(d.Lines) + " 行 / " + strconv.Itoa(d.Items) +
		" 条目 / " + strconv.Itoa(d.TODOs) + " 个 TODO"
}

// Entities returnback  as"   body"    body(hasboundary, getbefore 8  close word). 
func (d docSummary) Entities() []string {
	if len(d.Keywords) > 8 {
		return d.Keywords[:8]
	}
	return d.Keywords
}

// HitKeywords returnbacktask outnowed   close word(task↔  close  see). 
func (d docSummary) HitKeywords(task string) []string {
	var out []string
	for _, kw := range d.Keywords {
		if strings.Contains(task, kw) {
			out = append(out, kw)
		}
	}
	return out
}

// summarizeDocument from  in  out**close  + close word**(hasboundary,   ity). 
func summarizeDocument(doc string) docSummary {
	var d docSummary
	if strings.TrimSpace(doc) == "" {
		d.Note = "document 为空（未参与规划）"
		return d
	}
	runes := []rune(doc)
	if len(runes) > maxDocRunes {
		d.TooLarge = true
		doc = string(runes[:maxDocRunes])
	}
	d.Read = true
	seen := map[string]bool{}
	for _, line := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		d.Lines++
		// ①  obj formoverwrite: listtabletgt  + TODO/FIXME/XXX tgt  **all  obj**
		if isListItem(t) || isTodoLine(t) {
			d.Items++
		}
		if isTodoLine(t) {
			d.TODOs++
		}
		for _, tok := range tokenize(t) {
			if !seen[tok] {
				seen[tok] = true
				d.Keywords = append(d.Keywords, tok)
			}
		}
	}
	if len(d.Keywords) > 20 {
		d.Keywords = d.Keywords[:20]
	}
	if d.TooLarge {
		d.Note = "文档超限，已按前 " + strconv.Itoa(maxDocRunes) + " 字做摘要"
	}
	return d
}

// isListItem  diff seelisttable/tgt  form(** kind formallneedhas data**). 
func isListItem(t string) bool {
	for _, p := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	// numcharpt/ id: 1. / 1) / 1, 
	digits := 0
	for digits < len(t) && t[digits] >= '0' && t[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits < len(t) {
		switch t[digits] {
		case '.', ')':
			return true
		}
		if strings.HasPrefix(t[digits:], "、") {
			return true
		}
	}
	return false
}

// isTodoLine  diff TODO/FIXME/XXX tgt  ( or   id). 
func isTodoLine(t string) bool {
	u := strings.ToUpper(t)
	for _, m := range []string{"TODO", "FIXME", "XXX"} {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

// tokenize  out    word(inday  >=2 char,    >=3 char),   tgtptandempty . 
func tokenize(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) >= 2 {
			out = append(out, string(cur))
		}
		cur = nil
	}
	for _, r := range s {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF: //  char
			cur = append(cur, r)
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			cur = append(cur, r)
		default:
			flush()
		}
	}
	flush()
	return out
}
