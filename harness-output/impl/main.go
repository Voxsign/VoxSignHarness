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
	"time"
)

var (
	addr        = flag.String("addr", "127.0.0.1:8950", "HTTP network address")
	dataDir     = flag.String("data-dir", "./data", "Data directory")
	upstreamURL = flag.String("upstream", "http://127.0.0.1:8941", "Upstream harness URL")
)

type Session struct {
	ConversationID string `json:"conversation_id"`
}

type Task struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Target string `json:"target"`
}

type Feedback struct {
	Timestamp time.Time `json:"timestamp"`
	Feedback  string    `json:"feedback"`
}

var (
	sessions     = make(map[string]*Session)
	sessionsLock sync.Mutex
)

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

	http.ListenAndServe(*addr, nil)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()

	response := map[string]interface{}{
		"ok":       true,
		"service":  "vhs-voice",
		"upstream": *upstreamURL,
		"sessions": len(sessions),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleParse(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Text string `json:"text"`
	}
	json.NewDecoder(r.Body).Decode(&input)

	noiseWords := []string{"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后", "现在", "开始", "准备", "假设", "其实", "比如", "我觉得", "你知道", "大概", "应该", "可以"}
	actionWords := []string{"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交", "报告", "检查", "对比", "分析", "部署", "安装", "更新"}
	targetWords := []string{"GitHub 仓库", "服务", "需求", "产物", "测试"}

	cleanText := input.Text
	noiseRemoved := []string{}
	actions := []map[string]string{}

	for _, word := range noiseWords {
		if strings.Contains(cleanText, word) {
			cleanText = strings.ReplaceAll(cleanText, word, "")
			noiseRemoved = append(noiseRemoved, word)
		}
	}

	for _, action := range actionWords {
		if strings.Contains(cleanText, action) {
			target := ""
			for _, targetWord := range targetWords {
				if strings.Contains(cleanText, targetWord) {
					target = targetWord
					break
				}
			}
			actions = append(actions, map[string]string{"action": action, "target": target})
			break
		}
	}

	response := map[string]interface{}{
		"clean":         cleanText,
		"actions":       actions,
		"noise_removed": noiseRemoved,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleDecompose(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Clean string `json:"clean"`
	}
	json.NewDecoder(r.Body).Decode(&input)

	tasks := []map[string]interface{}{}
	actions := []string{"拉取", "编译", "部署"}
	for i, action := range actions {
		tasks = append(tasks, map[string]interface{}{
			"seq":    i + 1,
			"action": action,
			"target": "GitHub 仓库 example/repo",
		})
	}

	response := map[string]interface{}{
		"tasks": tasks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleResolve(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Actions []struct {
			Action string `json:"action"`
			Target string `json:"target"`
		} `json:"actions"`
	}
	json.NewDecoder(r.Body).Decode(&input)

	resolved := []map[string]string{}
	for _, action := range input.Actions {
		if action.Target == "" {
			resolved = append(resolved, map[string]string{"status": "pending_resolve"})
		} else {
			resolved = append(resolved, map[string]string{"status": "resolved", "action": action.Action, "target": action.Target})
		}
	}

	response := map[string]interface{}{
		"resolved": resolved,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Tasks []struct {
			Action string `json:"action"`
			Target string `json:"target"`
		} `json:"tasks"`
	}
	json.NewDecoder(r.Body).Decode(&input)

	results := []map[string]string{}
	for _, task := range input.Tasks {
		results = append(results, map[string]string{"status": "completed", "action": task.Action, "target": task.Target})
	}

	response := map[string]interface{}{
		"results": results,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleVoiceTasks(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimPrefix(r.URL.Path, "/v1/voice/tasks/")
	filePath := filepath.Join(*dataDir, "voice_sessions", conversationID+".jsonl")

	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	var sessions []Session
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var session Session
		decoder.Decode(&session)
		sessions = append(sessions, session)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var task Task
		json.NewDecoder(r.Body).Decode(&task)

		task.ID = fmt.Sprintf("%d", time.Now().UnixNano())
		filePath := filepath.Join(*dataDir, "tasks.jsonl")
		file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			http.Error(w, "Unable to save task", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		encoder := json.NewEncoder(file)
		encoder.Encode(task)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(task)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleTaskByID(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")
	filePath := filepath.Join(*dataDir, "tasks.jsonl")

	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	for decoder.More() {
		var task Task
		decoder.Decode(&task)
		if task.ID == taskID {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(task)
			return
		}
	}

	http.Error(w, "Task not found", http.StatusNotFound)
}