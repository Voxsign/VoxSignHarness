// trace.go —— 全链路轨迹（需求 §4.1 / §5 traces-asr.jsonl / §11.8）。
//
// 定位：**独立结构**，不进入 Engine.Correct（Correct 必须保持无状态纯函数）。
// 由 Pipeline 在每一步之后调用，append-only 落盘，每步带耗时与命中来源。
//
// 设计取舍：
//   - 只用 stdlib（O_APPEND 追加写），没有 fsnotify 这类第三方依赖；
//   - 写者串行（mu），读者无锁——轨迹文件本身是 append-only，天然可回放；
//   - 时间戳只在记录里，不影响处理结果（同一输入重放，step/source 序列一致）。
package asr

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// TraceRecord 是轨迹中的一条记录。
type TraceRecord struct {
	RequestID string  `json:"request_id"`
	SessionID string  `json:"session_id,omitempty"`
	Step      string  `json:"step"`   // retain|clean|dict|context|reference|intent|domain|punctuate
	Ms        float64 `json:"ms"`     // 该步耗时（毫秒）
	Source    string  `json:"source"` // 命中来源（rule:xxx / dictionary / store / -）
	Detail    string  `json:"detail,omitempty"`
	At        string  `json:"at"` // RFC3339Nano（UTC）
}

// Tracer 是 append-only 的轨迹写入器。
type Tracer struct {
	mu   sync.Mutex
	path string
	f    *os.File
	seq  int64
}

// NewTracer 打开（或创建）轨迹文件，只追加、不截断。
func NewTracer(path string) (*Tracer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("asr: 打开轨迹文件 %s: %w", path, err)
	}
	return &Tracer{path: path, f: f}, nil
}

// NextRequestID 生成一个进程内单调递增的请求号（便于把一次请求的多步串起来）。
func (t *Tracer) NextRequestID(prefix string) string {
	if t == nil {
		return prefix
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	return fmt.Sprintf("%s-%d", prefix, t.seq)
}

// Append 追加一条记录。空 At 自动补当前时间。
func (t *Tracer) Append(rec TraceRecord) error {
	if t == nil || t.f == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if rec.At == "" {
		rec.At = time.Now().UTC().Format(time.RFC3339Nano)
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("asr: 轨迹序列化失败: %w", err)
	}
	if _, err := t.f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("asr: 轨迹写入失败: %w", err)
	}
	return nil
}

// Path 返回轨迹文件路径（测试/审计用）。
func (t *Tracer) Path() string {
	if t == nil {
		return ""
	}
	return t.path
}

// Close 关闭文件。
func (t *Tracer) Close() error {
	if t == nil || t.f == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.f == nil {
		return nil
	}
	err := t.f.Close()
	t.f = nil
	return err
}
