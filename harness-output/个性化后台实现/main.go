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

func classifyIntent(text string) Intent {
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

var (
	dictionary = NewDictionary()
	dataDir    string
)

func main() {
	flag.StringVar(&dataDir, "data-dir", "./data", "Directory for data storage")
	flag.Parse()

	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/process", processHandler)

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fmt.Println("Error creating data directory:", err)
		return
	}

	fmt.Println("Server listening on 127.0.0.1:8080")
	http.ListenAndServe("127.0.0.1:8080", nil)
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

	var request struct {
		Text    string `json:"text"`
		Correct bool   `json:"correct"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	correctedText := dictionary.Correct(request.Text)
	intent := classifyIntent(request.Text)

	response := struct {
		CorrectedText string `json:"corrected_text"`
		Intent        Intent `json:"intent"`
	}{
		CorrectedText: correctedText,
		Intent:        intent,
	}

	saveTrace(request.Text, correctedText, intent)
	saveFeedback(request.Text, request.Correct)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func saveTrace(original, corrected string, intent Intent) {
	file, err := os.OpenFile(fmt.Sprintf("%s/traces.jsonl", dataDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening traces file:", err)
		return
	}
	defer file.Close()

	trace := struct {
		Original  string `json:"original"`
		Corrected string `json:"corrected"`
		Intent    Intent `json:"intent"`
	}{
		Original:  original,
		Corrected: corrected,
		Intent:    intent,
	}

	data, err := json.Marshal(trace)
	if err != nil {
		fmt.Println("Error marshaling trace:", err)
		return
	}

	writer := bufio.NewWriter(file)
	writer.Write(data)
	writer.WriteString("\n")
	writer.Flush()
}

func saveFeedback(text string, correct bool) {
	file, err := os.OpenFile(fmt.Sprintf("%s/feedback.jsonl", dataDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening feedback file:", err)
		return
	}
	defer file.Close()

	feedback := Feedback{
		Text:   text,
		Correct: correct,
	}

	data, err := json.Marshal(feedback)
	if err != nil {
		fmt.Println("Error marshaling feedback:", err)
		return
	}

	writer := bufio.NewWriter(file)
	writer.Write(data)
	writer.WriteString("\n")
	writer.Flush()
}