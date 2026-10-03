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
	if strings.Contains(text, "note") {
		return NOTE
	} else if strings.Contains(text, "query") {
		return QUERY
	} else if strings.Contains(text, "edit") {
		return EDIT
	} else if strings.Contains(text, "commit") {
		return COMMIT
	} else if strings.Contains(text, "orchestrate") {
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

func main() {
	flag.Parse()

	http.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	http.HandleFunc("/v1/process", func(w http.ResponseWriter, r *http.Request) {
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
	})

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create data directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Starting server on %s\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
	}
}