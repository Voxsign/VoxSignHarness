// server.go -- basely HTTP serveservice   (needrequire §6 tooutconnect  /  recv #1). 
//
//   : **    **,      service disconnect.  calluse Pipeline / Dictionary, 
// pipeclose  listizebecome JSON;    task,  trigger   numdataobj ofout   file( line #1). 
//
// base (  1 approve)onlydeliver: /v1/health, /v1/correct, /v1/dictionary, /v1/process. 
// /v1/process  intentclassify   2 approve, curbeforereturnback**low-confidence ASK + need_disambiguate**, 
//  isneedrequire 4.6     bot as(    at value -> clarification/  ASK),  is  already nowintentresolve . 
package asr

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"voicesign-harness/modelcenter"
	"voicesign-harness/plan"
)

// Server keephasmanageline,  provide HTTP handle. 
type Server struct {
	Pipe *Pipeline
	// IntentModel is   ** bot** type(onlyhasbaselylow-confidencetimeonlyuse; nil =  basely). 
	IntentModel   IntentModel
	IntentTimeout time.Duration
	// DataDir isbase numdataobj (  etc; emptythen asno  ). 
	DataDir string
	// PlanModel is L2 rule use type(nil ⇒ L2  occur ,  asand ruleform  ). 
	// byout notein(occurproduce: modelcenter   typeclientuserend;   :  ). **endpoint    use **. 
	PlanModel plan.PlanModel
	// L2ModelID isoccur   L2  type id(empty ⇒ from config/plan.json + env resolve ). 
	L2ModelID string
	// L2ConfigPath is config/plan.json path(empty ⇒ usedefault topath). 
	L2ConfigPath string
	// Models is typein   (  ).  emptytime**rule   `plan`   **(  ->  -> type), 
	// and L2   ** andas  path**( again  andstore). resolve    ⇒ fail-closed   . 
	Models *modelcenter.Config
	// Teach is"useuser   word" afterend  (CACHE-001 G2). 
	// pathuserule  alreadyhas  `/v1/observe`(VHS-ASR-001 P3 endpointlist), ** new path**. 
	// asempty ⇒  endpointreturnback 503(    keep). 
	Teach func(term, canonical string) error
	// ClearTaught  empty"useuser  word"(**    serveservicediffname**). 
	ClearTaught func() int
	// Blacklist is"  modify " afterend  (§5.1   5  ): pipeword in**modifywrite name **and  . 
	// asempty ⇒ /v1/blacklist returnback 503(    keep). 
	Blacklist func(term, note string) error
}

// NewServer   serveservice. 
func NewServer(p *Pipeline) *Server { return &Server{Pipe: p} }

// Handler returnbackroutebytable. 
// loopbackOnly reject back   (NF5: ASR serveserviceno  , only allowbase calluse). 
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(strings.Trim(host, "[]"))
		if ip == nil || !ip.IsLoopback() {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"loopback-only calls allowed"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/correct", s.handleCorrect)
	mux.HandleFunc("/v1/dictionary", s.handleDictionary)
	mux.HandleFunc("/v1/process", s.handleProcess)
	mux.HandleFunc("/v1/feedback", s.handleFeedback)
	mux.HandleFunc("/v1/blacklist", s.handleBlacklist)
	mux.HandleFunc("/v1/observe", s.handleObserve)
	mux.HandleFunc("/v1/lexicon", s.handleLexicon)
	mux.HandleFunc("/v1/task", s.handleTask)
	mux.HandleFunc("/v1/testpage", s.handleTestRun)
	mux.HandleFunc("/v1/testlog", s.handleTestLog)
	mux.HandleFunc("/", s.handleTestPage)
	return loopbackOnly(mux)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "contract_version": "1"})
}

type correctRequest struct {
	Text      string   `json:"text"`
	SessionID string   `json:"session_id"`
	Context   []string `json:"context"`
}

type correctResponse struct {
	ContractVersion        string       `json:"contract_version"`
	Raw                    string       `json:"raw"`
	Text                   string       `json:"text"`
	Punctuated             string       `json:"punctuated"`
	Corrections            []Correction `json:"corrections"`
	PunctuationCorrections []Correction `json:"punctuation_corrections"`
	Candidates             []Candidate  `json:"candidates"`
	Steps                  []Step       `json:"steps"`
}

