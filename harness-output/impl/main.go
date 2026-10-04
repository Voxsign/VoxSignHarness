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
	sessions   = make(map[string][]map[string]interface{})
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

	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
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

	clean, actions, noiseRemoved := parseText(req.Text)

	response := map[string]interface{}{
		"clean":         clean,
		"actions":       actions,
		"noise_removed": noiseRemoved,
	}
	json.NewEncoder(w).Encode(response)
}

func parseText(text string) (string, []map[string]string, []string) {
	fillers := []string{"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后", "现在", "开始", "准备", "假设", "其实", "比如", "我觉得", "你知道", "大概", "应该", "可以"}
	actions := []string{"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交", "报告", "检查", "对比", "分析", "部署", "安装", "更新"}

	words := strings.Fields(text)
	var cleanWords []string
	var noiseRemoved []string
	var detectedActions []map[string]string

	for _, word := range words {
		if contains(fillers, word) {
			noiseRemoved = append(noiseRemoved, word)
		} else {
			cleanWords = append(cleanWords, word)
		}
	}

	for _, word := range cleanWords {
		if contains(actions, word) {
			detectedActions = append(detectedActions, map[string]string{"action": word, "target": ""})
		}
	}

	cleanText := strings.Join(cleanWords, " ")
	return cleanText, detectedActions, noiseRemoved
}

func contains(slice []string, item string) bool {
	for _, a := range slice {
		if a == item {
			return true
		}
	}
	return false
}

func handleVoiceDecompose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Clean string `json:"clean"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	tasks := decomposeText(req.Clean)

	response := map[string]interface{}{
		"tasks": tasks,
	}
	json.NewEncoder(w).Encode(response)
}

func decomposeText(clean string) []map[string]interface{} {
	// Placeholder logic for decomposing text into tasks
	return []map[string]interface{}{
		{"seq": 1, "action": "拉取", "target": "GitHub 仓库 example/repo"},
	}
}

func handleVoiceResolve(w http.ResponseWriter, r *http.Request) {
	// Placeholder for resolve logic
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func handleVoiceRun(w http.ResponseWriter, r *http.Request) {
	// Placeholder for run logic
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func handleVoiceTasks(w http.ResponseWriter, r *http.Request) {
	// Placeholder for tasks logic
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	// Placeholder for tasks logic
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func handleTaskByID(w http.ResponseWriter, r *http.Request) {
	// Placeholder for task by ID logic
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func saveSession(conversationID string, data map[string]interface{}) error {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	if _, exists := sessions[conversationID]; !exists {
		sessions[conversationID] = []map[string]interface{}{}
	}
	sessions[conversationID] = append(sessions[conversationID], data)

	sessionDir := filepath.Join(*dataDir, "voice_sessions")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return err
	}

	sessionFile := filepath.Join(sessionDir, fmt.Sprintf("%s.jsonl", conversationID))
	file, err := os.OpenFile(sessionFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	entry, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if _, err := file.Write(append(entry, '\n')); err != nil {
		return err
	}

	return nil
}