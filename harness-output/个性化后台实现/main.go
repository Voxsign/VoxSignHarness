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

type Trace struct {
	Text   string `json:"text"`
	Intent Intent `json:"intent"`
}

var (
	addr     = flag.String("addr", "127.0.0.1:8080", "HTTP network address")
	dataDir  = flag.String("data-dir", "./data", "Data directory")
	dict     = NewDictionary()
	dictFile = "dictionary.jsonl"
)

func main() {
	flag.Parse()

	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/process", processHandler)

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		fmt.Println("Error creating data directory:", err)
		return
	}

	loadDictionary()

	fmt.Println("Listening on", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
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

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	correctedText := dict.Correct(req.Text)
	intent := classifyIntent(correctedText)

	trace := Trace{Text: correctedText, Intent: intent}
	saveTrace(trace)

	resp := struct {
		CorrectedText string `json:"corrected_text"`
		Intent        Intent `json:"intent"`
	}{
		CorrectedText: correctedText,
		Intent:        intent,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func saveTrace(trace Trace) {
	file, err := os.OpenFile(fmt.Sprintf("%s/traces.jsonl", *dataDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening traces file:", err)
		return
	}
	defer file.Close()

	data, err := json.Marshal(trace)
	if err != nil {
		fmt.Println("Error marshaling trace:", err)
		return
	}

	if _, err := file.Write(append(data, '\n')); err != nil {
		fmt.Println("Error writing trace:", err)
	}
}

func loadDictionary() {
	file, err := os.Open(fmt.Sprintf("%s/%s", *dataDir, dictFile))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		fmt.Println("Error opening dictionary file:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry struct {
			Word       string `json:"word"`
			Correction string `json:"correction"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			fmt.Println("Error unmarshaling dictionary entry:", err)
			continue
		}
		dict.Add(entry.Word, entry.Correction)
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading dictionary file:", err)
	}
}