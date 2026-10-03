package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var (
	dataDir string
	dictionary = make(map[string]string)
	mu sync.Mutex
)

type HealthResponse struct {
	Status string `json:"status"`
}

type ProcessRequest struct {
	Text      string `json:"text"`
	SessionID string `json:"session_id"`
	AudioMeta struct {
		Engine    string  `json:"engine"`
		Confidence float64 `json:"confidence"`
		Lang      string  `json:"lang"`
	} `json:"audio_meta"`
}

type ProcessResponse struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	Params   map[string]interface{} `json:"params"`
	Confidence float64 `json:"confidence"`
}

func init() {
	dataDir = os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	loadDictionary()
}

func loadDictionary() {
	filePath := filepath.Join(dataDir, "custom-dictionary.json")
	data, err := ioutil.ReadFile(filePath)
	if err == nil {
		json.Unmarshal(data, &dictionary)
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{Status: "healthy"}
	writeJSON(w, http.StatusOK, response)
}

func processHandler(w http.ResponseWriter, r *http.Request) {
	var req ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	
	mu.Lock()
	defer mu.Unlock()

	// Simulate processing
	response := ProcessResponse{
		Type: "EDIT",
		Path: "modules/quote",
		Params: map[string]interface{}{"text": req.Text},
		Confidence: req.AudioMeta.Confidence,
	}
	writeJSON(w, http.StatusOK, response)
}

func main() {
	http.HandleFunc("/v1/health", healthHandler)
	http.HandleFunc("/v1/process", processHandler)

	fmt.Println("Server is running on 127.0.0.1:8080")
	if err := http.ListenAndServe("127.0.0.1:8080", nil); err != nil {
		fmt.Println("Failed to start server:", err)
	}
}