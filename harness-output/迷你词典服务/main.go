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

type Feedback struct {
	Input    string `json:"input"`
	Feedback string `json:"feedback"`
}

type Request struct {
	Text string `json:"text"`
}

type Response struct {
	Result string `json:"result"`
}

var (
	dictionary = &Dictionary{entries: make(map[string]string)}
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

	loadDictionary()
	http.ListenAndServe("127.0.0.1:8080", nil)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func processHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	result := processText(req.Text)
	resp := Response{Result: result}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	appendToFile("traces.jsonl", req.Text)
}

func processText(text string) string {
	cleanedText := cleanText(text)
	correctedText := correctText(cleanedText)
	intent := classifyIntent(correctedText)
	return fmt.Sprintf("Intent: %s, Text: %s", intent, correctedText)
}

func cleanText(text string) string {
	return strings.TrimSpace(text)
}

func correctText(text string) string {
	words := strings.Fields(text)
	for i, word := range words {
		if corrected, found := dictionary.Lookup(word); found {
			words[i] = corrected
		}
	}
	return strings.Join(words, " ")
}

func classifyIntent(text string) string {
	if strings.HasPrefix(text, "note") {
		return "NOTE"
	} else if strings.HasPrefix(text, "query") {
		return "QUERY"
	} else if strings.HasPrefix(text, "edit") {
		return "EDIT"
	} else if strings.HasPrefix(text, "commit") {
		return "COMMIT"
	} else {
		return "ORCHESTRATE"
	}
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
	correction, found := d.entries[word]
	return correction, found
}

func loadDictionary() {
	file, err := os.Open(dataDir + "/dictionary.jsonl")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		fmt.Println("Error loading dictionary:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry map[string]string
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			for k, v := range entry {
				dictionary.Add(k, v)
			}
		}
	}
}

func appendToFile(filename, text string) {
	file, err := os.OpenFile(dataDir+"/"+filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	entry := map[string]string{"text": text}
	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Println("Error marshaling entry:", err)
		return
	}

	writer.WriteString(string(data) + "\n")
}