// vhs-voice: VHS langaudio   (needrequirerule  v1, 2026-10-04). 
//
//  "mobile clientlangaudio -> after   task" pos  : mobile clientpipelangaudio diffbecome baseafter, 
// first base   --     ->   refer   task -> domain/to patchsafety ->
//       harness(POST /v1/tasks + poll GET /v1/tasks/{id})-> langaudio    . 
//
// onlytgtapprove ,     dependency. portdefault 8950(VHS_VOICE_ADDR overwrite), 
// on   harness default http://127.0.0.1:8941(VHS_UPSTREAM overwrite). 
//     : <dataDir>/voice_sessions/<conversation_id>.jsonl(   ). 
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------- wordtable

var fillers = []string{
	"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后",
	"现在", "开始", "准备", "假设", "其实", "比如", "我觉得",
	"你知道", "大概", "应该", "可以",
}

var actionVerbs = []string{
	"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交",
	"报告", "检查", "对比", "分析", "部署", "安装", "更新",
}

var objectPatterns = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"GitHub 仓库", regexp.MustCompile(`GitHub\s*仓库\s*([\w\-./@:]+)`)},
	{"服务", regexp.MustCompile(`服务\s*([\w\-./@:]+)`)},
	{"需求", regexp.MustCompile(`需求\s*([\w\-./@:]+)`)},
	{"产物", regexp.MustCompile(`产物\s*([\w\-./@:]+)`)},
	{"测试", regexp.MustCompile(`测试\s*([\w\-./@:]+)`)},
}

//    -> to classdiff(resolve use, byclassdifffrom    patchsafetyto ). 
var actionKindMap = map[string]string{
	"拉取": "GitHub 仓库", "更新": "GitHub 仓库", "提交": "GitHub 仓库",
	"编译": "服务", "启动": "服务", "部署": "服务", "安装": "服务",
	"跑": "测试", "检查": "测试", "测试": "测试",
	"执行": "需求", "实现": "需求", "报告": "需求", "分析": "需求", "对比": "需求",
}

// ---------------------------------------------------------------- classtype

type voiceAction struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

type parseResp struct {
	Clean        string        `json:"clean"`
	Actions      []voiceAction `json:"actions"`
	NoiseRemoved []string      `json:"noise_removed"`
}

type taskItem struct {
	Seq    int    `json:"seq"`
	Action string `json:"action"`
	Target string `json:"target"`
}

type decomposeResp struct {
	Tasks  []taskItem `json:"tasks"`
	Count  int        `json:"count,omitempty"`
	Reason string     `json:"reason,omitempty"`
}

type resolvedItem struct {
	Action string `json:"action"`
	Target string `json:"target"`
	Source string `json:"source"`
}

type resolveResp struct {
	Resolved []resolvedItem `json:"resolved"`
	Context  map[string]any `json:"context"`
}

type runResp struct {
	Summary map[string]int `json:"summary"`
	Tasks   []runTask      `json:"tasks"`
	Next    []string       `json:"next"`
}

type runTask struct {
	Seq      int    `json:"seq"`
	Action   string `json:"action"`
	Target   string `json:"target"`
	TaskID   string `json:"task_id,omitempty"`
	Status   string `json:"status"`
	Result   string `json:"result,omitempty"`
	Question string `json:"question,omitempty"`
}

// sessionRecord iswrite   JSONL      (append-only). 
type sessionRecord struct {
	TS             time.Time      `json:"ts"`
	ConversationID string         `json:"conversation_id"`
	Phase          string         `json:"phase"` // parse|decompose|resolve|run|done
	Clean          string         `json:"clean,omitempty"`
	Actions        []voiceAction  `json:"actions,omitempty"`
	Tasks          []taskItem     `json:"tasks,omitempty"`
	Resolved       []resolvedItem `json:"resolved,omitempty"`
	RunTasks       []runTask      `json:"run_tasks,omitempty"`
	Raw            string         `json:"raw,omitempty"`
}

type server struct {
	upstream string
	dataDir  string
	client   *http.Client
}

// ---------------------------------------------------------------- endpoint

