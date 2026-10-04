package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

var (
	addr        = flag.String("addr", "127.0.0.1:8950", "HTTP network address")
	dataDir     = flag.String("data-dir", "./data", "Data directory")
	upstreamURL = flag.String("upstream", "http://127.0.0.1:8941", "Upstream harness URL")
)

type Session struct {
	ConversationID string `json:"conversation_id"`
	Text           string `json:"text"`
}

type Task struct {
	Seq    int    `json:"seq"`
	Action string `json:"action"`
	Target string `json:"target"`
}

type ParseResponse struct {
	Clean        string   `json:"clean"`
	Actions      []Task   `json:"actions"`
	NoiseRemoved []string `json:"noise_removed"`
}

type DecomposeResponse struct {
	Tasks []Task `json:"tasks"`
}

type RunResponse struct {
	Tasks []struct {
		TaskID string `json:"task_id"`
	} `json:"tasks"`
	Summary struct {
		Total int `json:"total"`
	} `json:"summary"`
}

var (
	sessions     = make(map[string][]Session)
	sessionsLock sync.Mutex
)

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
		fmt.Printf("Error starting server: %v\n", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()

	response := map[string]interface{}{
		"ok":       true,
		"service":  "vhs-voice",
		"upstream": *upstreamURL,
		"sessions": len(sessions),
	}
	json.NewEncoder(w).Encode(response)
}

func parseHandler(w http.ResponseWriter, r *http.Request) {
	var input map[string]string
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	text := input["text"]
	clean, actions, noiseRemoved := parseText(text)

	response := ParseResponse{
		Clean:        clean,
		Actions:      actions,
		NoiseRemoved: noiseRemoved,
	}
	json.NewEncoder(w).Encode(response)
}

func parseText(text string) (string, []Task, []string) {
	noiseWords := []string{"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后", "现在", "开始", "准备", "假设", "其实", "比如", "我觉得", "你知道", "大概", "应该", "可以"}
	actions := []string{"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交", "报告", "检查", "对比", "分析", "部署", "安装", "更新"}
	targets := []string{"GitHub 仓库", "服务", "需求", "产物", "测试"}

	words := strings.Fields(text)
	var cleanWords []string
	var noiseRemoved []string
	var tasks []Task

	for _, word := range words {
		if contains(noiseWords, word) {
			noiseRemoved = append(noiseRemoved, word)
		} else {
			cleanWords = append(cleanWords, word)
		}
	}

	cleanText := strings.Join(cleanWords, " ")

	for _, action := range actions {
		if strings.HasPrefix(cleanText, action) {
			target := ""
			for _, t := range targets {
				if strings.Contains(cleanText, t) {
					target = t
					break
				}
			}
			tasks = append(tasks, Task{Action: action, Target: target})
			break
		}
	}

	return cleanText, tasks, noiseRemoved
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func decomposeHandler(w http.ResponseWriter, r *http.Request) {
	var input map[string]string
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	clean := input["clean"]
	tasks := decomposeText(clean)

	response := DecomposeResponse{
		Tasks: tasks,
	}
	json.NewEncoder(w).Encode(response)
}

func decomposeText(clean string) []Task {
	// Placeholder logic for decomposing text into tasks
	// This should be replaced with actual logic
	return []Task{
		{Seq: 1, Action: "拉取", Target: "GitHub 仓库"},
		{Seq: 2, Action: "编译", Target: "服务"},
	}
}

func resolveHandler(w http.ResponseWriter, r *http.Request) {
	var input map[string]string
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	conversationID := input["conversation_id"]
	text := input["text"]

	sessionsLock.Lock()
	history, exists := sessions[conversationID]
	sessionsLock.Unlock()

	if !exists || len(history) == 0 {
		if strings.Contains(text, "那个") || strings.Contains(text, "这个") || strings.Contains(text, "它") || strings.Contains(text, "帮我") {
			response := map[string]interface{}{
				"resolved":        false,
				"target":          "unresolved",
				"pending_resolve": true,
			}
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// Placeholder logic for resolving target
	// This should be replaced with actual logic
	resolvedTarget := "resolved_target"

	response := map[string]interface{}{
		"resolved": true,
		"target":   resolvedTarget,
	}
	json.NewEncoder(w).Encode(response)
}

func runHandler(w http.ResponseWriter, r *http.Request) {
	var input map[string]string
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	text := input["text"]
	conversationID := input["conversation_id"]

	tasks := decomposeText(text)
	var taskIDs []string

	for _, task := range tasks {
		taskText := fmt.Sprintf("%s %s", task.Action, task.Target)
		taskID, err := submitTask(taskText, conversationID)
		if err != nil {
			http.Error(w, "Failed to submit task", http.StatusInternalServerError)
			return
		}
		taskIDs = append(taskIDs, taskID)
	}

	response := RunResponse{
		Tasks: make([]struct{ TaskID string }, len(taskIDs)),
		Summary: struct {
			Total int `json:"total"`
		}{Total: len(taskIDs)},
	}

	for i, taskID := range taskIDs {
		response.Tasks[i].TaskID = taskID
	}

	json.NewEncoder(w).Encode(response)
}

func submitTask(text, conversationID string) (string, error) {
	// Placeholder logic for submitting a task to the upstream harness
	// This should be replaced with actual logic
	return "task_id", nil
}

func tasksHandler(w http.ResponseWriter, r *http.Request) {
	// Placeholder logic for handling tasks
	// This should be replaced with actual logic
	w.WriteHeader(http.StatusOK)
}

func taskHandler(w http.ResponseWriter, r *http.Request) {
	// Placeholder logic for handling a specific task
	// This should be replaced with actual logic
	w.WriteHeader(http.StatusOK)
}

func voiceTasksHandler(w http.ResponseWriter, r *http.Request) {
	// Placeholder logic for handling voice tasks
	// This should be replaced with actual logic
	w.WriteHeader(http.StatusOK)
}