func (s *Server) handleCorrect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var req correctRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	res := s.Pipe.Process(req.Text, req.SessionID)
	writeJSON(w, http.StatusOK, correctResponse{
		ContractVersion:        "1",
		Raw:                    res.Raw,
		Text:                   res.Text,
		Punctuated:             res.Punctuated,
		Corrections:            res.Corrections,
		PunctuationCorrections: res.PunctuationCorrections,
		Candidates:             res.Candidates,
		Steps:                  res.Steps,
	})
}

// intentResponse is**   v1**     status(needrequire §6). 
// charseg   and contracts/intent-v1.schema.json   (onlyadd modify). 
type intentResponse struct {
	ContractVersion  string         `json:"contract_version"`
	Type             string         `json:"type"`
	Path             string         `json:"path"`
	Params           map[string]any `json:"params"`
	Confidence       float64        `json:"confidence"`
	NeedDisambiguate bool           `json:"need_disambiguate"`
	DomainSuggestion []string       `json:"domain_suggestion"`
	Control          string         `json:"control"`
	Confirmable      *confirmedRef  `json:"confirmable,omitempty"`
	// ContextSources is**notein  attribution**(SCOPE-PROFILE-01:     from  ). 
	//   : handwritten | zhiji | learned | project-map | none -- **none isposrulebecome **(   ). 
	ContextSources []string `json:"context_sources"`
	// ContextSourcesDetails  **origbecause**(e.g. no_profile),      value. 
	ContextSourcesDetails string `json:"context_sources_details,omitempty"`
	// ConfirmPatternMiss: confirm  ** empty  **(   ontime   see,  then inratenofrom  ). 
	ConfirmPatternMiss bool   `json:"confirm_pattern_miss,omitempty"`
	Degraded           bool   `json:"degraded,omitempty"`
	DegradedReason     string `json:"degraded_reason,omitempty"`
	Traces             []Step `json:"traces"`
}

// SourceNone is context_sources  **posrule  become **:    has  . 
const SourceNone = "none"

type processRequest struct {
	Text      string `json:"text"`
	SessionID string `json:"session_id"`
	AudioMeta any    `json:"audio_meta"`
	// Context: **calluse    onunder **(   C:     bycalluse  back, serveservice   change). 
	Context []string `json:"context,omitempty"`
	// Confirmed: **calluse  back confirmclose **(serveservicedata  use, but     ). 
	Confirmed *confirmedRef `json:"confirmed,omitempty"`
}

// confirmedRef is    confirmclose (     giveout,    bycalluse  back). 
type confirmedRef struct {
	Mention   string `json:"mention"`
	Canonical string `json:"canonical"`
}

// pickPathFromContext fromcalluse give onunder  get   use path(  : only  form state). 
func pickPathFromContext(ctx []string) string {
	for _, c := range ctx {
		c = strings.TrimSpace(c)
		for _, prefix := range []string{"打开 ", "打开", "open "} {
			if strings.HasPrefix(c, prefix) {
				return strings.TrimSpace(strings.TrimPrefix(c, prefix))
			}
		}
		if strings.ContainsAny(c, "/.") && !strings.Contains(c, " ") {
			return c
		}
	}
	return ""
}

// profileSources returnback  notein  (attribution,  keep in ). 
//
// needrequire(SCOPE-PROFILE-01): **    nil**; no  filetimegive** formvalue**( is charseg); 
// and**   safety     **(needrequire 4.5). 
// ⚠️ L3   zhiji / learned / project-map  class**   now**(etc value   decide). 
//
// `none` is**posrule  become **( is "none:no_profile"  kind  char  )--
//      ofout, etcat"  tableshow has   value raise      ", by            . 
// **" has"and"has"  splitlist**; origbecause(no_profile)  details,      value. 
func profileSources(dataDir string) (sources []string, details string) {
	if dataDir == "" {
		return []string{SourceNone}, "no_data_dir"
	}
	p := filepath.Join(dataDir, "profile", "handwritten.json")
	if b, err := os.ReadFile(p); err == nil && len(bytes.TrimSpace(b)) > 0 {
		return []string{"handwritten"}, ""
	}
	return []string{SourceNone}, "no_profile"
}

