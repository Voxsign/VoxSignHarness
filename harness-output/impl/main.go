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
	addr       = flag.String("addr", "127.0.0.1:8950", "HTTP network address")
	dataDir    = flag.String("data-dir", "./data", "Data directory")
	upstream   = flag.String("upstream", "http://127.0.0.1:8941", "Upstream harness address")
	sessions   = make(map[string][]string)
	sessionsMu sync.Mutex
)

type HealthResponse struct {
	Ok       bool   `json:"ok"`
	Service  string `json:"service"`
	Upstream string `json:"upstream"`
	Sessions int    `json:"sessions"`
}

type ParseRequest struct {
	Text string `json:"text"`
}

type ParseResponse struct {
	Clean        string   `json:"clean"`
	Actions      []Action `json:"actions"`
	NoiseRemoved []string `json:"noise_removed"`
}

type Action struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

type DecomposeRequest struct {
	Clean string `json:"clean"`
}

type DecomposeResponse struct {
	Tasks []Task `json:"tasks"`
}

type Task struct {
	Seq    int    `json:"seq"`
	Action string `json:"action"`
	Target string `json:"target"`
}

type ResolveRequest struct {
	Text           string `json:"text"`
	ConversationID string `json:"conversation_id"`
}

type ResolveResponse struct {
	Resolved       bool   `json:"resolved"`
	Target         string `json:"target"`
	PendingResolve bool   `json:"pending_resolve"`
}

type RunRequest struct {
	Text           string `json:"text"`
	ConversationID string `json:"conversation_id"`
}

type RunResponse struct {
	TaskID    string `json:"task_id"`
	Submitted bool   `json:"submitted"`
	Summary   struct {
		Total int `json:"total"`
	} `json:"summary"`
}

func main() {
	flag.Parse()

	http.HandleFunc("/v1/voice/health", healthHandler)
	http.HandleFunc("/v1/voice/parse", parseHandler)
	http.HandleFunc("/v1/voice/decompose", decomposeHandler)
	http.HandleFunc("/v1/voice/resolve", resolveHandler)
	http.HandleFunc("/v1/voice/run", runHandler)
	http.HandleFunc("/v1/tasks", tasksHandler)
	http.HandleFunc("/v1/tasks/", taskHandler)
	http.HandleFunc("/v1/voice/tasks/", voiceTasksHandler)

	fmt.Printf("Listening on %s...\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Ok:       true,
		Service:  "vhs-voice",
		Upstream: *upstream,
		Sessions: len(sessions),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func parseHandler(w http.ResponseWriter, r *http.Request) {
	var req ParseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Dummy parse logic
	cleanText := strings.ReplaceAll(req.Text, "那个", "")
	action := "执行"
	target := "服务"
	noiseRemoved := []string{"那个"}

	resp := ParseResponse{
		Clean: cleanText,
		Actions: []Action{
			{Action: action, Target: target},
		},
		NoiseRemoved: noiseRemoved,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func decomposeHandler(w http.ResponseWriter, r *http.Request) {
	var req DecomposeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Dummy decomposition logic
	tasks := []Task{
		{Seq: 1, Action: "拉取", Target: "GitHub 仓库"},
		{Seq: 2, Action: "编译", Target: "服务"},
	}

	resp := DecomposeResponse{Tasks: tasks}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func resolveHandler(w http.ResponseWriter, r *http.Request) {
	var req ResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	history, exists := sessions[req.ConversationID]
	if !exists || len(history) == 0 {
		if strings.Contains(req.Text, "那个") || strings.Contains(req.Text, "这个") || strings.Contains(req.Text, "它") || strings.Contains(req.Text, "帮我") {
			resp := ResolveResponse{Resolved: false, Target: "unresolved", PendingResolve: true}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
	}

	// Dummy resolve logic
	resp := ResolveResponse{Resolved: true, Target: "服务", PendingResolve: false}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func runHandler(w http.ResponseWriter, r *http.Request) {
	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Dummy run logic
	resp := RunResponse{
		TaskID:    "12345",
		Submitted: true,
		Summary: struct {
			Total int `json:"total"`
		}{Total: 2},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func tasksHandler(w http.ResponseWriter, r *http.Request) {
	// Dummy tasks handler
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Tasks endpoint"))
}

func taskHandler(w http.ResponseWriter, r *http.Request) {
	// Dummy task handler
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Task endpoint"))
}

func voiceTasksHandler(w http.ResponseWriter, r *http.Request) {
	// Dummy voice tasks handler
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Voice tasks endpoint"))
}