package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	addr        = flag.String("addr", "127.0.0.1:8950", "HTTP network address")
	dataDir     = flag.String("data-dir", "./data", "Data directory")
	upstreamURL = os.Getenv("VHS_UPSTREAM")
)

type Task struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Details string `json:"details"`
}

type Session struct {
	ConversationID string `json:"conversation_id"`
	Tasks          []Task `json:"tasks"`
}

var (
	sessions   = make(map[string]*Session)
	sessionsMu sync.Mutex
)

func main() {
	flag.Parse()
	if upstreamURL == "" {
		upstreamURL = "http://127.0.0.1:8941"
	}

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
		fmt.Printf("Error starting server: %v\n", err)
	}
}

func handleVoiceHealth(w http.ResponseWriter, r *http.Request) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	response := map[string]interface{}{
		"ok":       true,
		"service":  "vhs-voice",
		"upstream": upstreamURL,
		"sessions": len(sessions),
	}
	json.NewEncoder(w).Encode(response)
}

func handleVoiceParse(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	noiseWords := []string{"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后", "现在", "开始", "准备", "假设", "其实", "比如", "我觉得", "你知道", "大概", "应该", "可以"}
	actionWords := []string{"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交", "报告", "检查", "对比", "分析", "部署", "安装", "更新"}
	objectWords := []string{"GitHub 仓库", "服务", "需求", "产物", "测试"}

	cleanText := input.Text
	noiseRemoved := []string{}
	for _, word := range noiseWords {
		if strings.Contains(cleanText, word) {
			cleanText = strings.ReplaceAll(cleanText, word, "")
			noiseRemoved = append(noiseRemoved, word)
		}
	}

	actions := []map[string]string{}
	for _, action := range actionWords {
		if strings.Contains(cleanText, action) {
			target := ""
			for _, object := range objectWords {
				if strings.Contains(cleanText, object) {
					target = object
					break
				}
			}
			actions = append(actions, map[string]string{"action": action, "target": target})
		}
	}

	response := map[string]interface{}{
		"clean":         cleanText,
		"actions":       actions,
		"noise_removed": noiseRemoved,
	}
	json.NewEncoder(w).Encode(response)
}

func handleVoiceDecompose(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Clean string `json:"clean"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	tasks := []map[string]interface{}{
		{"seq": 1, "action": "拉取", "target": "GitHub 仓库"},
		{"seq": 2, "action": "编译", "target": "产物"},
	}

	response := map[string]interface{}{
		"tasks": tasks,
	}
	json.NewEncoder(w).Encode(response)
}

func handleVoiceResolve(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Clean string `json:"clean"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	resolvedTarget := "unresolved"
	if strings.Contains(input.Clean, "GitHub 仓库") {
		resolvedTarget = "GitHub 仓库"
	}

	response := map[string]interface{}{
		"resolved_target": resolvedTarget,
	}
	json.NewEncoder(w).Encode(response)
}

func handleVoiceRun(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Tasks []map[string]interface{} `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	taskIDs := []string{}
	for _, task := range input.Tasks {
		taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
		taskIDs = append(taskIDs, taskID)
	}

	response := map[string]interface{}{
		"task_ids": taskIDs,
	}
	json.NewEncoder(w).Encode(response)
}

func handleVoiceTasks(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimPrefix(r.URL.Path, "/v1/voice/tasks/")
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	session, exists := sessions[conversationID]
	if !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(session)
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var task Task
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
			http.Error(w, "Invalid input", http.StatusBadRequest)
			return
		}
		task.ID = fmt.Sprintf("task-%d", time.Now().UnixNano())
		task.Status = "pending"
		task.Details = "Task created"

		sessionsMu.Lock()
		defer sessionsMu.Unlock()
		session, exists := sessions[task.ID]
		if !exists {
			session = &Session{ConversationID: task.ID, Tasks: []Task{}}
			sessions[task.ID] = session
		}
		session.Tasks = append(session.Tasks, task)

		json.NewEncoder(w).Encode(task)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleTaskByID(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	for _, session := range sessions {
		for _, task := range session.Tasks {
			if task.ID == taskID {
				json.NewEncoder(w).Encode(task)
				return
			}
		}
	}

	http.Error(w, "Task not found", http.StatusNotFound)
}