// detectConfirmation  diff"confirm, thenis X" stateandgiveout   close (serveservice keepstore ). 
func detectConfirmation(text string) *confirmedRef {
	if !strings.Contains(text, "确认") {
		return nil
	}
	for _, sep := range []string{"就是", "指的是", "是"} {
		if i := strings.Index(text, sep); i >= 0 {
			canon := strings.TrimSpace(strings.Trim(text[i+len(sep):], "。，,. ！!"))
			if canon != "" {
				return &confirmedRef{Mention: "那个模块", Canonical: canon}
			}
		}
	}
	return nil
}

func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var req processRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	res := s.Pipe.Process(req.Text, req.SessionID)
	// baselyruleresolve intent( line #6:   pathbasely); low-confidencetimeonlyby type bot,   i.e.  . 
	var cs []string
	var csDetails string
	cs, csDetails = profileSources(s.DataDir)
	var confirmable *confirmedRef
	missConfirm := false
	ir := ClassifyIntentWith(r.Context(), res.Text, s.IntentModel, s.IntentTimeout)
	//    C: **serveservice keep ize  state**;  resolve need onunder /confirmsafety bycalluse   . 
	var path string
	if req.Confirmed != nil && req.Confirmed.Canonical != "" {
		path = req.Confirmed.Canonical
		ir.NeedDisambiguate = false
	} else if p := pickPathFromContext(req.Context); p != "" {
		path = p
		ir.NeedDisambiguate = false
	}
	if ref := detectConfirmation(res.Text); ref != nil {
		confirmable = ref
	} else if strings.Contains(res.Text, "确认") {
		// useuser confirm, but     on ⇒ ** empty  **(   ). 
		missConfirm = true
	}
	writeJSON(w, http.StatusOK, intentResponse{
		ContractVersion:       "1",
		Type:                  ir.Type,
		Path:                  path,
		Params:                map[string]any{},
		Confidence:            ir.Confidence,
		NeedDisambiguate:      ir.NeedDisambiguate,
		DomainSuggestion:      ir.DomainSuggestion,
		Control:               ir.Control,
		Degraded:              ir.Degraded,
		DegradedReason:        ir.DegradedReason,
		Confirmable:           confirmable,
		ContextSources:        cs,
		ContextSourcesDetails: csDetails,
		ConfirmPatternMiss:    missConfirm,
		Traces:                res.Steps,
	})
}

// handleObserve is"useuser   word" in (G2: owner orig "     word"). 
//     tgt user_taught(   ); emptyword/samevalue  reject( thencachebe  become"  all  in"). 
func (s *Server) handleObserve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	if s.Teach == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "teaching disabled"})
		return
	}
	var req struct {
		Term      string `json:"term"`
		Canonical string `json:"canonical"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(req.Term) == "" || strings.TrimSpace(req.Canonical) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "term/canonical must not be empty"})
		return
	}
	if strings.TrimSpace(req.Term) == strings.TrimSpace(req.Canonical) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "term and canonical are identical, nothing to teach"})
		return
	}
	if err := s.Teach(req.Term, req.Canonical); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "source": "user_taught"})
}

// handleLexicon iswordtablemanage in (pathget rule  VHS-ASR-001 P3  endpointlist). 
// curbefore keep {"op":"clear_taught"}: **only useuser  word**,   serveservicediffname. 
func (s *Server) handleLexicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var req struct {
		Op string `json:"op"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	switch req.Op {
	case "clear_taught":
		if s.ClearTaught == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "lexicon disabled"})
			return
		}
		n := s.ClearTaught()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": n, "scope": "user_taught"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown op: " + req.Op})
	}
}

