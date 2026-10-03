package main

// VoxSign ASR 个性化后台服务（需求规格 v3 契约版，仅标准库）
// 端点：GET /v1/health | GET /v1/dict | POST /v1/term | POST /v1/correct |
//       POST /v1/process | POST /v1/feedback | POST /v1/blacklist(+GET/DELETE)
// 持久化：data/dictionary.json + data/blacklist.json（启动加载/写回）、data/feedback.jsonl（append-only）

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

var (
	addr    = flag.String("addr", "127.0.0.1:8080", "HTTP network address")
	dataDir = flag.String("data-dir", "./data", "Data directory")
)

// ---------- 数据模型（契约 v3） ----------

type TermEntry struct {
	Term     string   `json:"term"`
	Variants []string `json:"variants,omitempty"`
	Category string   `json:"category,omitempty"`
	Source   string   `json:"source,omitempty"`
}

type Dictionary struct {
	Terms map[string]*TermEntry `json:"terms"`
	mu    sync.RWMutex
}

func NewDictionary() *Dictionary {
	return &Dictionary{Terms: make(map[string]*TermEntry)}
}

func (d *Dictionary) Add(entry TermEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Terms[entry.Term] = &entry
}

// Lookup 先精确命中 term，再命中 variants（变体 → 规范词）。
func (d *Dictionary) Lookup(word string) (*TermEntry, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if e, ok := d.Terms[word]; ok {
		return e, true
	}
	for _, e := range d.Terms {
		for _, v := range e.Variants {
			if v == word {
				return e, true
			}
		}
	}
	return nil, false
}

// Save 原子写回 dictionary.json（临时文件 + rename）。
func (d *Dictionary) Save(dir string) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return writeJSONAtomic(filepath.Join(dir, "dictionary.json"), d)
}

func (d *Dictionary) Load(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "dictionary.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return json.Unmarshal(data, d)
}

type Blacklist struct {
	Terms map[string]string `json:"terms"` // term → note
	mu    sync.RWMutex
}

func NewBlacklist() *Blacklist {
	return &Blacklist{Terms: make(map[string]string)}
}

func (b *Blacklist) Add(term, note string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Terms[term] = note
}

func (b *Blacklist) Remove(term string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.Terms, term)
}

func (b *Blacklist) Contains(term string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.Terms[term]
	return ok
}

func (b *Blacklist) Save(dir string) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return writeJSONAtomic(filepath.Join(dir, "blacklist.json"), b)
}

func (b *Blacklist) Load(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "blacklist.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return json.Unmarshal(data, b)
}

type Feedback struct {
	Raw       string `json:"raw"`
	Corrected string `json:"corrected"`
	Accepted  bool   `json:"accepted"`
	Reason    string `json:"reason"`
}

// ---------- 持久化 ----------

