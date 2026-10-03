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

type Server struct {
	dictionary *Dictionary
	dataDir    string
}

func NewServer(dataDir string) *Server {
	return &Server{
		dictionary: NewDictionary(),
		dataDir:    dataDir,
	}
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) processHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	correctedText := s.dictionary.Correct(request.Text)
	intent := classifyIntent(request.Text)

	response := struct {
		CorrectedText string `json:"corrected_text"`
		Intent        Intent `json:"intent"`
	}{
		CorrectedText: correctedText,
		Intent:        intent,
	}

	if err := appendToFile(s.dataDir+"/traces.jsonl", request); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) feedbackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var feedback Feedback
	if err := json.NewDecoder(r.Body).Decode(&feedback); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if err := appendToFile(s.dataDir+"/feedback.jsonl", feedback); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP network address")
	dataDir := flag.String("data-dir", "./data", "Data directory")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating data directory: %v\n", err)
		os.Exit(1)
	}

	server := NewServer(*dataDir)

	http.HandleFunc("/v1/health", server.healthHandler)
	http.HandleFunc("/v1/process", server.processHandler)
	http.HandleFunc("/v1/feedback", server.feedbackHandler)

	fmt.Printf("Starting server on %s\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}
}