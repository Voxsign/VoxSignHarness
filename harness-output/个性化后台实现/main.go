package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
)

// Dictionary 用户教词词典（term → correction），线程安全，JSONL 持久化。
type Dictionary struct {
	entries map[string]string
	mu      sync.RWMutex
}

func NewDictionary() *Dictionary {
	return &Dictionary{entries: make(map[string]string)}
}

func (d *Dictionary) Add(word, correction string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries[word] = correction
}

func (d *Dictionary) Delete(word string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.entries, word)
}

func (d *Dictionary) Lookup(word string) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	correction, exists := d.entries[word]
	return correction, exists
}

// Snapshot 返回全量词条（dict 端点用）。
func (d *Dictionary) Snapshot() []map[string]string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]map[string]string, 0, len(d.entries))
	for w, c := range d.entries {
		out = append(out, map[string]string{"word": w, "correction": c})
	}
	return out
}

// Correct 应用词典纠错（黑名单词跳过）。
func (d *Dictionary) Correct(text string) string {
	corrected, _ := d.CorrectWithApplied(text)
	return corrected
}

// CorrectWithApplied 返回纠错结果 + 实际应用的映射列表（correct 端点用）。
func (d *Dictionary) CorrectWithApplied(text string) (string, []string) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	words := strings.Fields(text)
	var applied []string
	for i, word := range words {
		if _, blocked := blookup(word); blocked {
			continue
		}
		if correction, exists := d.entries[word]; exists {
			words[i] = correction
			applied = append(applied, word+"→"+correction)
		}
	}
	return strings.Join(words, " "), applied
}

// Save 全量写回 dictionary.jsonl（重启不丢）。
func (d *Dictionary) Save(path string) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var buf bytes.Buffer
	for w, c := range d.entries {
		b, err := json.Marshal(map[string]string{"word": w, "correction": c})
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
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

type Feedback struct {
	Text    string `json:"text"`
	Correct bool   `json:"correct"`
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
	dictFile = "dictionary.jsonl"
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

	loadDictionary()
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

// termHandler POST /v1/term {term, correction} → 增词并持久化
func termHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Term       string `json:"term"`
		Correction string `json:"correction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	if req.Term == "" || req.Correction == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "term 与 correction 均必填"})
		return
	}
	dict.Add(req.Term, req.Correction)
	if err := dict.Save(fmt.Sprintf("%s/%s", *dataDir, dictFile)); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "term": req.Term, "correction": req.Correction})
}

// dictHandler GET /v1/dict → 全量词典
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

	resp := struct {
		CorrectedText string `json:"corrected_text"`
		Intent        Intent `json:"intent"`
	}{
		CorrectedText: correctedText,
		Intent:        intent,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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

func loadDictionary() {
	file, err := os.Open(fmt.Sprintf("%s/%s", *dataDir, dictFile))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		fmt.Println("Error opening dictionary file:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry struct {
			Word       string `json:"word"`
			Correction string `json:"correction"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			fmt.Println("Error unmarshaling dictionary entry:", err)
			continue
		}
		dict.Add(entry.Word, entry.Correction)
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading dictionary file:", err)
	}
}