func writeJSONAtomic(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func appendFeedback(fb Feedback, dir string) error {
	line, err := json.Marshal(fb)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, "feedback.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(string(line) + "\n")
	return err
}

// ---------- 纠错 / 意图 ----------

// correctText 应用词典映射与黑名单拦截：
//  1. 黑名单命中 → 不纠（applied: blacklist-skip）
//  2. 词典精确/变体命中 → 纠为规范词（applied: dict）
//  3. 前缀相似（rune 级，中文安全）→ 纠为第一个同前缀词（applied: prefix）
//  4. 其余原样（applied: none）
func correctText(text string, dict *Dictionary, bl *Blacklist) (string, []string) {
	if text == "" {
		return text, []string{"none"}
	}
	trimmed := strings.TrimSpace(text)
	if bl.Contains(trimmed) {
		return text, []string{"blacklist-skip:" + trimmed}
	}
	if e, ok := dict.Lookup(trimmed); ok {
		return e.Term, []string{"dict:" + trimmed + "→" + e.Term}
	}
	// rune 前缀相似（不切坏 UTF-8 中文）。
	first, _ := utf8.DecodeRuneInString(trimmed)
	dict.mu.RLock()
	for _, e := range dict.Terms {
		kFirst, _ := utf8.DecodeRuneInString(e.Term)
		if kFirst == first {
			dict.mu.RUnlock()
			return e.Term, []string{"prefix:" + trimmed + "→" + e.Term}
		}
	}
	dict.mu.RUnlock()
	return text, []string{"none"}
}

func classifyIntent(text string) string {
	t := strings.ToLower(text)
	switch {
	case strings.Contains(t, "note"):
		return "NOTE"
	case strings.Contains(t, "query"), strings.Contains(t, "查"):
		return "QUERY"
	case strings.Contains(t, "edit"), strings.Contains(t, "改"):
		return "EDIT"
	case strings.Contains(t, "commit"), strings.Contains(t, "提交"):
		return "COMMIT"
	default:
		return "ORCHESTRATE"
	}
}

// ---------- HTTP ----------

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func methodGuard(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func main() {
	flag.Parse()
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		fmt.Printf("Failed to create data directory: %v\n", err)
		return
	}

	dict := NewDictionary()
	bl := NewBlacklist()
	if err := dict.Load(*dataDir); err != nil {
		fmt.Printf("Failed to load dictionary: %v\n", err)
	}
	if err := bl.Load(*dataDir); err != nil {
		fmt.Printf("Failed to load blacklist: %v\n", err)
	}

	// 1) 健康检查。
	http.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		if !methodGuard(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	// 2) 全量词典（terms 数组）。
	http.HandleFunc("/v1/dict", func(w http.ResponseWriter, r *http.Request) {
		if !methodGuard(w, r, http.MethodGet) {
			return
		}
		dict.mu.RLock()
		terms := make([]*TermEntry, 0, len(dict.Terms))
		for _, e := range dict.Terms {
			terms = append(terms, e)
		}
		dict.mu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]interface{}{"terms": terms, "count": len(terms)})
	})

	// 3) 增词（term/variants/source），写回 dictionary.json。
	http.HandleFunc("/v1/term", func(w http.ResponseWriter, r *http.Request) {
		if !methodGuard(w, r, http.MethodPost) {
			return
		}
		var entry TermEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil || strings.TrimSpace(entry.Term) == "" {
			http.Error(w, `{"error":"term required"}`, http.StatusBadRequest)
			return
		}
		dict.Add(entry)
		if err := dict.Save(*dataDir); err != nil {
			http.Error(w, `{"error":"persist failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"term": entry.Term, "saved": "dictionary.json"})
	})

	// 4) 个性化纠错：{text} → {corrected, applied}。
	http.HandleFunc("/v1/correct", func(w http.ResponseWriter, r *http.Request) {
		if !methodGuard(w, r, http.MethodPost) {
			return
		}
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		corrected, applied := correctText(req.Text, dict, bl)
		writeJSON(w, http.StatusOK, map[string]interface{}{"corrected": corrected, "applied": applied})
	})

	// 5) 意图分类 + 纠错：{text} → {intent, corrected}。
	http.HandleFunc("/v1/process", func(w http.ResponseWriter, r *http.Request) {
		if !methodGuard(w, r, http.MethodPost) {
			return
		}
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		intent := classifyIntent(req.Text)
		corrected, applied := correctText(req.Text, dict, bl)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"intent": intent, "corrected": corrected, "applied": applied,
		})
	})

	// 6) 反馈学习：{raw, corrected, accepted, reason}，reason 必带，JSONL append-only。
	http.HandleFunc("/v1/feedback", func(w http.ResponseWriter, r *http.Request) {
		if !methodGuard(w, r, http.MethodPost) {
			return
		}
		var fb Feedback
		if err := json.NewDecoder(r.Body).Decode(&fb); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(fb.Reason) == "" {
			http.Error(w, `{"error":"reason required（✘ 必带原因）"}`, http.StatusBadRequest)
			return
		}
		if err := appendFeedback(fb, *dataDir); err != nil {
			http.Error(w, `{"error":"persist failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"saved": "feedback.jsonl"})
	})

	// 7) 黑名单：POST {term, note} → blacklist.json；GET 列表；DELETE ?term= 解除。
	http.HandleFunc("/v1/blacklist", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var req struct {
				Term string `json:"term"`
				Note string `json:"note"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Term) == "" {
				http.Error(w, `{"error":"term required"}`, http.StatusBadRequest)
				return
			}
			bl.Add(req.Term, req.Note)
			if err := bl.Save(*dataDir); err != nil {
				http.Error(w, `{"error":"persist failed"}`, http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"term": req.Term, "saved": "blacklist.json"})
		case http.MethodGet:
			bl.mu.RLock()
			terms := make([]map[string]string, 0, len(bl.Terms))
			for t, n := range bl.Terms {
				terms = append(terms, map[string]string{"term": t, "note": n})
			}
			bl.mu.RUnlock()
			writeJSON(w, http.StatusOK, map[string]interface{}{"blacklist": terms, "count": len(terms)})
		case http.MethodDelete:
			term := r.URL.Query().Get("term")
			if term == "" {
				http.Error(w, `{"error":"term required"}`, http.StatusBadRequest)
				return
			}
			bl.Remove(term)
			_ = bl.Save(*dataDir)
			writeJSON(w, http.StatusOK, map[string]string{"term": term, "removed": "true"})
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	fmt.Printf("Listening on %s...\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
	}
}
