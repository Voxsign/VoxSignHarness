package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
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

	http.HandleFunc("/v1/tasks", handleTasksProxy)
	http.HandleFunc("/v1/tasks/", handleTaskProxy)

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
		"clean":        clean,
		"actions":      actions,
		"noise_removed": noiseRemoved,
	}
	json.NewEncoder(w).Encode(response)
}

func parseText(text string) (string, []map[string]string, []string) {
	fillers := []string{"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后", "现在", "开始", "准备", "假设", "其实", "比如", "我觉得", "你知道", "大概", "应该", "可以"}
	actions := []string{"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交", "报告", "检查", "对比", "分析", "部署", "安装", "更新"}
	targets := []string{"GitHub 仓库", "服务", "需求", "产物", "测试"}

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
			for _, t := range targets {
				if strings.Contains(text, t) {
					target = t
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
	// For simplicity, assume each sentence is a task
	sentences := strings.Split(clean, "。")
	var tasks []map[string]interface{}
	for i, sentence := range sentences {
		if sentence != "" {
			tasks = append(tasks, map[string]interface{}{
				"seq":    i + 1,
				"action": "执行",
				"target": sentence,
			})
		}
	}
	return tasks
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

	// Simulate resolution logic
	resolved := map[string]string{
		"action": req.Action,
		"target": req.Target + " (resolved)",
	}

	json.NewEncoder(w).Encode(resolved)
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tasks []map[string]interface{} `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Simulate task execution
	for _, task := range req.Tasks {
		fmt.Printf("Executing task: %v\n", task)
	}

	response := map[string]interface{}{
		"status": "success",
	}
	json.NewEncoder(w).Encode(response)
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimPrefix(r.URL.Path, "/v1/voice/tasks/")
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	if tasks, ok := sessions[conversationID]; ok {
		response := map[string]interface{}{
			"count": len(tasks),
			"tasks": tasks,
		}
		json.NewEncoder(w).Encode(response)
	} else {
		http.Error(w, "Conversation not found", http.StatusNotFound)
	}
}

func handleTasksProxy(w http.ResponseWriter, r *http.Request) {
	proxyRequest(w, r, *upstream+"/v1/tasks")
}

func handleTaskProxy(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")
	proxyRequest(w, r, *upstream+"/v1/tasks/"+taskID)
}

func proxyRequest(w http.ResponseWriter, r *http.Request, url string) {
	req, err := http.NewRequest(r.Method, url, r.Body)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}
	req.Header = r.Header

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to execute request", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	body, _ := ioutil.ReadAll(resp.Body)
	w.Write(body)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}