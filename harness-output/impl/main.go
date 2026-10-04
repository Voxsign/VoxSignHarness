package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
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

	http.HandleFunc("/v1/voice/health", handleHealth)
	http.HandleFunc("/v1/voice/parse", handleParse)
	http.HandleFunc("/v1/voice/decompose", handleDecompose)
	http.HandleFunc("/v1/voice/resolve", handleResolve)
	http.HandleFunc("/v1/voice/run", handleRun)
	http.HandleFunc("/v1/voice/tasks/", handleTasks)

	fmt.Printf("Listening on %s...\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
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

func handleParse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	noiseWords := []string{"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后", "现在", "开始", "准备", "假设", "其实", "比如", "我觉得", "你知道", "大概", "应该", "可以"}
	actions := []string{"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交", "报告", "检查", "对比", "分析", "部署", "安装", "更新"}
	targets := []string{"GitHub 仓库", "服务", "需求", "产物", "测试"}

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

	for _, tgt := range targets {
		if strings.Contains(cleanText, tgt) {
			target = tgt
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

func handleDecompose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Clean string `json:"clean"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	tasks := []map[string]interface{}{
		{"seq": 1, "action": "拉取", "target": "GitHub 仓库 smithpeter/voicesi"},
		// Add more tasks based on the clean text
	}

	response := map[string]interface{}{
		"tasks": tasks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleResolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Simulate resolving domain/object
	resolvedTarget := req.Target + " resolved"

	response := map[string]interface{}{
		"action": req.Action,
		"target": resolvedTarget,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tasks []struct {
			Action string `json:"action"`
			Target string `json:"target"`
		} `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Simulate running tasks
	results := []map[string]interface{}{}
	for _, task := range req.Tasks {
		results = append(results, map[string]interface{}{
			"action": task.Action,
			"target": task.Target,
			"status": "completed",
		})
	}

	response := map[string]interface{}{
		"results": results,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimPrefix(r.URL.Path, "/v1/voice/tasks/")
	if conversationID == "" {
		http.Error(w, "Conversation ID required", http.StatusBadRequest)
		return
	}

	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	sessionFile := filepath.Join(*dataDir, "voice_sessions", conversationID+".jsonl")
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		http.Error(w, "Conversation not found", http.StatusNotFound)
		return
	}

	data, err := ioutil.ReadFile(sessionFile)
	if err != nil {
		http.Error(w, "Error reading session file", http.StatusInternalServerError)
		return
	}

	lines := strings.Split(string(data), "\n")
	count := len(lines) - 1 // Last line is empty due to trailing newline

	response := map[string]interface{}{
		"count": count,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}