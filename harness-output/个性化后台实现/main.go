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
	for entry := range d.entries {
		if strings.HasPrefix(entry, word) || strings.HasSuffix(entry, word) {
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

func classifyIntent(text string) Intent {
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

type Request struct {
	Text string `json:"text"`
}

type Response struct {
	CorrectedText string `json:"corrected_text"`
	Intent        Intent `json:"intent"`
}

var (
	dictionary = NewDictionary()
	dataDir    string
)

func init() {
	flag.StringVar(&dataDir, "data-dir", "./data", "Directory for data storage")
}

func main() {
	flag.Parse()

	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/process", processHandler)

	if err := os.MkdirAll(dataDir, os.ModePerm); err != nil {
		fmt.Println("Error creating data directory:", err)
		return
	}

	fmt.Println("Server listening on 127.0.0.1:8080")
	if err := http.ListenAndServe("127.0.0.1:8080", nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
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

	correctedText := dictionary.Correct(req.Text)
	intent := classifyIntent(req.Text)

	resp := Response{
		CorrectedText: correctedText,
		Intent:        intent,
	}

	if err := logTrace(req.Text, correctedText, intent); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func logTrace(originalText, correctedText string, intent Intent) error {
	file, err := os.OpenFile(fmt.Sprintf("%s/traces.jsonl", dataDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	trace := map[string]interface{}{
		"original_text":  originalText,
		"corrected_text": correctedText,
		"intent":         intent,
	}

	data, err := json.Marshal(trace)
	if err != nil {
		return err
	}

	writer := bufio.NewWriter(file)
	if _, err := writer.WriteString(string(data) + "\n"); err != nil {
		return err
	}
	return writer.Flush()
}

func logFeedback(feedback Feedback) error {
	file, err := os.OpenFile(fmt.Sprintf("%s/feedback.jsonl", dataDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(feedback)
	if err != nil {
		return err
	}

	writer := bufio.NewWriter(file)
	if _, err := writer.WriteString(string(data) + "\n"); err != nil {
		return err
	}
	return writer.Flush()
}