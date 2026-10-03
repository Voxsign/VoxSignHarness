package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
)

// TermEntry 词典词条（需求契约字段：term/variants/category/source + 兼容 correction）。
// term = 标准形式；variants = 该词的识别变体（文本中命中 variants → 纠为 term）。
type TermEntry struct {
	Term       string   `json:"term"`
	Correction string   `json:"correction"`
	Variants   []string `json:"variants"`
	Category   string   `json:"category"`
	Source     string   `json:"source"`
}

// Dictionary 用户教词词典，线程安全，持久化为 dictionary.json（JSON 对象）。
type Dictionary struct {
	canon map[string]TermEntry // term → entry
	alias map[string]string    // variant/term → canon term（纠错查找）
	mu    sync.RWMutex
}

func NewDictionary() *Dictionary {
	return &Dictionary{canon: make(map[string]TermEntry), alias: make(map[string]string)}
}

// Add 增词：term 标准形式，variants 变体（含 term 本身），source 来源。
func (d *Dictionary) Add(term, correction string, variants []string, source string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if correction == "" {
		correction = term
	}
	if len(variants) == 0 {
		variants = []string{term}
	}
	d.canon[term] = TermEntry{
		Term: term, Correction: correction, Variants: variants,
		Category: "user", Source: source,
	}
	for _, v := range variants {
		d.alias[v] = term
	}
	d.alias[term] = term
}

func (d *Dictionary) Delete(term string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if e, ok := d.canon[term]; ok {
		for _, v := range e.Variants {
			delete(d.alias, v)
		}
		delete(d.alias, term)
	}
	delete(d.canon, term)
}

// Snapshot 返回全量词条（/v1/dict 用，字段含 term/variants/category/source）。
func (d *Dictionary) Snapshot() []TermEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]TermEntry, 0, len(d.canon))
	for _, e := range d.canon {
		out = append(out, e)
	}
	return out
}

// Correct 应用词典纠错（黑名单词跳过）。
func (d *Dictionary) Correct(text string) string {
	corrected, _ := d.CorrectWithApplied(text)
	return corrected
}

// CorrectWithApplied 返回纠错结果 + 实际应用的映射列表（/v1/correct 用）。
func (d *Dictionary) CorrectWithApplied(text string) (string, []string) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	words := strings.Fields(text)
	var applied []string
	for i, word := range words {
		if _, blocked := blookup(word); blocked {
			continue
		}
		if canon, ok := d.alias[word]; ok {
			e := d.canon[canon]
			words[i] = e.Correction
			applied = append(applied, word+"→"+e.Correction)
		}
	}
	return strings.Join(words, " "), applied
}

