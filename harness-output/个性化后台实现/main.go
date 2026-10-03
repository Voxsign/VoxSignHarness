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

var (
	addr     = flag.String("addr", "127.0.0.1:8080", "HTTP network address")
	dataDir  = flag.String("data-dir", "./data", "Data directory")
	authKey  = "secret" // Placeholder for authentication
	dict     = make(map[string]string)
	dictLock sync.RWMutex
)

type Response struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func main() {
	flag.Parse()

	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/process", processHandler)
	http.HandleFunc("/v1/blacklist", blacklistHandler)
	http.HandleFunc("/v1/dict", dictHandler)
	http.HandleFunc("/v1/term", termHandler)
	http.HandleFunc("/v1/correct", correctHandler)
	http.HandleFunc("/v1/feedback", feedbackHandler)

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		fmt.Println("Error creating data directory:", err)
		return
	}

	fmt.Println("Server is listening on", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode(Response{Status: "ok"})
}

func processHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Placeholder for processing logic
	json.NewEncoder(w).Encode(Response{Status: "processed"})
}

func blacklistHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Placeholder for blacklist logic
	json.NewEncoder(w).Encode(Response{Status: "blacklisted"})
}

func dictHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		dictLock.RLock()
		defer dictLock.RUnlock()
		json.NewEncoder(w).Encode(dict)
		return
	}

	if r.Method == http.MethodPost {
		var entry map[string]string
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		dictLock.Lock()
		for k, v := range entry {
			dict[k] = v
		}
		dictLock.Unlock()
		json.NewEncoder(w).Encode(Response{Status: "added"})
		return
	}

	if r.Method == http.MethodDelete {
		var entry map[string]string
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		dictLock.Lock()
		for k := range entry {
			delete(dict, k)
		}
		dictLock.Unlock()
		json.NewEncoder(w).Encode(Response{Status: "deleted"})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func termHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Placeholder for term logic
	json.NewEncoder(w).Encode(Response{Status: "term"})
}

func correctHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	correctedText := correctText(request.Text)
	json.NewEncoder(w).Encode(map[string]string{"corrected": correctedText})
}

func correctText(text string) string {
	dictLock.RLock()
	defer dictLock.RUnlock()
	words := strings.Fields(text)
	for i, word := range words {
		if correction, exists := dict[word]; exists {
			words[i] = correction
		}
	}
	return strings.Join(words, " ")
}

func feedbackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var feedback struct {
		Feedback string `json:"feedback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&feedback); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	file, err := os.OpenFile(fmt.Sprintf("%s/feedback.jsonl", *dataDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	if err := json.NewEncoder(writer).Encode(feedback); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writer.Flush()
	json.NewEncoder(w).Encode(Response{Status: "feedback recorded"})
}