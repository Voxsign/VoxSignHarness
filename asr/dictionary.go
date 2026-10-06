// dictionary.go --  ityizeword : langaudiorefer add modify + JSON keep ize +    (needrequire 4.2/4.1). 
//
//   : **  close **,   in Engine.Correct. Pipeline   Correct ofaftercalluse Apply, 
// because  Correct    numityand has C1–C4 finishsafety accept  . 
//
// safesafetygetto( line #3/#5): 
//   - delete/modify    risk: noconfirm  returnback need_confirm, **   **; 
//   -    in   (0.95) connect  ; objtgt audio in(useuser ed write )0.85; 
//   -   JSON fail-open: resolve   keep  fast ,   ,  empty (needrequire 4.9). 
//
// keep ize: fast  JSON(orig write:  timefile + rename)+      `<path>.ops.jsonl`(append-only), 
// atis  heavystart  , also   baseback (needrequire 4.2). 
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

// DictionaryEntry is  word  obj(charsegto needrequire 4.2  close ). 
type DictionaryEntry struct {
	RawSpeech string `json:"raw_speech"` //  lang/ word; asemptytableshow"onlybyobjtgt audio  "
	Target    string `json:"target"`     // tgtapprovewrite 
	Scope     string `json:"scope"`      // global | project
	Priority  int    `json:"priority"`   //     first
	Source    string `json:"source"`     // voice | user_edit | reviewer | manual
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// dictionaryFile iskeep izefast     form. 
type dictionaryFile struct {
	Version int               `json:"version"`
	Entries []DictionaryEntry `json:"entries"`
}

// dictOp is append-only         . 
type dictOp struct {
	Version int             `json:"version"`
	Op      string          `json:"op"`
	Entry   DictionaryEntry `json:"entry"`
	At      string          `json:"at"`
}

// Dictionary isword    timestatus: fast  +  audio   +      . 
type Dictionary struct {
	mu      sync.Mutex
	path    string
	hist    string
	entries []DictionaryEntry
	version int
	hash    string // filein   (     ;   mtime change  )

	table    map[rune]string
	byPinyin map[string][]DictionaryEntry
	pyLens   []int
}

// NewDictionary   (orinitstartize)word . file store  asemptyword ,    . 
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

// Reload   fileis changeizeand   (needrequire 4.1: JSON  keep   ). 
// returnbackis sendoccur  .   JSON  overwrite fast (fail-open). 
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

// reloadLocked readgetfile; force=false time   change connectreturnback. 
// calluseer keep .   period no calluse. 
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
		// fail-open: keep  fast , pipeerror backcalluse (  crash)
		return fmt.Errorf("asr: 词典 JSON 非法（保留旧快照）: %w", err)
	}
	d.entries = f.Entries
	d.version = f.Version
	d.hash = h
	d.reindexLocked()
	return nil
}

// reindexLocked heavy objtgt audio  . calluseer keep . 
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
			continue // tableoutchar(  audio/occur )  and audio  
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

// Add newaddorchangenew  word  obj(same raw_speech+target  assame  ), and i.e.  . 
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
			// keep first   timetime; out  in  obj   has created_at, thenpatch . 
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

// Delete deleteand term   (raw_speech or target)  obj. 
// confirm=false time** delete,    **, calluse data returnback need_confirm( line #3). 
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

// List returnback objfast (by first   ). 
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

// Version returnbackcurbeforefast  baseid. 
func (d *Dictionary) Version() int {
	if d == nil {
		return 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.version
}

// Path returnbackword filepath. 
func (d *Dictionary) Path() string {
	if d == nil {
		return ""
	}
	return d.path
}

// saveLocked orig    +       . calluseer keep . 
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

// Apply   text on useword : first   in(   first), againobjtgt audio in. 
// returnbackcorrectionafter  baseand   Correction(Kind == "dictionary"). 
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

	// 1)    in: by raw_speech     ,      first(needrequire 4.2     ). 
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

	// 2) objtgt audio in: useuser ed write ,   sameaudio  all back(e.g.    ->   ). 
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
// langaudiorefer resolve (needrequire 4.2:  keeplangaudiorefer  stateadd modify)
// ---------------------------------------------------------------------------

var voiceAddPrefixes = []string{"帮我记住", "记住", "记一下", "记录", "添加到词典", "加到词典"}
var voiceDeletePrefixes = []string{"删掉", "删除", "忘掉", "忘记", "去掉"}

// ParseVoiceAdd from"  ,   is in  " class lang resolve outobjtgtwrite . 
// raw_speech  empty: becauseasuseuseronly pos write ,   by** audio**  Apply stage back. 
func ParseVoiceAdd(text string) (DictionaryEntry, bool) {
	rest, ok := trimVoicePrefix(text, voiceAddPrefixes)
	if !ok {
		return DictionaryEntry{}, false
	}
	if i := strings.Index(rest, "是"); i > 0 {
		rest = rest[:i] // "  is in  " -> "  "
	}
	target := strings.Trim(rest, " ，,。.、：:！!？?的了吧啊呀")
	if !validTerm(target) {
		return DictionaryEntry{}, false
	}
	return DictionaryEntry{Target: target, Scope: "global", Priority: 9, Source: "voice"}, true
}

// ParseVoiceDelete from"    " class lang resolve out  word ( needuseuserconfirm). 
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

// validTerm limitrestrictword   andchar ,   pipe sent  becomeword (    ). 
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
