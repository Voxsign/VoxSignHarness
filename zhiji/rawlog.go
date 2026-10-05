// rawlog.go —— 知己 · 原始观察 append-only 日志（架构 v1.1 §6.5 / G1 落地）。
//
// OnInput 每条原始观察整行落 raw.jsonl，永不删、永不改、永不覆写；
// 与 STM 滚动窗口互补：STM 是工作记忆（窗口外可丢），raw.jsonl 是外化全量资产。
// detail survival 探针细节只要写进去过，结构上不可能再丢。
package zhiji

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// RawObs 一条原始观察（原文不摘要；Provenance 匿名内嵌=Go 的 inline，字段提升到顶层 JSON）。
type RawObs struct {
	ID     string    `json:"id"` // raw-N 自增
	At     time.Time `json:"at"`
	TurnID string    `json:"turn_id,omitempty"`
	Text   string    `json:"text"` // 原文，不摘要
	Provenance       // 匿名内嵌：origin/model/confidence/derived_from 提升到顶层（外层 TurnID 优先）
}

// RawLog 原始观察追加日志（JSONL 一行一条，绝不覆写已有内容）。
type RawLog struct {
	path    string
	mu      sync.Mutex
	nextRaw int
}

// NewRawLog 打开/创建 raw.jsonl；文件不存在则新建；存在则按行数推算 nextRaw 自增起点。
func NewRawLog(path string) *RawLog {
	l := &RawLog{path: path, nextRaw: 1}
	if path == "" {
		return l
	}
	b, err := os.ReadFile(path)
	if err != nil {
		// 文件不存在 → Append 时 O_CREATE 新建
		return l
	}
	n := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	l.nextRaw = n + 1
	return l
}

// Append 纯追加一行 JSONL（O_APPEND|O_CREATE|O_WRONLY），绝不修改/删除/覆写已有行。
func (l *RawLog) Append(o RawObs) error {
	if l == nil || l.path == "" {
		return errors.New("zhiji: rawlog 未配置")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if o.ID == "" {
		o.ID = fmt.Sprintf("raw-%d", l.nextRaw)
		l.nextRaw++
	}
	if o.At.IsZero() {
		o.At = time.Now()
	}
	b, err := json.Marshal(o)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(append(b, '\n'))
	return err
}

// Len 当前 raw 观察条数（扫文件行数）。
func (l *RawLog) Len() int {
	if l == nil || l.path == "" {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b, err := os.ReadFile(l.path)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}
