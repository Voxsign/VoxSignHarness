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
	addr       = flag.String("addr", "127.0.0.1:8950", "service address")
	dataDir    = flag.String("data-dir", "./data", "data directory")
	upstream   = os.Getenv("VHS_UPSTREAM")
	sessions   = make(map[string]int)
	sessionsMu sync.Mutex
)

func main() {
	flag.Parse()
	if upstream == "" {
		upstream = "http://127.0.0.1:8941"
	}

	http.HandleFunc("/v1/voice/health", handleHealth)
	http.HandleFunc("/v1/voice/parse", handleParse)
	http.HandleFunc("/v1/voice/decompose", handleDecompose)
	http.HandleFunc("/v1/tasks", handleTasks)
	http.HandleFunc("/v1/voice", handleVoice)

	fmt.Printf("Listening on %s\n", *addr)
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

	tasks := decomposeTasks(req.Clean)
	response := map[string]interface{}{
		"tasks": tasks,
	}
	json.NewEncoder(w).Encode(response)
}

func decomposeTasks(clean string) []map[string]interface{} {
	// Placeholder for task decomposition logic
	return []map[string]interface{}{
		{"seq": 1, "action": "拉取", "target": "GitHub 仓库 example/repo"},
	}
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	// Placeholder for tasks handling logic
	w.WriteHeader(http.StatusNotImplemented)
}

func handleVoice(w http.ResponseWriter, r *http.Request) {
	// Placeholder for voice handling logic
	w.WriteHeader(http.StatusNotImplemented)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}