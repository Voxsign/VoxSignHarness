// Package trajectory 提供 append-only JSONL 轨迹日志（架构 §10）。
// 这是任何架构方案下都需要的硬底座：无条件保留 ASR 原文（先于一切处理落盘）、
// 逐轮记录模型/动作/回执；按 request_id 可完整重放一次指令的「原文→理解→动作→结果」。
package trajectory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"voicesign-harness/contract"
)

// 轨迹条目类型（kind）。
const (
	KindInputRaw    = "input_raw"     // ASR 原文（原始证据，必须先于任何处理写入）
	KindInputClean  = "input_clean"   // 清洗后
	KindInputCorrec = "input_correct" // 纠错后
	KindIntent      = "intent"        // 意图 JSON
	KindStart       = "start"         // 一次请求开始（含 model/prompt_hash）
	KindModel       = "model"         // 模型原始输出
	KindActions     = "actions"       // 模型请求的动作计划
	KindReceipts    = "receipts"      // 动作回执
	KindFinal       = "final"         // 最终答复
	KindError       = "error"         // 错误
)

// Entry 是一条轨迹事件。Content 与结构化字段（Intent/Actions/Receipts）按 kind 二选一或并存。
type Entry struct {
	Ts         string             `json:"ts"`
	RequestID  string             `json:"request_id"`
	Turn       int                `json:"turn,omitempty"`
	Kind       string             `json:"kind"`
	Model      string             `json:"model,omitempty"`
	PromptHash string             `json:"prompt_hash,omitempty"`
	LatencyMs  int64              `json:"latency_ms,omitempty"`
	Content    string             `json:"content,omitempty"`
	Intent     *contract.Intent   `json:"intent,omitempty"`
	Actions    []contract.Action  `json:"actions,omitempty"`
	Receipts   []contract.Receipt `json:"receipts,omitempty"`
	Err        string             `json:"err,omitempty"`
}

// Trajectory 是 append-only JSONL 写入器（0600，逐条落盘，可并发）。
type Trajectory struct {
	mu sync.Mutex
	f  *os.File
}

// Open 打开（或创建）当日轨迹文件 trajectory-YYYYMMDD.jsonl。
func Open(dir string) (*Trajectory, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("创建轨迹目录 %s 失败: %w", dir, err)
	}
	name := filepath.Join(dir, "trajectory-"+time.Now().Format("20060102")+".jsonl")
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开轨迹文件 %s 失败: %w", name, err)
	}
	return &Trajectory{f: f}, nil
}

// Write 追加一条事件（直接写盘，不缓冲，保证崩溃后已写条目完整）。
func (t *Trajectory) Write(e Entry) error {
	if e.Ts == "" {
		e.Ts = time.Now().Format(time.RFC3339)
	}
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("轨迹事件序列化失败: %w", err)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("轨迹写入失败: %w", err)
	}
	return nil
}

// Close 关闭轨迹文件。
func (t *Trajectory) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.f.Close()
}
