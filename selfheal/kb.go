package selfheal

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// KBFileName 是异常知识库文件名（位于 <log_dir>/ 下）。
const KBFileName = "exceptions.jsonl"

// KBEntry 是 exceptions.jsonl 的一行（按指纹去重，加载时取最新 updated）。
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

// KB 是异常知识库：错误指纹 → 根因/修复。加载时按 fingerprint 去重取最新；
// 命中直接复用（0 模型调用）；回写仅发生在【修复成功后】。并发安全。
type KB struct {
	path string
	mu   sync.Mutex
	m    map[string]KBEntry
}

// OpenKB 加载 <log_dir>/exceptions.jsonl；文件不存在 → 空知识库（不报错）。
// 解析坏行跳过，绝不因坏行让诊断层整体不可用。
func OpenKB(path string) *KB {
	kb := &KB{path: path, m: map[string]KBEntry{}}
	f, err := os.Open(path)
	if err != nil {
		return kb // 首次运行/文件缺失 → 空库
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
			continue // 坏行跳过
		}
		// 同指纹：后读的覆盖先读的（文件按时间追加，后者更新）。
		kb.m[e.Fingerprint] = e
	}
	return kb
}

// Lookup 按指纹查知识库；命中返回结论（source=kb）。
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

// Remember 在【修复成功后】回写/更新一条知识：指纹已存在则 hits+1 并刷新结论与 updated；
// 不存在则追加一行。任何 IO 错误都静默（知识库是加速器，写失败不影响主链）。
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

// appendLine 把一条记录追加到磁盘（调用方已持数据副本，不持锁，避免 IO 阻塞其他读）。
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

// Count 返回当前知识库条目数（测试/诊断用）。
func (kb *KB) Count() int {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	return len(kb.m)
}
