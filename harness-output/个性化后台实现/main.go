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

type Request struct {
	Text string `json:"text"`
}

type Response struct {
	CorrectedText string `json:"corrected_text"`
	Intent        Intent `json:"intent"`
}

var (
	addr     = flag.String("addr", "127.0.0.1:8080", "HTTP network address")
	dataDir  = flag.String("data-dir", "./data", "Data directory")
	dict     = NewDictionary()
	dictFile = "dictionary.jsonl"
)

func loadDictionary() error {
	file, err := os.Open(fmt.Sprintf("%s/%s", *dataDir, dictFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry map[string]string
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return err
		}
		for k, v := range entry {
			dict.Add(k, v)
		}
	}
	return scanner.Err()
}

func saveFeedback(feedback Feedback) error {
	file, err := os.OpenFile(fmt.Sprintf("%s/feedback.jsonl", *dataDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(feedback)
	if err != nil {
		return err
	}

	_, err = file.Write(append(data, '\n'))
	return err
}

func saveTrace(trace interface{}) error {
	file, err := os.OpenFile(fmt.Sprintf("%s/traces.jsonl", *dataDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(trace)
	if err != nil {
		return err
	}

	_, err = file.Write(append(data, '\n'))
	return err
}

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
	intent := classifyIntent(req.Text)

	resp := Response{
		CorrectedText: correctedText,
		Intent:        intent,
	}

	if err := saveTrace(resp); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating data directory: %v\n", err)
		os.Exit(1)
	}

	if err := loadDictionary(); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading dictionary: %v\n", err)
		os.Exit(1)
	}

	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/process", processHandler)

	fmt.Printf("Listening on %s...\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}
}