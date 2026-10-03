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
	words map[string]struct{}
	mu    sync.RWMutex
}

func NewDictionary() *Dictionary {
	return &Dictionary{words: make(map[string]struct{})}
}

func (d *Dictionary) Add(word string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.words[word] = struct{}{}
}

func (d *Dictionary) Delete(word string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.words, word)
}

func (d *Dictionary) Exists(word string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, exists := d.words[word]
	return exists
}

func (d *Dictionary) Correct(word string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, exists := d.words[word]; exists {
		return word
	}
	// Simple correction: return the first word that starts with the same letter
	for w := range d.words {
		if strings.HasPrefix(w, string(word[0])) {
			return w
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

func classifyIntent(text string) Intent {
	if strings.HasPrefix(text, "note") {
		return NOTE
	} else if strings.HasPrefix(text, "query") {
		return QUERY
	} else if strings.HasPrefix(text, "edit") {
		return EDIT
	} else if strings.HasPrefix(text, "commit") {
		return COMMIT
	} else {
		return ORCHESTRATE
	}
}

type Feedback struct {
	Text   string `json:"text"`
	Correct bool   `json:"correct"`
}

func appendToFile(filename string, data interface{}) error {
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

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func processHandler(dict *Dictionary, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		corrected := dict.Correct(req.Text)
		intent := classifyIntent(req.Text)

		resp := struct {
			Corrected string `json:"corrected"`
			Intent    Intent `json:"intent"`
		}{
			Corrected: corrected,
			Intent:    intent,
		}

		appendToFile(dataDir+"/traces.jsonl", req)
		appendToFile(dataDir+"/usage.jsonl", resp)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func feedbackHandler(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var feedback Feedback
		if err := json.NewDecoder(r.Body).Decode(&feedback); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		appendToFile(dataDir+"/feedback.jsonl", feedback)

		w.WriteHeader(http.StatusOK)
	}
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
	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/process", processHandler(dict, *dataDir))
	http.HandleFunc("/v1/feedback", feedbackHandler(*dataDir))

	fmt.Println("Starting server on", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}