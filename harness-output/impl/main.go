package main

import (
	"encoding/json"
	"flag"
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

	http.ListenAndServe(*addr, nil)
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
	json.NewDecoder(r.Body).Decode(&req)

	clean, actions, noiseRemoved := parseText(req.Text)

	response := map[string]interface{}{
		"clean":        clean,
		"actions":      actions,
		"noise_removed": noiseRemoved,
	}
	json.NewEncoder(w).Encode(response)
}

func parseText(text string) (string, []map[string]string, []string) {
	noiseWords := []string{"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后", "现在", "开始", "准备", "假设", "其实", "比如", "我觉得", "你知道", "大概", "应该", "可以"}
	actionWords := []string{"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交", "报告", "检查", "对比", "分析", "部署", "安装", "更新"}
	objectWords := []string{"GitHub 仓库", "服务", "需求", "产物", "测试"}

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
			action := map[string]string{"action": word, "target": ""}
			for _, objWord := range objectWords {
				if strings.Contains(text, objWord) {
					action["target"] = objWord
					break
				}
			}
			actions = append(actions, action)
		}
	}

	return strings.Join(cleanWords, " "), actions, noiseRemoved
}

func handleDecompose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Clean string `json:"clean"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	tasks := decomposeTasks(req.Clean)

	response := map[string]interface{}{
		"tasks": tasks,
	}
	json.NewEncoder(w).Encode(response)
}

func decomposeTasks(clean string) []map[string]interface{} {
	// Simple decomposition logic for demonstration
	actions := strings.Split(clean, "，")
	var tasks []map[string]interface{}
	for i, action := range actions {
		tasks = append(tasks, map[string]interface{}{
			"seq":    i + 1,
			"action": action,
			"target": "",
		})
	}
	return tasks
}

func handleResolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		Target string `json:"target"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Simple resolve logic for demonstration
	if req.Target == "" {
		req.Target = "默认对象"
	}

	response := map[string]interface{}{
		"action": req.Action,
		"target": req.Target,
	}
	json.NewEncoder(w).Encode(response)
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tasks []map[string]interface{} `json:"tasks"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Simulate task execution
	for _, task := range req.Tasks {
		task["status"] = "completed"
	}

	response := map[string]interface{}{
		"results": req.Tasks,
	}
	json.NewEncoder(w).Encode(response)
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimPrefix(r.URL.Path, "/v1/voice/tasks/")
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
	json.NewEncoder(w).Encode(response)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}