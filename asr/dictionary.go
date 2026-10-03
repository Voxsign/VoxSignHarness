// dictionary.go —— 个性化词典：语音指令增删改 + JSON 持久化 + 热加载（需求 4.2/4.1）。
//
// 定位：**独立结构**，不进入 Engine.Correct。Pipeline 在 Correct 之后调用 Apply，
// 因此 Correct 的纯函数性与既有 C1–C4 完全不受影响。
//
// 安全取向（红线 #3/#5）：
//   - 删除/改偏好属高风险：无确认一律返回 need_confirm，**不落盘**；
//   - 精确命中高置信（0.95）直接替换；目标近音命中（用户教过的写法）0.85；
//   - 坏 JSON fail-open：解析失败保留旧快照，不崩、不空转（需求 4.9）。
//
// 持久化：快照 JSON（原子写：临时文件 + rename）+ 操作历史 `<path>.ops.jsonl`（append-only），
// 于是既能重启恢复，也能做版本回溯（需求 4.2）。
package asr

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DictionaryEntry 是一条词典条目（字段对齐需求 4.2 的结构）。
type DictionaryEntry struct {
	RawSpeech string `json:"raw_speech"` // 口语/错词；为空表示"只按目标近音匹配"
	Target    string `json:"target"`     // 标准写法
	Scope     string `json:"scope"`      // global | project
	Priority  int    `json:"priority"`   // 越大越优先
	Source    string `json:"source"`     // voice | user_edit | reviewer | manual
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// dictionaryFile 是持久化快照的磁盘格式。
type dictionaryFile struct {
	Version int               `json:"version"`
	Entries []DictionaryEntry `json:"entries"`
}

// dictOp 是 append-only 操作历史里的一条。
type dictOp struct {
	Version int             `json:"version"`
	Op      string          `json:"op"`
	Entry   DictionaryEntry `json:"entry"`
	At      string          `json:"at"`
}

// Dictionary 是词典的运行时状态：快照 + 近音索引 + 热加载水位。
type Dictionary struct {
	mu      sync.Mutex
	path    string
	hist    string
	entries []DictionaryEntry
	version int
	hash    string // 文件内容哈希（热加载水位；比 mtime 更可靠）

	table    map[rune]string
	byPinyin map[string][]DictionaryEntry
	pyLens   []int
}

// NewDictionary 加载（或初始化）词典。文件不存在视为空词典，不报错。
func NewDictionary(path string) (*Dictionary, error) {
	d := &Dictionary{
		path:     path,
		hist:     path + ".ops.jsonl",
		table:    buildPinyinTable(),
		byPinyin: map[string][]DictionaryEntry{},
	}
	if err := d.reloadLocked(true); err != nil {
		return nil, err
	}
	return d, nil
}

// Reload 检查文件是否变化并热加载（需求 4.1：JSON 支持热加载）。
// 返回是否发生了替换。坏 JSON 不覆盖旧快照（fail-open）。
func (d *Dictionary) Reload() (bool, error) {
	if d == nil {
		return false, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	before := d.hash
	if err := d.reloadLocked(false); err != nil {
		return false, err
	}
	return d.hash != before, nil
}

// reloadLocked 读取文件；force=false 时哈希未变直接返回。
// 调用者须持锁。构造期可无锁调用。
func (d *Dictionary) reloadLocked(force bool) error {
	data, err := os.ReadFile(d.path)
	if err != nil {
		if os.IsNotExist(err) {
			if force {
				d.entries = nil
				d.hash = ""
				d.version = 0
				d.reindexLocked()
			}
			return nil
		}
		return fmt.Errorf("asr: 读词典 %s: %w", d.path, err)
	}
	sum := sha256.Sum256(data)
	h := hex.EncodeToString(sum[:])
	if !force && h == d.hash {
		return nil
	}
	var f dictionaryFile
	if err := json.Unmarshal(data, &f); err != nil {
		// fail-open：保留旧快照，把错误交回调用方（不 crash）
		return fmt.Errorf("asr: 词典 JSON 非法（保留旧快照）: %w", err)
	}
	d.entries = f.Entries
	d.version = f.Version
	d.hash = h
	d.reindexLocked()
	return nil
}

// reindexLocked 重建目标近音索引。调用者须持锁。
func (d *Dictionary) reindexLocked() {
	d.byPinyin = map[string][]DictionaryEntry{}
	lens := map[int]bool{}
	for _, e := range d.entries {
		if e.Target == "" {
			continue
		}
		rs := []rune(e.Target)
		parts := make([]string, len(rs))
		ok := true
		for i, r := range rs {
			s, good := d.table[r]
			if !good {
				ok = false
				break
			}
			parts[i] = s
		}
		if !ok {
			continue // 表外字（含多音/生僻）不参与近音匹配
		}
		key := strings.Join(parts, "|")
		d.byPinyin[key] = append(d.byPinyin[key], e)
		lens[len(rs)] = true
	}
	d.pyLens = d.pyLens[:0]
	for n := range lens {
		d.pyLens = append(d.pyLens, n)
	}
	sort.Ints(d.pyLens)
}

// Add 新增或更新一条词典条目（同 raw_speech+target 视为同一条），并立即落盘。
func (d *Dictionary) Add(e DictionaryEntry) error {
	if d == nil {
		return fmt.Errorf("asr: 词典未初始化")
	}
	if strings.TrimSpace(e.Target) == "" {
		return fmt.Errorf("asr: 词典条目缺 target")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if e.Scope == "" {
		e.Scope = "global"
	}
	if e.Priority == 0 {
		e.Priority = 5
	}
	if e.Source == "" {
		e.Source = "manual"
	}
	e.CreatedAt = now
	d.mu.Lock()
	defer d.mu.Unlock()
	replaced := false
	for i := range d.entries {
		if d.entries[i].RawSpeech == e.RawSpeech && d.entries[i].Target == e.Target {
			// 保留首次创建时间；外部导入的条目可能没有 created_at，则补齐。
			if d.entries[i].CreatedAt != "" {
				e.CreatedAt = d.entries[i].CreatedAt
			}
			e.UpdatedAt = now
			d.entries[i] = e
			replaced = true
			break
		}
	}
	if !replaced {
		d.entries = append(d.entries, e)
	}
	d.reindexLocked()
	return d.saveLocked("add", e)
}

// Delete 删除与 term 匹配（raw_speech 或 target）的条目。
// confirm=false 时**不删除、不落盘**，调用方据此返回 need_confirm（红线 #3）。
func (d *Dictionary) Delete(term string, confirm bool) (bool, error) {
	if d == nil {
		return false, fmt.Errorf("asr: 词典未初始化")
	}
	if !confirm {
		return false, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	kept := d.entries[:0]
	var removed []DictionaryEntry
	for _, e := range d.entries {
		if e.Target == term || e.RawSpeech == term {
			removed = append(removed, e)
			continue
		}
		kept = append(kept, e)
	}
	if len(removed) == 0 {
		return false, nil
	}
	d.entries = kept
	d.reindexLocked()
	if err := d.saveLocked("delete", removed[0]); err != nil {
		return false, err
	}
	return true, nil
}

// List 返回条目快照（按优先级降序）。
func (d *Dictionary) List() []DictionaryEntry {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]DictionaryEntry, len(d.entries))
	copy(out, d.entries)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].Target < out[j].Target
	})
	return out
}