// Save 全量写回 dictionary.json（JSON 对象，重启不丢）。
func (d *Dictionary) Save(path string) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	terms := make(map[string]TermEntry, len(d.canon))
	for t, e := range d.canon {
		terms[t] = e
	}
	b, err := json.MarshalIndent(map[string]any{"terms": terms}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// Load 读取 dictionary.json（新格式）；若不存在则尝试迁移旧 dictionary.jsonl。
func (d *Dictionary) Load(path string) error {
	b, err := os.ReadFile(path)
	if err == nil {
		var doc struct {
			Terms map[string]TermEntry `json:"terms"`
		}
		if err := json.Unmarshal(b, &doc); err == nil && len(doc.Terms) > 0 {
			for t, e := range doc.Terms {
				e.Term = t
				d.Add(e.Term, e.Correction, e.Variants, e.Source)
			}
			return nil
		}
		// 旧版直接 map：term → correction
		var legacy map[string]string
		if json.Unmarshal(b, &legacy) == nil && len(legacy) > 0 {
			for w, c := range legacy {
				d.Add(c, c, []string{w}, "legacy")
			}
			return nil
		}
	}
	// 兼容旧 jsonl（{"word":…,"correction":…} 每行）
	file, ferr := os.Open(strings.TrimSuffix(path, ".json") + ".jsonl")
	if ferr != nil {
		if os.IsNotExist(ferr) {
			return nil
		}
		return ferr
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry struct {
			Word       string `json:"word"`
			Correction string `json:"correction"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			continue
		}
		d.Add(entry.Correction, entry.Correction, []string{entry.Word}, "legacy")
	}
	return scanner.Err()
}

// Blacklist "这个改错了"黑名单（term → note），JSON 持久化。
var (
	blacklist   = map[string]string{}
	blacklistMu sync.Mutex
)

func blookup(term string) (string, bool) {
	blacklistMu.Lock()
	defer blacklistMu.Unlock()
	n, ok := blacklist[term]
	return n, ok
}

func badd(term, note string) {
	blacklistMu.Lock()
	blacklist[term] = note
	blacklistMu.Unlock()
}

func blacklistSave(path string) error {
	blacklistMu.Lock()
	defer blacklistMu.Unlock()
	b, err := json.MarshalIndent(blacklist, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func blacklistLoad(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	blacklistMu.Lock()
	blacklist = m
	blacklistMu.Unlock()
	return nil
}

type Intent string

const (
	NOTE        Intent = "NOTE"
	QUERY       Intent = "QUERY"
	EDIT        Intent = "EDIT"
	COMMIT      Intent = "COMMIT"
	ORCHESTRATE Intent = "ORCHESTRATE"
)

func classifyIntent(text string) Intent {
	if strings.Contains(text, "note") {
		return NOTE
	} else if strings.Contains(text, "query") {
		return QUERY
	} else if strings.Contains(text, "edit") {
		return EDIT
	} else if strings.Contains(text, "commit") {
		return COMMIT
	} else {
		return ORCHESTRATE
	}
}

// FeedbackRec 反馈学习记录（append-only feedback.jsonl；✘ 必带 reason）。
type FeedbackRec struct {
	Raw       string `json:"raw"`
	Corrected string `json:"corrected"`
	Accepted  bool   `json:"accepted"`
	Reason    string `json:"reason"`
}

type Trace struct {
	Text   string `json:"text"`
	Intent Intent `json:"intent"`
}

var (
	addr     = flag.String("addr", "127.0.0.1:8080", "HTTP network address")
	dataDir  = flag.String("data-dir", "./data", "Data directory")
	dict     = NewDictionary()
	dictFile = "dictionary.json"
)

func main() {
	flag.Parse()

	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/correct", correctHandler)
	http.HandleFunc("/v1/term", termHandler)
	http.HandleFunc("/v1/dict", dictHandler)
	http.HandleFunc("/v1/feedback", feedbackHandler)
	http.HandleFunc("/v1/blacklist", blacklistHandler)
	http.HandleFunc("/v1/process", processHandler)

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		fmt.Println("Error creating data directory:", err)
		return
	}

	if err := dict.Load(fmt.Sprintf("%s/%s", *dataDir, dictFile)); err != nil {
		fmt.Println("Error loading dictionary:", err)
	}
	if err := blacklistLoad(fmt.Sprintf("%s/blacklist.json", *dataDir)); err != nil {
		fmt.Println("Error loading blacklist:", err)
	}

	fmt.Println("Listening on", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]bool{"ok": true})
}

// correctHandler POST /v1/correct {text} → {corrected, applied}
func correctHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	corrected, applied := dict.CorrectWithApplied(req.Text)
	writeJSON(w, map[string]any{"corrected": corrected, "applied": applied})
}

// termHandler POST /v1/term {term, variants, source, correction?} → 增词并持久化
// 契约：term=标准形式，variants=识别变体（text 命中 variants → 纠为 term），correction 兼容缺省=term。
func termHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Term       string   `json:"term"`
		Correction string   `json:"correction"`
		Variants   []string `json:"variants"`
		Source     string   `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	if req.Term == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "term 必填"})
		return
	}
	if req.Source == "" {
		req.Source = "manual"
	}
	dict.Add(req.Term, req.Correction, req.Variants, req.Source)
	if err := dict.Save(fmt.Sprintf("%s/%s", *dataDir, dictFile)); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "term": req.Term, "variants": req.Variants, "source": req.Source})
}

// dictHandler GET /v1/dict → 全量词典（terms 数组：term/variants/category/source）
func dictHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"terms": dict.Snapshot()})
}

// feedbackHandler POST /v1/feedback {raw, corrected, accepted, reason} → append feedback.jsonl
func feedbackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var rec FeedbackRec
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	if rec.Reason == "" && !rec.Accepted {
		rec.Reason = "user_marked_wrong" // ✘ 必带 reason，空记默认
	}
	file, err := os.OpenFile(fmt.Sprintf("%s/feedback.jsonl", *dataDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer file.Close()
	b, err := json.Marshal(rec)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err := file.Write(append(b, '\n')); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "record": rec})
}

// blacklistHandler POST /v1/blacklist {term, note} → 写入 blacklist.json
func blacklistHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Term string `json:"term"`
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	if req.Term == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "term 必填"})
		return
	}
	badd(req.Term, req.Note)
	if err := blacklistSave(fmt.Sprintf("%s/blacklist.json", *dataDir)); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "term": req.Term, "note": req.Note})
}

// processHandler POST /v1/process {text} → {intent, corrected, corrected_text}
// 契约：intent ∈ NOTE/QUERY/EDIT/COMMIT/ORCHESTRATE；corrected = 纠错结果（corrected_text 兼容保留）。
func processHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	correctedText := dict.Correct(req.Text)
	intent := classifyIntent(correctedText)

	trace := Trace{Text: correctedText, Intent: intent}
	saveTrace(trace)

	writeJSON(w, map[string]any{
		"intent":        intent,
		"corrected":     correctedText,
		"corrected_text": correctedText, // 兼容旧字段
	})
}

func saveTrace(trace Trace) {
	file, err := os.OpenFile(fmt.Sprintf("%s/traces.jsonl", *dataDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening traces file:", err)
		return
	}
	defer file.Close()

	data, err := json.Marshal(trace)
	if err != nil {
		fmt.Println("Error marshaling trace:", err)
		return
	}

	if _, err := file.Write(append(data, '\n')); err != nil {
		fmt.Println("Error writing trace:", err)
	}
}
