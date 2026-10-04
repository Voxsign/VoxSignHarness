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
	sessionsMu sync.Mutex
	sessions   = make(map[string][]string)
)

type HealthResponse struct {
	Ok       bool   `json:"ok"`
	Service  string `json:"service"`
	Upstream string `json:"upstream"`
	Sessions int    `json:"sessions"`
}

type ParseRequest struct {
	Text string `json:"text"`
}

type ParseResponse struct {
	Clean        string   `json:"clean"`
	Actions      []Action `json:"actions"`
	NoiseRemoved []string `json:"noise_removed"`
}

type Action struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

type DecomposeRequest struct {
	Clean string `json:"clean"`
}

type DecomposeResponse struct {
	Tasks []Task `json:"tasks"`
}

type Task struct {
	Seq    int    `json:"seq"`
	Action string `json:"action"`
	Target string `json:"target"`
}

type ResolveRequest struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

type ResolveResponse struct {
	ResolvedTarget string `json:"resolved_target"`
}

type RunRequest struct {
	Tasks []Task `json:"tasks"`
}

type RunResponse struct {
	Results []TaskResult `json:"results"`
}

type TaskResult struct {
	Seq    int    `json:"seq"`
	Status string `json:"status"`
}

func main() {
	flag.Parse()

	http.HandleFunc("/v1/voice/health", handleHealth)
	http.HandleFunc("/v1/voice/parse", handleParse)
	http.HandleFunc("/v1/voice/decompose", handleDecompose)
	http.HandleFunc("/v1/voice/resolve", handleResolve)
	http.HandleFunc("/v1/voice/run", handleRun)
	http.HandleFunc("/v1/voice/tasks/", handleTasks)

	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	resp := HealthResponse{
		Ok:       true,
		Service:  "vhs-voice",
		Upstream: *upstream,
		Sessions: len(sessions),
	}
	json.NewEncoder(w).Encode(resp)
}

func handleParse(w http.ResponseWriter, r *http.Request) {
	var req ParseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	noiseWords := []string{"就是", "那个", "对吧", "好不好", "好吧", "怎么样", "然后", "现在", "开始", "准备", "假设", "其实", "比如", "我觉得", "你知道", "大概", "应该", "可以"}
	actionWords := []string{"执行", "跑", "测试", "拉取", "编译", "启动", "实现", "提交", "报告", "检查", "对比", "分析", "部署", "安装", "更新"}
	targetWords := []string{"GitHub 仓库", "服务", "需求", "产物", "测试"}

	cleanText := req.Text
	noiseRemoved := []string{}
	for _, noise := range noiseWords {
		if strings.Contains(cleanText, noise) {
			cleanText = strings.ReplaceAll(cleanText, noise, "")
			noiseRemoved = append(noiseRemoved, noise)
		}
	}

	actions := []Action{}
	for _, action := range actionWords {
		if strings.Contains(cleanText, action) {
			target := ""
			for _, targetWord := range targetWords {
				if strings.Contains(cleanText, targetWord) {
					target = targetWord
					break
				}
			}
			actions = append(actions, Action{Action: action, Target: target})
			break
		}
	}

	resp := ParseResponse{
		Clean:        cleanText,
		Actions:      actions,
		NoiseRemoved: noiseRemoved,
	}
	json.NewEncoder(w).Encode(resp)
}

func handleDecompose(w http.ResponseWriter, r *http.Request) {
	var req DecomposeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// For simplicity, assume each action-target pair is a separate task
	tasks := []Task{}
	seq := 1
	for _, action := range []string{"拉取", "编译", "部署"} {
		if strings.Contains(req.Clean, action) {
			tasks = append(tasks, Task{Seq: seq, Action: action, Target: "GitHub 仓库"})
			seq++
		}
	}

	resp := DecomposeResponse{Tasks: tasks}
	json.NewEncoder(w).Encode(resp)
}

func handleResolve(w http.ResponseWriter, r *http.Request) {
	var req ResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Simulate resolving target
	resolvedTarget := req.Target + " (resolved)"
	resp := ResolveResponse{ResolvedTarget: resolvedTarget}
	json.NewEncoder(w).Encode(resp)
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	results := []TaskResult{}
	for _, task := range req.Tasks {
		// Simulate task execution
		results = append(results, TaskResult{Seq: task.Seq, Status: "completed"})
	}

	resp := RunResponse{Results: results}
	json.NewEncoder(w).Encode(resp)
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimPrefix(r.URL.Path, "/v1/voice/tasks/")
	if conversationID == "" {
		http.Error(w, "Conversation ID required", http.StatusBadRequest)
		return
	}

	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	tasks, ok := sessions[conversationID]
	if !ok {
		http.Error(w, "Conversation not found", http.StatusNotFound)
		return
	}

	resp := map[string]interface{}{
		"count": len(tasks),
		"tasks": tasks,
	}
	json.NewEncoder(w).Encode(resp)
}

func saveSession(conversationID string, data string) error {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	if _, ok := sessions[conversationID]; !ok {
		sessions[conversationID] = []string{}
	}
	sessions[conversationID] = append(sessions[conversationID], data)

	sessionDir := filepath.Join(*dataDir, "voice_sessions")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return err
	}

	sessionFile := filepath.Join(sessionDir, conversationID+".jsonl")
	f, err := os.OpenFile(sessionFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(data + "\n"); err != nil {
		return err
	}

	return nil
}