// log.go —— 知己 · 全量调用日志与 detail survival 探针（架构 v1.0 §12 落地）。
//
// 调用日志 JSONL 追加写（task_profile/model/outcome/cost/retry 全量埋点，
// 是路由/压缩自举训练集的原料）；探针统计由 store 侧承载（SurvivalRate）。
package zhiji

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CallLogStore 调用日志存储（JSONL 追加，进程内并发安全）。
type CallLogStore struct {
	path string
	mu   sync.Mutex
	f    *os.File
	w    *bufio.Writer
}

// NewCallLogStore 打开/创建日志文件（JSONL 追加模式）。
func NewCallLogStore(dir string) (*CallLogStore, error) {
	if dir == "" {
		return nil, errors.New("zhiji: 日志目录不能为空")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "call_log.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &CallLogStore{path: path, f: f, w: bufio.NewWriter(f)}, nil
}

// Append 追加一条调用日志（JSONL 一行一条）。
func (l *CallLogStore) Append(log CallLog) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if log.ID == "" {
		log.ID = callID()
	}
	if log.At.IsZero() {
		log.At = time.Now()
	}
	b, err := json.Marshal(log)
	if err != nil {
		return err
	}
	if _, err := l.w.Write(append(b, '\n')); err != nil {
		return err
	}
	return l.w.Flush()
}

// Flush 强制落盘。
func (l *CallLogStore) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Flush()
}

// Close 关闭日志文件。
func (l *CallLogStore) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.w.Flush()
	return l.f.Close()
}

// Count 当前日志条数（自举训练集规模统计用）。
func (l *CallLogStore) Count() (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.w.Flush(); err != nil {
		return 0, err
	}
	f, err := os.Open(l.path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	n := 0
	for sc.Scan() {
		if len(sc.Bytes()) > 0 {
			n++
		}
	}
	return n, sc.Err()
}

// callID 日志 ID 生成（纳秒时间戳，进程内唯一）。
func callID() string {
	return fmt.Sprintf("call-%d", time.Now().UnixNano())
}
