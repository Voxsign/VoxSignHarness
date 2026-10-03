package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
)

var (
	addr    = flag.String("addr", "127.0.0.1:8080", "HTTP network address")
	dataDir = flag.String("data-dir", "./data", "Data directory")
)

type Dictionary struct {
	entries map[string]string
	mu      sync.RWMutex
}

func NewDictionary() *Dictionary {
	return &Dictionary{entries: make(map[string]string)}
}

func (d *Dictionary) Add(term, definition string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries[term] = definition
}

func (d *Dictionary) Delete(term string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.entries, term)
}

func (d *Dictionary) Lookup(term string) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	definition, exists := d.entries[term]
	return definition, exists
}

func (d *Dictionary) Correct(term string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, exists := d.entries[term]; exists {
		return term
	}
	// Simple correction logic: return the first entry that starts with the same letter
	for k := range d.entries {
		if strings.HasPrefix(k, string(term[0])) {
			return k
		}
	}
	return term
}

type Feedback struct {
	Term   string `json:"term"`
	Correct bool   `json:"correct"`
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

	_, err = file.WriteString(string(data) + "\n")
	return err
}

func saveTrace(trace map[string]interface{}) error {
	file, err := os.OpenFile(fmt.Sprintf("%s/traces.jsonl", *dataDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(trace)
	if err != nil {
		return err
	}

	_, err = file.WriteString(string(data) + "\n")
	return err
}

func classifyIntent(text string) string {
	if strings.Contains(text, "note") {
		return "NOTE"
	} else if strings.Contains(text, "query") {
		return "QUERY"
	} else if strings.Contains(text, "edit") {
		return "EDIT"
	} else if strings.Contains(text, "commit") {
		return "COMMIT"
	} else {
		return "ORCHESTRATE"
	}
}

func main() {
	flag.Parse()

	dict := NewDictionary()

	http.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	http.HandleFunc("/v1/dict", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var entry struct {
				Term       string `json:"term"`
				Definition string `json:"definition"`
			}
			if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}
			dict.Add(entry.Term, entry.Definition)
			w.WriteHeader(http.StatusCreated)
		} else if r.Method == http.MethodDelete {
			term := r.URL.Query().Get("term")
			dict.Delete(term)
			w.WriteHeader(http.StatusNoContent)
		} else if r.Method == http.MethodGet {
			term := r.URL.Query().Get("term")
			if definition, exists := dict.Lookup(term); exists {
				json.NewEncoder(w).Encode(map[string]string{"term": term, "definition": definition})
			} else {
				http.Error(w, "Not found", http.StatusNotFound)
			}
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/v1/term", func(w http.ResponseWriter, r *http.Request) {
		term := r.URL.Query().Get("term")
		if definition, exists := dict.Lookup(term); exists {
			json.NewEncoder(w).Encode(map[string]string{"term": term, "definition": definition})
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	})

	http.HandleFunc("/v1/correct", func(w http.ResponseWriter, r *http.Request) {
		term := r.URL.Query().Get("term")
		corrected := dict.Correct(term)
		json.NewEncoder(w).Encode(map[string]string{"original": term, "corrected": corrected})
	})

	http.HandleFunc("/v1/feedback", func(w http.ResponseWriter, r *http.Request) {
		var feedback Feedback
		if err := json.NewDecoder(r.Body).Decode(&feedback); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if err := saveFeedback(feedback); err != nil {
			http.Error(w, "Failed to save feedback", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})

	http.HandleFunc("/v1/blacklist", func(w http.ResponseWriter, r *http.Request) {
		// Placeholder for blacklist functionality
		w.WriteHeader(http.StatusNotImplemented)
	})

	http.HandleFunc("/v1/process", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		intent := classifyIntent(request.Text)
		trace := map[string]interface{}{
			"text":   request.Text,
			"intent": intent,
		}

		if err := saveTrace(trace); err != nil {
			http.Error(w, "Failed to save trace", http.StatusInternalServerError)
			return
		}

		response := map[string]string{"intent": intent}
		json.NewEncoder(w).Encode(response)
	})

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		fmt.Printf("Failed to create data directory: %v\n", err)
		return
	}

	fmt.Printf("Listening on %s...\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
	}
}