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
	addr       string
	dataDir    string
	upstream   string
	sessions   = make(map[string][]string)
	sessionsMu sync.Mutex
)

func init() {
	flag.StringVar(&addr, "addr", getEnv("VHS_VOICE_ADDR", "127.0.0.1:8950"), "address to listen on")
	flag.StringVar(&dataDir, "data-dir", getEnv("VHS_DATA_DIR", "./data"), "directory to store data")
	flag.StringVar(&upstream, "upstream", getEnv("VHS_UPSTREAM", "http://127.0.0.1:8941"), "upstream service address")
}

func main() {
	flag.Parse()

	http.HandleFunc("/v1/voice/health", handleHealth)
	http.HandleFunc("/v1/voice/parse", handleParse)
	http.HandleFunc("/v1/voice/decompose", handleDecompose)
	http.HandleFunc("/v1/voice/resolve", handleResolve)
	http.HandleFunc("/v1/voice/run", handleRun)
	http.HandleFunc("/v1/voice/tasks/", handleVoiceTasks)
	http.HandleFunc("/v1/tasks", handleTasks)
	http.HandleFunc("/v1/tasks/", handleTaskByID)

	fmt.Printf("Listening on %s...\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	response := map[string]interface{}{
		"ok":       true,
		"service":  "vhs-voice",
		"upstream": upstream,
		"sessions": len(sessions),
	}
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

	clean, actions, noiseRemoved := parseText(req.Text)

	response := map[string]interface{}{
		"clean":         clean,
		"actions":       actions,
		"noise_removed": noiseRemoved,
	}
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

	tasks := decomposeText(req.Clean)

	response := map[string]interface{}{
		"tasks": tasks,
	}
	json.NewEncoder(w).Encode(response)
}

func handleResolve(w http.ResponseWriter, r *http.Request) {
	// Placeholder for resolve logic
	w.WriteHeader(http.StatusNotImplemented)
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	// Placeholder for run logic
	w.WriteHeader(http.StatusNotImplemented)
}

func handleVoiceTasks(w http.ResponseWriter, r *http.Request) {
	// Placeholder for voice tasks logic
	w.WriteHeader(http.StatusNotImplemented)
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	// Placeholder for tasks logic
	w.WriteHeader(http.StatusNotImplemented)
}

func handleTaskByID(w http.ResponseWriter, r *http.Request) {
	// Placeholder for task by ID logic
	w.WriteHeader(http.StatusNotImplemented)
}

func parseText(text string) (string, []map[string]string, []string) {
	noiseWords := []string{"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后", "现在", "开始", "准备", "假设", "其实", "比如", "我觉得", "你知道", "大概", "应该", "可以"}
	actionWords := []string{"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交", "报告", "检查", "对比", "分析", "部署", "安装", "更新"}

	words := strings.Fields(text)
	var cleanWords []string
	var noiseRemoved []string
	var actions []map[string]string

	for _, word := range words {
		if contains(noiseWords, word) {
			noiseRemoved = append(noiseRemoved, word)
		} else {
			cleanWords = append(cleanWords, word)
		}
	}

	for _, word := range cleanWords {
		if contains(actionWords, word) {
			actions = append(actions, map[string]string{"action": word, "target": ""})
		}
	}

	clean := strings.Join(cleanWords, " ")
	return clean, actions, noiseRemoved
}

func decomposeText(clean string) []map[string]interface{} {
	// Placeholder for decompose logic
	return []map[string]interface{}{}
}

func contains(slice []string, item string) bool {
	for _, a := range slice {
		if a == item {
			return true
		}
	}
	return false
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}