func main() {
	addr := envOr("VHS_VOICE_ADDR", "8950")
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	upstream := envOr("VHS_UPSTREAM", "http://127.0.0.1:8941")
	dataDir := envOr("VHS_VOICE_DATA", "./voice-data")
	if err := os.MkdirAll(filepath.Join(dataDir, "voice_sessions"), 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	s := &server{
		upstream: upstream,
		dataDir:  dataDir,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/voice/health", s.handleHealth)
	mux.HandleFunc("/v1/voice/parse", s.handleParse)
	mux.HandleFunc("/v1/voice/decompose", s.handleDecompose)
	mux.HandleFunc("/v1/voice/resolve", s.handleResolve)
	mux.HandleFunc("/v1/voice/run", s.handleRun)
	mux.HandleFunc("/v1/voice/tasks/", s.handleTasksHistory)
	log.Printf("vhs-voice listening on %s, upstream=%s, dataDir=%s", addr, upstream, dataDir)
	if err := http.ListenAndServe(addr, logMW(mux)); err != nil {
		log.Fatalf("vhs-voice 退出: %v", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func logMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, _ := sessionCount(s.dataDir)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"service":  "vhs-voice",
		"upstream": s.upstream,
		"sessions": n,
	})
}

// ---------------------------------------------------------------- parse

func (s *server) handleParse(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	resp := parseSpoken(in.Text)
	s.appendRecord(in.Text, "", "parse", resp.Clean, resp.Actions, nil, nil, nil)
	writeJSON(w, http.StatusOK, resp)
}

func parseSpoken(text string) parseResp {
	removed := []string{}
	for _, f := range fillers {
		for strings.Contains(text, f) {
			text = strings.ReplaceAll(text, f, "")
			removed = append(removed, f)
		}
	}
	//     empty / id link
	text = strings.Join(strings.Fields(text), " ")
	actions := extractActions(text)
	return parseResp{Clean: strings.TrimSpace(text), Actions: actions, NoiseRemoved: uniqKeep(removed)}
}

func extractActions(text string) []voiceAction {
	var actions []voiceAction
	//    first  , getfirstword
	firstVerb := ""
	for _, v := range actionVerbs {
		if strings.Contains(text, v) {
			firstVerb = v
			break
		}
	}
	if firstVerb == "" {
		return nil
	}
	target := extractObject(text)
	actions = append(actions, voiceAction{Action: firstVerb, Target: target})
	return actions
}

func extractObject(text string) string {
	for _, p := range objectPatterns {
		if m := p.re.FindStringSubmatch(text); len(m) > 1 {
			return p.kind + " " + strings.TrimSpace(m[1])
		}
	}
	//   formto :   wordofafter nameword lang( fix word/linkconnectword)
	if idx := indexOfAnyVerb(text); idx >= 0 {
		rest := strings.TrimSpace(text[idx+len(firstVerbAt(text)):])
		rest = trimModifiers(rest)
		rest = trimLinkVerbPrefix(rest)
		if rest != "" && !isFillerOnly(rest) {
			return rest
		}
	}
	return ""
}

func indexOfAnyVerb(text string) int {
	for _, v := range actionVerbs {
		if i := strings.Index(text, v); i >= 0 {
			return i
		}
	}
	return -1
}

func firstVerbAt(text string) string {
	idx := indexOfAnyVerb(text)
	if idx < 0 {
		return ""
	}
	for _, v := range actionVerbs {
		if strings.Index(text, v) == idx {
			return v
		}
	}
	return ""
}

// trimLinkVerbPrefix   "and/and/and +   word"before (e.g."  andstart serveservice"->"serveservice"). 
func trimLinkVerbPrefix(s string) string {
	links := []string{"并", "和", "且", "然后"}
	for {
		changed := false
		for _, l := range links {
			if strings.HasPrefix(s, l) {
				s = strings.TrimSpace(strings.TrimPrefix(s, l))
				changed = true
				break
			}
		}
		if changed {
			continue
		}
		if v := firstVerbAt(s); v != "" && strings.HasPrefix(s, v) && len(s) > len(v) {
			s = strings.TrimSpace(s[len(v):])
			continue
		}
		break
	}
	return s
}

var modifierRe = regexp.MustCompile(`^(一下|一个|一次|最新版|全部|所有|一下下|一遍|一轮|一下新)\s*`)

func trimModifiers(s string) string {
	for {
		if m := modifierRe.FindStringSubmatch(s); len(m) > 0 {
			s = strings.TrimSpace(s[len(m[0]):])
			continue
		}
		break
	}
	return s
}

func isFillerOnly(s string) bool {
	for _, f := range fillers {
		s = strings.ReplaceAll(s, f, "")
	}
	return strings.TrimSpace(s) == ""
}

// ---------------------------------------------------------------- decompose

func (s *server) handleDecompose(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Clean string `json:"clean"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	resp := decomposeText(in.Clean)
	s.appendRecord(in.Clean, "", "decompose", in.Clean, nil, resp.Tasks, nil, nil)
	writeJSON(w, http.StatusOK, resp)
}

func decomposeText(clean string) decomposeResp {
	// bysplit    lang
	phrases := splitPhrases(clean)
	tasks := []taskItem{}
	seq := 1
	for _, ph := range phrases {
		ph = strings.TrimSpace(ph)
		if ph == "" {
			continue
		}
		verb := firstVerbAt(ph)
		if verb == "" {
			continue
		}
		target := extractObject(ph)
		tasks = append(tasks, taskItem{Seq: seq, Action: verb, Target: target})
		seq++
	}
	if len(tasks) == 0 {
		return decomposeResp{Tasks: []taskItem{}, Reason: "无动作"}
	}
	return decomposeResp{Tasks: tasks, Count: len(tasks)}
}

var sepRe = regexp.MustCompile(`[，,、。;；!！?？\n]`)

func splitPhrases(s string) []string {
	parts := sepRe.Split(s, -1)
	out := []string{}
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}

// ---------------------------------------------------------------- resolve

func (s *server) handleResolve(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text           string        `json:"text"`
		ConversationID string        `json:"conversation_id"`
		Actions        []voiceAction `json:"actions"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	resp := s.resolve(in.ConversationID, in.Actions)
	s.appendRecord(in.Text, in.ConversationID, "resolve", in.Text, in.Actions, nil, resp.Resolved, nil)
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) resolve(conversationID string, actions []voiceAction) resolveResp {
	resolved := []resolvedItem{}
	for _, a := range actions {
		item := resolvedItem{Action: a.Action, Target: a.Target}
		switch {
		case a.Target != "":
			item.Source = "explicit"
		default:
			// session:    3   by  classdiff to 
			if t, ok := s.findFromSession(conversationID, a.Action); ok {
				item.Target = t
				item.Source = "session"
			} else {
				item.Target = "unresolved"
				item.Source = "default"
			}
		}
		resolved = append(resolved, item)
	}
	return resolveResp{Resolved: resolved, Context: map[string]any{"conversation_id": conversationID}}
}

func (s *server) findFromSession(conversationID, action string) (string, bool) {
	kind := actionKindMap[action]
	if kind == "" {
		return "", false
	}
	recs := s.readRecentSession(conversationID, 3)
	// from to 
	for i := len(recs) - 1; i >= 0; i-- {
		rec := recs[i]
		cands := []string{}
		for _, a := range rec.Actions {
			cands = append(cands, a.Target)
		}
		for _, t := range rec.Resolved {
			cands = append(cands, t.Target)
		}
		for _, c := range cands {
			if strings.Contains(c, kind) && c != "" {
				return c, true
			}
		}
	}
	return "", false
}

// ---------------------------------------------------------------- run

func (s *server) handleRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text           string `json:"text"`
		ConversationID string `json:"conversation_id"`
		Document       string `json:"document"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	cid := in.ConversationID
	if cid == "" {
		cid = "default"
	}
	// 1. parse
	p := parseSpoken(in.Text)
	s.appendRecord(in.Text, cid, "parse", p.Clean, p.Actions, nil, nil, nil)
	// 2. decompose
	dc := decomposeText(p.Clean)
	s.appendRecord(p.Clean, cid, "decompose", p.Clean, p.Actions, dc.Tasks, nil, nil)
	// 3. resolve
	rs := s.resolve(cid, mapActions(p.Actions, dc.Tasks))
	s.appendRecord(p.Clean, cid, "resolve", p.Clean, p.Actions, dc.Tasks, rs.Resolved, nil)

	// 4.     on 
	tasks := []runTask{}
	next := []string{}
	if len(dc.Tasks) == 0 {
		tasks = []runTask{{Seq: 1, Action: "ask", Target: "", Status: "need_ask", Result: dc.Reason}}
		next = append(next, "未识别到动作，请换个说法")
	} else {
		for i, t := range dc.Tasks {
			rt := runTask{Seq: t.Seq, Action: t.Action, Target: t.Target}
			//  patchsafetyto  -> pending_resolve,    
			if t.Target == "" || t.Target == "unresolved" {
				rt.Status = "pending_resolve"
				rt.Result = "对象未补全，待确认"
				next = append(next, fmt.Sprintf("任务%d「%s」对象未补全，请补充对象", t.Seq, t.Action))
				tasks = append(tasks, rt)
				continue
			}
			tid, status, question, result, err := s.dispatchUpstream(cid, in.Document, t)
			if err != nil {
				rt.Status = "failed"
				rt.Result = err.Error()
				next = append(next, fmt.Sprintf("任务%d投递失败：%v", t.Seq, err))
			} else {
				rt.TaskID = tid
				rt.Status = status
				rt.Result = result
				rt.Question = question
				if status == "need_ask" {
					next = append(next, fmt.Sprintf("任务%d需要确认：%s", t.Seq, question))
				} else if status == "failed" {
					next = append(next, fmt.Sprintf("任务%d失败，请重试", t.Seq))
				}
			}
			tasks = append(tasks, rt)
			_ = i
		}
	}
	s.appendRecord(p.Clean, cid, "run", p.Clean, p.Actions, dc.Tasks, rs.Resolved, tasks)

	summary := map[string]int{"total": len(tasks), "done": 0, "need_ask": 0, "failed": 0}
	for _, t := range tasks {
		switch t.Status {
		case "done":
			summary["done"]++
		case "need_ask", "pending_resolve":
			summary["need_ask"]++
		case "failed":
			summary["failed"]++
		}
	}
	writeJSON(w, http.StatusOK, runResp{Summary: summary, Tasks: tasks, Next: next})
}

// mapActions use decompose   action/target overwrite parse    list(decompose change ). 
func mapActions(parsed []voiceAction, tasks []taskItem) []voiceAction {
	if len(tasks) > 0 {
		out := []voiceAction{}
		for _, t := range tasks {
			out = append(out, voiceAction{Action: t.Action, Target: t.Target})
		}
		return out
	}
	return parsed
}

// dispatchUpstream    taskto  harness andpoll endstate. 
func (s *server) dispatchUpstream(cid, document string, t taskItem) (taskID, status, question, result string, err error) {
	payload := map[string]any{
		"text":            fmt.Sprintf("%s %s", t.Action, t.Target),
		"conversation_id": cid,
	}
	if document != "" {
		payload["document"] = document
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, s.upstream+"/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+envOr("VHS_UPSTREAM_TOKEN", ""))
	resp, err := s.client.Do(req)
	if err != nil {
		return "", "", "", "", fmt.Errorf("上游不可达: %v", err)
	}
	defer resp.Body.Close()
	var created struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", "", "", "", fmt.Errorf("上游响应解析失败: %v", err)
	}
	if created.Error != "" {
		return "", "", "", "", errors.New(created.Error)
	}
	// poll
	deadline := time.Now().Add(90 * time.Second)
	for {
		st, q, res := s.pollTask(created.TaskID)
		switch st {
		case "done", "need_ask", "need_confirm", "failed":
			status = st
			question = q
			result = summarizeResult(res)
			return created.TaskID, status, question, result, nil
		default: // running / its 
			if time.Now().After(deadline) {
				return created.TaskID, "failed", "", "轮询超时", nil
			}
			time.Sleep(2 * time.Second)
		}
	}
}

func (s *server) pollTask(taskID string) (status, question, result string) {
	req, _ := http.NewRequest(http.MethodGet, s.upstream+"/v1/tasks/"+taskID, nil)
	req.Header.Set("Authorization", "Bearer "+envOr("VHS_UPSTREAM_TOKEN", ""))
	resp, err := s.client.Do(req)
	if err != nil {
		return "failed", "", "上游轮询不可达"
	}
	defer resp.Body.Close()
	var out struct {
		Status   string         `json:"status"`
		Question string         `json:"question"`
		Options  []string       `json:"options"`
		Receipt  map[string]any `json:"receipt"`
		Error    string         `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Error != "" {
		return "failed", "", out.Error
	}
	st := out.Status
	if st == "" {
		st = "running"
	}
	q := out.Question
	if q == "" && len(out.Options) > 0 {
		q = strings.Join(out.Options, " / ")
	}
	res := ""
	if b, err := json.Marshal(out.Receipt); err == nil {
		res = string(b)
	}
	return st, q, res
}

// summarizeResult pipe receipt JSON  become   sent(langaudio   <=120 char). 
func summarizeResult(receiptJSON string) string {
	if receiptJSON == "" {
		return "已完成"
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(receiptJSON), &m); err != nil {
		//  disconnectorigkind
		return cut(receiptJSON, 120)
	}
	//   get view.result / view.Result / result charseg
	for _, k := range []string{"view.result", "view.Result", "result"} {
		if v, ok := getPath(m, k); ok {
			return cut(fmt.Sprint(v), 120)
		}
	}
	return cut(receiptJSON, 120)
}

func getPath(m map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = m
	for _, p := range parts {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = mm[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ---------------------------------------------------------------- E6 backread

func (s *server) handleTasksHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cid := strings.TrimPrefix(r.URL.Path, "/v1/voice/tasks/")
	if cid == "" {
		http.Error(w, "conversation_id 缺失", http.StatusBadRequest)
		return
	}
	recs := s.readSession(cid)
	//    has run stagetask,  heavy(by task_id/seq)
	seen := map[string]bool{}
	tasks := []taskItem{}
	for _, rec := range recs {
		for _, t := range rec.Tasks {
			key := strconv.Itoa(t.Seq) + ":" + t.Action + ":" + t.Target
			if !seen[key] {
				seen[key] = true
				tasks = append(tasks, t)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": cid,
		"tasks":           tasks,
		"count":           len(tasks),
	})
}

// ----------------------------------------------------------------     

func (s *server) appendRecord(raw, cid, phase, clean string, actions []voiceAction, tasks []taskItem, resolved []resolvedItem, runTasks []runTask) {
	rec := sessionRecord{
		TS:             time.Now(),
		ConversationID: cid,
		Phase:          phase,
		Clean:          clean,
		Actions:        actions,
		Tasks:          tasks,
		Resolved:       resolved,
		RunTasks:       runTasks,
		Raw:            raw,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	path := s.sessionPath(cid)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

func (s *server) sessionPath(cid string) string {
	safe := cid
	if cid == "" {
		safe = "default"
	}
	safe = strings.ReplaceAll(safe, "/", "_")
	safe = strings.ReplaceAll(safe, "..", "_")
	return filepath.Join(s.dataDir, "voice_sessions", safe+".jsonl")
}

func (s *server) readSession(cid string) []sessionRecord {
	f, err := os.Open(s.sessionPath(cid))
	if err != nil {
		return nil
	}
	defer f.Close()
	recs := []sessionRecord{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var rec sessionRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err == nil {
			recs = append(recs, rec)
		}
	}
	return recs
}

func (s *server) readRecentSession(cid string, n int) []sessionRecord {
	all := s.readSession(cid)
	if len(all) <= n {
		return all
	}
	return all[len(all)-n:]
}

func sessionCount(dataDir string) (int, error) {
	dir := filepath.Join(dataDir, "voice_sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			n++
		}
	}
	return n, nil
}

// ----------------------------------------------------------------   

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return false
	}
	if err := json.Unmarshal(body, v); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func uniqKeep(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