// handleFeedback back   rev : **  as  word  obj**(source/created_at    )
// **andand**     FeedbackRecord to feedback.jsonl(A8/A9: ✔/✘     , ✘  origbecause). 
// note : by ASR-MODEL-02 L2, useuser formrev   user_explicit,  ed learn   ; 
//  form disconnectonly  ed learn(base   now). 
func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var fb struct {
		TextRaw   string `json:"text_raw"`
		TextFinal string `json:"text_final"`
		Accepted  bool   `json:"accepted"`
		Reason    string `json:"reason"`
		Source    string `json:"source"`
	}
	if err := readJSON(r, &fb); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if s.Pipe.Dict == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dictionary disabled"})
		return
	}
	if fb.Accepted && fb.TextRaw != "" && fb.TextFinal != "" && fb.TextRaw != fb.TextFinal {
		src := fb.Source
		if src == "" {
			src = "user_edit"
		}
		if err := s.Pipe.Dict.Add(DictionaryEntry{RawSpeech: fb.TextRaw, Target: fb.TextFinal, Source: src}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}
	// ① back day : **   **(✔/✘ all      userev ); ✘   origbecause(emptythendefault). 
	reason := strings.TrimSpace(fb.Reason)
	if !fb.Accepted && reason == "" {
		reason = DefaultRejectReason
	}
	src := fb.Source
	if src == "" {
		src = "testpage"
	}
	rec := FeedbackRecord{
		At: time.Now().UTC().Format(time.RFC3339Nano), Raw: fb.TextRaw, Corrected: fb.TextFinal,
		Accepted: fb.Accepted, Reason: reason, Source: src,
	}
	logErr := ""
	if err := s.appendFeedback(rec); err != nil {
		logErr = err.Error() // back     **  **,    ,   disconnect
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"feedback": map[string]any{
			"accepted": rec.Accepted, "reason": rec.Reason, "path": s.feedbackPath(),
			"lines": s.feedbackLines(), "log_error": logErr,
		},
	})
}

// handleBlacklist is"  modify "in : pipeword inmodifywrite name and  (A10). 
// verify:  empty term; hook    ⇒ 503(    keep). 
func (s *Server) handleBlacklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	if s.Blacklist == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "blacklist disabled"})
		return
	}
	var req struct {
		Op   string `json:"op"`
		Term string `json:"term"`
		Note string `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	switch req.Op {
	case "add":
		if strings.TrimSpace(req.Term) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "term must not be empty"})
			return
		}
		if err := s.Blacklist(req.Term, req.Note); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "term": strings.TrimSpace(req.Term), "note": strings.TrimSpace(req.Note)})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown op: " + req.Op})
	}
}

type dictionaryRequest struct {
	Text      string `json:"text"` // langaudiorefer orig 
	Op        string `json:"op"`   // add | update | delete | list
	RawSpeech string `json:"raw_speech"`
	Target    string `json:"target"`
	Scope     string `json:"scope"`
	Priority  int    `json:"priority"`
	Source    string `json:"source"`
	Term      string `json:"term"`
	Confirm   bool   `json:"confirm"`
}

func (s *Server) handleDictionary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	if s.Pipe.Dict == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dictionary disabled"})
		return
	}
	var req dictionaryRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	// langaudiorefer  first: first Add, again Delete(allis riskbyout pathhas  protect ). 
	if req.Text != "" {
		if e, ok := ParseVoiceAdd(req.Text); ok {
			if err := s.Pipe.Dict.Add(e); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entry": e, "version": s.Pipe.Dict.Version()})
			return
		}
		if term, ok := ParseVoiceDelete(req.Text); ok {
			if !req.Confirm {
				writeJSON(w, http.StatusOK, map[string]any{"need_confirm": true, "term": term})
				return
			}
			_, _ = s.Pipe.Dict.Delete(term, true)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": true, "term": term, "version": s.Pipe.Dict.Version()})
			return
		}
	}

	switch req.Op {
	case "add", "update":
		e := DictionaryEntry{RawSpeech: req.RawSpeech, Target: req.Target, Scope: req.Scope, Priority: req.Priority, Source: req.Source}
		if e.Source == "" {
			e.Source = "manual"
		}
		if err := s.Pipe.Dict.Add(e); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entry": e, "version": s.Pipe.Dict.Version()})
	case "delete":
		//  line #3: noconfirm   , and**   **(file  origkind). 
		if !req.Confirm {
			writeJSON(w, http.StatusOK, map[string]any{"need_confirm": true, "term": req.Term})
			return
		}
		deleted, err := s.Pipe.Dict.Delete(req.Term, true)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted, "term": req.Term, "version": s.Pipe.Dict.Version()})
	case "list":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entries": s.Pipe.Dict.List(), "version": s.Pipe.Dict.Version()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown op"})
	}
}
