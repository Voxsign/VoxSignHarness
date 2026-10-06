package selfheal

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// KBFileName iserror   filename( at <log_dir>/ under). 
const KBFileName = "exceptions.jsonl"

// KBEntry is exceptions.jsonl    (byrefer  heavy,   timeget new updated). 
type KBEntry struct {
	Fingerprint string         `json:"fingerprint"`
	Category    string         `json:"category"`
	RootCause   string         `json:"root_cause"`
	Confidence  float64        `json:"confidence"`
	Recoverable bool           `json:"recoverable"`
	Suggestion  string         `json:"suggestion"`
	Action      string         `json:"action"`
	RetryParams map[string]any `json:"retry_params,omitempty"`
	Hits        int            `json:"hits"`
	Updated     string         `json:"updated"`
}

// KB iserror   : errorrefer  -> rootbecause/fix .   timeby fingerprint  heavyget new; 
//  in connect use(0  typecalluse); write-backonlysendoccur [fix become after]. andsendsafesafety. 
type KB struct {
	path string
	mu   sync.Mutex
	m    map[string]KBEntry
}

// OpenKB    <log_dir>/exceptions.jsonl; file store  -> empty   (   ). 
// resolve    ed,   because    disconnect  body  use. 
func OpenKB(path string) *KB {
	kb := &KB{path: path, m: map[string]KBEntry{}}
	f, err := os.Open(path)
	if err != nil {
		return kb // first   /file   -> empty 
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e KBEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil || e.Fingerprint == "" {
			continue //    ed
		}
		// samerefer : afterread overwritefirstread (filebytimetime  , aftererchangenew). 
		kb.m[e.Fingerprint] = e
	}
	return kb
}

// Lookup byrefer     ;  inreturnbackclose (source=kb). 
func (kb *KB) Lookup(fp string) (Diagnosis, bool) {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	e, ok := kb.m[fp]
	if !ok {
		return Diagnosis{}, false
	}
	d := Diagnosis{
		Category: e.Category, RootCause: e.RootCause, Confidence: e.Confidence,
		Recoverable: e.Recoverable, Suggestion: e.Suggestion, Action: e.Action,
		RetryParams: e.RetryParams, Source: "kb", Fingerprint: fp,
	}
	d.normalizeAction()
	d.friendlySuggestion()
	return d, true
}

// Remember  [fix become after]write-back/changenew    : refer alreadystore then hits+1 and newclose and updated; 
//  store then    .    IO errorall  (   is   , write      chain). 
func (kb *KB) Remember(d Diagnosis) {
	kb.mu.Lock()
	e := KBEntry{
		Fingerprint: d.Fingerprint, Category: d.Category, RootCause: d.RootCause,
		Confidence: d.Confidence, Recoverable: d.Recoverable, Suggestion: d.Suggestion,
		Action: d.Action, RetryParams: d.RetryParams, Hits: 1,
		Updated: time.Now().Format(time.RFC3339),
	}
	if old, ok := kb.m[d.Fingerprint]; ok {
		e.Hits = old.Hits + 1
	}
	kb.m[d.Fingerprint] = e
	kb.mu.Unlock()
	kb.appendLine(e)
}

// appendLine pipe      to  (calluse alreadykeepnumdata base,  keep ,    IO   its read). 
func (kb *KB) appendLine(e KBEntry) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	f, err := os.OpenFile(kb.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// Count returnbackcurbefore    objnum(  / disconnectuse). 
func (kb *KB) Count() int {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	return len(kb.m)
}
