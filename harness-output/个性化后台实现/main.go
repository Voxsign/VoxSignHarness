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

func processHandler(dict *Dictionary, dataDir string) http.HandlerFunc {
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

		if err := AppendToFile(dataDir+"/traces.jsonl", req); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := AppendToFile(dataDir+"/usage.jsonl", resp); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
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
		fmt.Fprintf(os.Stderr, "Error creating data directory: %v\n", err)
		os.Exit(1)
	}

	dict := NewDictionary()
	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/process", processHandler(dict, *dataDir))

	fmt.Printf("Starting server on %s\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}
}