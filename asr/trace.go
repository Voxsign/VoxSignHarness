// trace.go -- safetychainroutetrace(needrequire §4.1 / §5 traces-asr.jsonl / §11.8). 
//
//   : **  close **,   in Engine.Correct(Correct   keepkeepnostatus  num). 
// by Pipeline     ofaftercalluse, append-only   ,     timeand in  . 
//
//   get : 
//   - onlyuse stdlib(O_APPEND   write),  has fsnotify  class   dependency; 
//   - writeerserial(mu), readerno --tracefilebase is append-only, dayhowever back ; 
//   - timetime only    ,    handleclose (same  inheavy , step/source  list  ). 
package asr

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// TraceRecord istracein     . 
type TraceRecord struct {
	Kind      TraceStepKind `json:"kind,omitempty"`
	RequestID string        `json:"request_id"`
	SessionID string        `json:"session_id,omitempty"`
	Step      string        `json:"step"`   // retain|clean|dict|context|reference|intent|domain|punctuate
	Ms        float64       `json:"ms"`     //    time( sec)
	Source    string        `json:"source"` //  in  (rule:xxx / dictionary / store / -)
	Detail    string        `json:"detail,omitempty"`
	At        string        `json:"at"` // RFC3339Nano(UTC)
}

// Tracer is append-only  tracewrite . 
type Tracer struct {
	mu   sync.Mutex
	path string
	f    *os.File
	seq  int64
}

// NewTracer  open(or  )tracefile, only  ,   disconnect. 
func NewTracer(path string) (*Tracer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("asr: 打开轨迹文件 %s: %w", path, err)
	}
	return &Tracer{path: path, f: f}, nil
}

// NextRequestID occurbecome  processin call add  requireid(thenatpipe   require    raise ). 
func (t *Tracer) NextRequestID(prefix string) string {
	if t == nil {
		return prefix
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	return fmt.Sprintf("%s-%d", prefix, t.seq)
}

// Append       . empty At   patchcurbeforetimetime. 
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

// Path returnbacktracefilepath(  /  use). 
func (t *Tracer) Path() string {
	if t == nil {
		return ""
	}
	return t.path
}

// Close close file. 
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
