package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

func main() {
	flag.Parse()

	http.HandleFunc("/v1/voice/health", handleVoiceHealth)
	http.HandleFunc("/v1/voice/parse", handleVoiceParse)
	http.HandleFunc("/v1/voice/decompose", handleVoiceDecompose)
	http.HandleFunc("/v1/voice/resolve", handleVoiceResolve)
	http.HandleFunc("/v1/voice/run", handleVoiceRun)
	http.HandleFunc("/v1/voice/tasks/", handleVoiceTasks)

	http.HandleFunc("/v1/tasks", handleTasks)
	http.HandleFunc("/v1/tasks/", handleTaskByID)

	fmt.Printf("Listening on %s...\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Println("Failed to start server:", err)
	}
}

func handleVoiceHealth(w http.ResponseWriter, r *http.Request) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	response := map[string]interface{}{
		"ok":       true,
		"service":  "vhs-voice",
		"upstream": *upstream,
		"sessions": len(sessions),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleVoiceParse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	noiseWords := []string{"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后", "现在", "开始", "准备", "假设", "其实", "比如", "我觉得", "你知道", "大概", "应该", "可以"}
	actions := []string{"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交", "报告", "检查", "对比", "分析", "部署", "安装", "更新"}

	cleanText := req.Text
	noiseRemoved := []string{}
	for _, word := range noiseWords {
		if strings.Contains(cleanText, word) {
			cleanText = strings.ReplaceAll(cleanText, word, "")
			noiseRemoved = append(noiseRemoved, word)
		}
	}

	action := ""
	target := ""
	for _, act := range actions {
		if strings.Contains(cleanText, act) {
			action = act
			break
		}
	}

	response := map[string]interface{}{
		"clean":         cleanText,
		"actions":       []map[string]string{{"action": action, "target": target}},
		"noise_removed": noiseRemoved,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleVoiceDecompose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Clean string `json:"clean"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	tasks := []map[string]interface{}{
		{"seq": 1, "action": "拉取", "target": "GitHub 仓库 smithpeter/voicesi"},
	}

	response := map[string]interface{}{
		"tasks": tasks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleVoiceResolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Placeholder logic for resolving domain/object
	resolvedTarget := req.Target
	if resolvedTarget == "" {
		resolvedTarget = "default target"
	}

	response := map[string]interface{}{
		"resolved_target": resolvedTarget,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleVoiceRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tasks []map[string]interface{} `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Simulate task execution
	results := []map[string]interface{}{}
	for _, task := range req.Tasks {
		results = append(results, map[string]interface{}{
			"task":   task,
			"status": "completed",
		})
	}

	response := map[string]interface{}{
		"results": results,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleVoiceTasks(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimPrefix(r.URL.Path, "/v1/voice/tasks/")
	if conversationID == "" {
		http.Error(w, "Conversation ID required", http.StatusBadRequest)
		return
	}

	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	tasks, ok := sessions[conversationID]
	if !ok {
		http.Error(w, "Conversation not found", http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"count": len(tasks),
		"tasks": tasks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Simulate task creation
	response := map[string]interface{}{
		"task_id": "12345",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleTaskByID(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")
	if taskID == "" {
		http.Error(w, "Task ID required", http.StatusBadRequest)
		return
	}

	// Simulate task retrieval
	response := map[string]interface{}{
		"task_id": taskID,
		"status":  "completed",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func saveSession(conversationID string, data string) error {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	if _, ok := sessions[conversationID]; !ok {
		sessions[conversationID] = []string{}
	}
	sessions[conversationID] = append(sessions[conversationID], data)

	sessionDir := filepath.Join(*dataDir, "voice_sessions")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return err
	}

	sessionFile := filepath.Join(sessionDir, conversationID+".jsonl")
	f, err := os.OpenFile(sessionFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(data + "\n"); err != nil {
		return err
	}

	return nil
}