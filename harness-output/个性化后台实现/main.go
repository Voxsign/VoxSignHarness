package main

import (
	"bufio"
	"encoding/json"
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
	// Simple correction logic: return the first word that starts with the same letter
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
	Correct bool   `json:"correct"`
	Comment string `json:"comment"`
}

func appendToFile(filePath string, data interface{}) error {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

func processHandler(dict *Dictionary, feedbackFilePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Text    string   `json:"text"`
			Feedback Feedback `json:"feedback"`
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		correctedText := dict.Correct(request.Text)
		intent := classifyIntent(request.Text)

		response := map[string]interface{}{
			"corrected_text": correctedText,
			"intent":         intent,
		}

		if err := appendToFile(feedbackFilePath, request.Feedback); err != nil {
			http.Error(w, "Failed to save feedback", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, response)
	}
}

func main() {
	dict := NewDictionary()
	dict.Add("hello", "A greeting")
	dict.Add("world", "The earth")

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	feedbackFilePath := fmt.Sprintf("%s/feedback.jsonl", dataDir)

	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/process", processHandler(dict, feedbackFilePath))

	addr := "127.0.0.1:8080"
	fmt.Printf("Listening on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
	}
}