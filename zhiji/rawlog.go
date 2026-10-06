// rawlog.go --    · origstart   append-only day (   v1.1 §6.5 / G1  ly). 
//
// OnInput   origstart      raw.jsonl,    ,   modify,    write; 
// and STM      patch: STM is    (  out  ), raw.jsonl isoutizesafety  produce. 
// detail survival    nodeonlyneedwrite  ed, close on   again . 
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

// RawObs   origstart  (orig   need; Provenance  namein =Go   inline, charseg  totop  JSON). 
type RawObs struct {
	ID     string    `json:"id"` // raw-N  add
	At     time.Time `json:"at"`
	TurnID string    `json:"turn_id,omitempty"`
	Text   string    `json:"text"` // orig ,   need
	Provenance       //  namein : origin/model/confidence/derived_from   totop (out  TurnID  first)
}

// RawLog origstart    day (JSONL     ,    writealreadyhasin ). 
type RawLog struct {
	path    string
	mu      sync.Mutex
	nextRaw int
}

// NewRawLog  open/   raw.jsonl; file store thennew ; store thenby num   nextRaw  addraisept. 
func NewRawLog(path string) *RawLog {
	l := &RawLog{path: path, nextRaw: 1}
	if path == "" {
		return l
	}
	b, err := os.ReadFile(path)
	if err != nil {
		// file store  -> Append time O_CREATE new 
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

// Append       JSONL(O_APPEND|O_CREATE|O_WRONLY),   modify/delete/ writealreadyhas . 
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

// Len curbefore raw    num( file num). 
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
