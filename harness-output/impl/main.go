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
	sessions   = make(map[string][]map[string]interface{})
	sessionsMu sync.Mutex
)

func init() {
	flag.StringVar(&addr, "addr", "127.0.0.1:8950", "service address")
	flag.StringVar(&dataDir, "data-dir", "./data", "data directory")
	flag.StringVar(&upstream, "upstream", "http://127.0.0.1:8941", "upstream service address")
}

func main() {
	flag.Parse()

	http.HandleFunc("/v1/voice/health", handleHealth)
	http.HandleFunc("/v1/voice/parse", handleParse)
	http.HandleFunc("/v1/voice/decompose", handleDecompose)
	http.HandleFunc("/v1/voice/resolve", handleResolve)
	http.HandleFunc("/v1/voice/feedback", handleFeedback)
	http.HandleFunc("/v1/tasks", handleTasks)
	http.HandleFunc("/v1/voice", handleVoice)

	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Println("Failed to start server:", err)
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

func parseText(text string) (string, []map[string]string, []string) {
	fillers := []string{"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后", "现在", "开始", "准备", "假设", "其实", "比如", "我觉得", "你知道", "大概", "应该", "可以"}
	actions := []string{"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交", "报告", "检查", "对比", "分析", "部署", "安装", "更新"}
	objects := []string{"GitHub 仓库", "服务", "需求", "产物", "测试"}

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

	for _, action := range actions {
		if strings.HasPrefix(text, action) {
			target := ""
			for _, obj := range objects {
				if strings.Contains(text, obj) {
					target = obj
					break
				}
			}
			detectedActions = append(detectedActions, map[string]string{"action": action, "target": target})
			break
		}
	}

	return strings.Join(cleanWords, " "), detectedActions, noiseRemoved
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

func decomposeText(clean string) []map[string]interface{} {
	// Placeholder for actual decomposition logic
	return []map[string]interface{}{
		{"seq": 1, "action": "拉取", "target": "GitHub 仓库 example/repo"},
	}
}

func handleResolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tasks []map[string]interface{} `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	resolvedTasks := resolveTasks(req.Tasks)

	response := map[string]interface{}{
		"resolved_tasks": resolvedTasks,
	}
	json.NewEncoder(w).Encode(response)
}

func resolveTasks(tasks []map[string]interface{}) []map[string]interface{} {
	// Placeholder for actual resolution logic
	return tasks
}

func handleFeedback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Feedback string `json:"feedback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	appendFeedback(req.Feedback)

	response := map[string]interface{}{
		"status": "feedback recorded",
	}
	json.NewEncoder(w).Encode(response)
}

func appendFeedback(feedback string) {
	filePath := filepath.Join(dataDir, "feedback.jsonl")
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Failed to open feedback file:", err)
		return
	}
	defer f.Close()

	entry := map[string]string{"feedback": feedback}
	data, _ := json.Marshal(entry)
	f.Write(data)
	f.Write([]byte("\n"))
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	// Placeholder for tasks handling logic
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func handleVoice(w http.ResponseWriter, r *http.Request) {
	// Placeholder for voice handling logic
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}