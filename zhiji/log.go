// log.go --    · safety calluseday and detail survival   (   v1.0 §12  ly). 
//
// calluseday  JSONL   write(task_profile/model/outcome/cost/retry safety  pt, 
// isrouteby/        orig );     by store side  (SurvivalRate). 
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

// CallLogStore calluseday storestore(JSONL   , processinandsendsafesafety). 
type CallLogStore struct {
	path string
	mu   sync.Mutex
	f    *os.File
	w    *bufio.Writer
}

// NewCallLogStore  open/  day file(JSONL    form). 
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

// Append     calluseday (JSONL     ). 
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

// Flush  restrict  . 
func (l *CallLogStore) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Flush()
}

// Close close day file. 
func (l *CallLogStore) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.w.Flush()
	return l.f.Close()
}

// Count curbeforeday  num(     rule   use). 
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

// callID day  ID occurbecome( sectimetime , processinunique). 
func callID() string {
	return fmt.Sprintf("call-%d", time.Now().UnixNano())
}
