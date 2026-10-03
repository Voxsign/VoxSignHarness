package main

import (
	"bufio"
	"encoding/json"
	"errors"
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

func (d *Dictionary) Correct(text string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	words := strings.Fields(text)
	for i, word := range words {
		if correction, exists := d.entries[word]; exists {
			words[i] = correction
		}
	}
	return strings.Join(words, " ")
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
	if strings.HasPrefix(text, "note:") {
		return NOTE
	} else if strings.HasPrefix(text, "query:") {
		return QUERY
	} else if strings.HasPrefix(text, "edit:") {
		return EDIT
	} else if strings.HasPrefix(text, "commit:") {
		return COMMIT
	} else if strings.HasPrefix(text, "orchestrate:") {
		return ORCHESTRATE
	}
	return NOTE
}

type Feedback struct {
	Text   string `json:"text"`
	Correct bool   `json:"correct"`
}

func AppendToFile(filename string, data interface{}) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	if err := encoder.Encode(data); err != nil {
		return err
	}
	return writer.Flush()
}

type Request struct {
	Text string `json:"text"`
}

type Response struct {
	CorrectedText string `json:"corrected_text"`
	Intent        Intent `json:"intent"`
}

var (
	addr    = flag.String("addr", "127.0.0.1:8080", "HTTP network address")
	dataDir = flag.String("data-dir", "./data", "Data directory")
	dict    = NewDictionary()
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func processHandler(w http.ResponseWriter, r *http.Request) {
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

	if err := AppendToFile(fmt.Sprintf("%s/traces.jsonl", *dataDir), req); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := AppendToFile(fmt.Sprintf("%s/usage.jsonl", *dataDir), resp); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func feedbackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var feedback Feedback
	if err := json.NewDecoder(r.Body).Decode(&feedback); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if err := AppendToFile(fmt.Sprintf("%s/feedback.jsonl", *dataDir), feedback); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	flag.Parse()

	if _, err := os.Stat(*dataDir); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(*dataDir, 0755); err != nil {
			fmt.Printf("Failed to create data directory: %v\n", err)
			return
		}
	}

	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/process", processHandler)
	http.HandleFunc("/v1/feedback", feedbackHandler)

	fmt.Printf("Starting server on %s\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}