// Version 返回当前快照版本号。
func (d *Dictionary) Version() int {
	if d == nil {
		return 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.version
}

// Path 返回词典文件路径。
func (d *Dictionary) Path() string {
	if d == nil {
		return ""
	}
	return d.path
}

// saveLocked 原子落盘 + 追加操作历史。调用者须持锁。
func (d *Dictionary) saveLocked(op string, e DictionaryEntry) error {
	d.version++
	b, err := json.MarshalIndent(dictionaryFile{Version: d.version, Entries: d.entries}, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(d.path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	tmp := d.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, d.path); err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	d.hash = hex.EncodeToString(sum[:])

	if d.hist != "" {
		rec, _ := json.Marshal(dictOp{Version: d.version, Op: op, Entry: e, At: time.Now().UTC().Format(time.RFC3339Nano)})
		if fh, err := os.OpenFile(d.hist, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			_, _ = fh.Write(append(rec, '\n'))
			_ = fh.Close()
		}
	}
	return nil
}

// Apply 在 text 上应用词典：先精确命中（最长优先），再目标近音命中。
// 返回纠错后的文本与逐条 Correction（Kind == "dictionary"）。
func (d *Dictionary) Apply(text string) (string, []Correction) {
	if d == nil || text == "" {
		return text, nil
	}
	d.mu.Lock()
	entries := append([]DictionaryEntry(nil), d.entries...)
	byPinyin := d.byPinyin
	lens := append([]int(nil), d.pyLens...)
	d.mu.Unlock()
	if len(entries) == 0 {
		return text, nil
	}

	runes := []rune(text)
	var spans []span

	// 1) 精确命中：按 raw_speech 长度降序，最长匹配优先（需求 4.2 匹配策略）。
	exact := make([]DictionaryEntry, 0, len(entries))
	for _, e := range entries {
		if e.RawSpeech != "" {
			exact = append(exact, e)
		}
	}
	sort.SliceStable(exact, func(i, j int) bool {
		if len([]rune(exact[i].RawSpeech)) != len([]rune(exact[j].RawSpeech)) {
			return len([]rune(exact[i].RawSpeech)) > len([]rune(exact[j].RawSpeech))
		}
		return exact[i].Priority > exact[j].Priority
	})
	for i := 0; i < len(runes); {
		matched := false
		for _, e := range exact {
			fr := []rune(e.RawSpeech)
			if i+len(fr) <= len(runes) && string(runes[i:i+len(fr)]) == e.RawSpeech {
				spans = append(spans, span{
					start: i, end: i + len(fr), from: e.RawSpeech, to: e.Target,
					kind: "dictionary", conf: 0.95,
					evidence: "词典精确命中：" + e.RawSpeech + " → " + e.Target + "（source=" + e.Source + "）",
				})
				i += len(fr)
				matched = true
				break
			}
		}
		if !matched {
			i++
		}
	}

	// 2) 目标近音命中：用户教过的写法，任何同音窗口都召回（如 季总 → 冀总）。
	for i := 0; i < len(runes); i++ {
		for _, l := range lens {
			if i+l > len(runes) {
				break
			}
			win := runes[i : i+l]
			parts := make([]string, l)
			ok := true
			for k, r := range win {
				s, good := d.table[r]
				if !good {
					ok = false
					break
				}
				parts[k] = s
			}
			if !ok {
				continue
			}
			key := strings.Join(parts, "|")
			for _, e := range byPinyin[key] {
				w := string(win)
				if w == e.Target {
					continue
				}
				spans = append(spans, span{
					start: i, end: i + l, from: w, to: e.Target,
					kind: "dictionary", conf: 0.85,
					evidence: "词典近音命中：目标「" + e.Target + "」（source=" + e.Source + "）",
				})
			}
		}
	}

	resolved := resolve(spans, len(runes))
	if len(resolved) == 0 {
		return text, nil
	}
	return rebuild(runes, runeOffsets(text), resolved)
}

// ---------------------------------------------------------------------------
// 语音指令解析（需求 4.2：支持语音指令动态增删改）
// ---------------------------------------------------------------------------

var voiceAddPrefixes = []string{"帮我记住", "记住", "记一下", "记录", "添加到词典", "加到词典"}
var voiceDeletePrefixes = []string{"删掉", "删除", "忘掉", "忘记", "去掉"}

// ParseVoiceAdd 从"记住，冀总是冀中的冀"这类口语里解析出目标写法。
// raw_speech 留空：因为用户只说了正确写法，错法由**近音**在 Apply 阶段召回。
func ParseVoiceAdd(text string) (DictionaryEntry, bool) {
	rest, ok := trimVoicePrefix(text, voiceAddPrefixes)
	if !ok {
		return DictionaryEntry{}, false
	}
	if i := strings.Index(rest, "是"); i > 0 {
		rest = rest[:i] // "冀总是冀中的冀" → "冀总"
	}
	target := strings.Trim(rest, " ，,。.、：:！!？?的了吧啊呀")
	if !validTerm(target) {
		return DictionaryEntry{}, false
	}
	return DictionaryEntry{Target: target, Scope: "global", Priority: 9, Source: "voice"}, true
}

// ParseVoiceDelete 从"删掉冀总"这类口语里解析出待删词条（仍需用户确认）。
func ParseVoiceDelete(text string) (string, bool) {
	rest, ok := trimVoicePrefix(text, voiceDeletePrefixes)
	if !ok {
		return "", false
	}
	term := strings.Trim(rest, " ，,。.、：:！!？?这个词条")
	if !validTerm(term) {
		return "", false
	}
	return term, true
}

func trimVoicePrefix(text string, prefixes []string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	for _, p := range prefixes {
		if strings.HasPrefix(trimmed, p) {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, p))
			return strings.TrimLeft(rest, "，,：: "), true
		}
	}
	return "", false
}

// validTerm 限制词条长度与字符，避免把整句话学成词条（宁漏不错）。
func validTerm(s string) bool {
	rs := []rune(s)
	if len(rs) == 0 || len(rs) > 12 {
		return false
	}
	for _, r := range rs {
		if r == ' ' || r == '\t' || r == '\n' {
			return false
		}
		if isPunctRune(r) {
			return false
		}
	}
	return true
}
