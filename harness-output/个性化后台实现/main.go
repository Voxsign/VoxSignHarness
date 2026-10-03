package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
)

type Dictionary struct {
	entries map[string]string
	mu      sync.RWMutex
}

func NewDictionary() *Dictionary {
	return &Dictionary{entries: make(map[string]string)}
}

func (d *Dictionary) Add(word, definition string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries[word] = definition
}

func (d *Dictionary) Delete(word string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.entries, word)
}

func (d *Dictionary) Lookup(word string) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	definition, exists := d.entries[word]
	return definition, exists
}

func (d *Dictionary) Correct(word string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, exists := d.entries[word]; exists {
		return word
	}
	// Simple correction: return the first word that starts with the same letter
	for entry := range d.entries {
		if strings.HasPrefix(entry, string(word[0])) {
			return entry
		}
	}
	return word
}

type Intent string

const (
	NOTE        Intent = "NOTE"
	QUERY       Intent = "QUERY"
	EDIT        Intent = "EDIT"
	COMMIT      Intent = "COMMIT"
	ORCHESTRATE Intent = "ORCHESTRATE"
)

func ClassifyIntent(text string) Intent {
	text = strings.ToLower(text)
	switch {
	case strings.Contains(text, "note"):
		return NOTE
	case strings.Contains(text, "query"):
		return QUERY
	case strings.Contains(text, "edit"):
		return EDIT
	case strings.Contains(text, "commit"):
		return COMMIT
	default:
		return ORCHESTRATE
	}
}

type Feedback struct {
	Text   string `json:"text"`
	Correct bool   `json:"correct"`
}

func AppendFeedback(filePath string, feedback Feedback) error {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(feedback)
}

type Request struct {
	Text string `json:"text"`
}

type Response struct {
	CorrectedText string `json:"corrected_text"`
	Intent        Intent `json:"intent"`
}

func processHandler(dict *Dictionary, feedbackPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		correctedText := dict.Correct(req.Text)
		intent := ClassifyIntent(req.Text)

		resp := Response{
			CorrectedText: correctedText,
			Intent:        intent,
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		feedback := Feedback{
			Text:   req.Text,
			Correct: correctedText == req.Text,
		}
		if err := AppendFeedback(feedbackPath, feedback); err != nil {
			fmt.Println("Error appending feedback:", err)
		}
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP network address")
	dataDir := flag.String("data-dir", "./data", "Data directory")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		fmt.Println("Error creating data directory:", err)
		return
	}

	dict := NewDictionary()
	feedbackPath := *dataDir + "/feedback.jsonl"

	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/process", processHandler(dict, feedbackPath))

	fmt.Println("Starting server on", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}