// Package cache  now" again "   cache(M2 task  #8 /    v2 §14.1). 
//
//  insemantic: cur (intent, domain,  limit, coreferencealready resolve)    finishsafety nowon alreadyconfirm case time, 
//  connect useon   decide (" again "),   confirm  .   /  /domain base change -> safety  . 
// keep izeas   JSON file,   policy version and per-entry TTL.     dependency. 
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// QuadKey is     : intent / domain /  limit / coreference(already resolve).   safetyetconly same case . 
type QuadKey struct {
	Intent string `json:"intent"`
	Space  string `json:"space"`
	Perm   string `json:"perm"`
	Ref    string `json:"ref"`
}

// Entry is  cache confirmdecide . Version = writetime  policy  base( base  i.e.    ). 
type Entry struct {
	Key       QuadKey   `json:"key"`
	Decision  string    `json:"decision"`
	Version   int       `json:"version"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Store is   cache keep sent . Path as JSON filepath; Version ascurbefore policy  base; 
// TTL as   entry  has period. entries/mu as now node(  out,    frozen API  status). 
type Store struct {
	Path    string        `json:"path"`
	Version int           `json:"version"`
	TTL     time.Duration `json:"ttl_seconds"`

	mu      sync.RWMutex
	entries map[QuadKey]Entry
}

// diskFormat is   JSON  state. 
type diskFormat struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// Open  open(ornew )   cache. file store  -> emptycache, Version=1; store then  . 
func Open(path string, ttl time.Duration) (*Store, error) {
	if filepath.Ext(path) == "" {
		return nil, fmt.Errorf("cache 路径应指向 .json 文件: %q", path)
	}
	s := &Store{Path: path, Version: 1, TTL: ttl, entries: map[QuadKey]Entry{}}
	data, err := os.ReadFile(path)
	if err == nil {
		var df diskFormat
		if jerr := json.Unmarshal(data, &df); jerr != nil {
			return nil, fmt.Errorf("解析缓存文件 %s 失败: %w", path, jerr)
		}
		s.Version = df.Version
		if s.Version < 1 {
			s.Version = 1
		}
		for _, e := range df.Entries {
			s.entries[e.Key] = e
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取缓存文件 %s 失败: %w", path, err)
	}
	return s, nil
}

// persist pipecurbeforestatusorig   (write timefileagain rename,      JSON). 
func (s *Store) persist() error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}
	df := diskFormat{Version: s.Version}
	for _, e := range s.entries {
		df.Entries = append(df.Entries, e)
	}
	data, err := json.MarshalIndent(df, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化缓存失败: %w", err)
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("写缓存临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		return fmt.Errorf("提交缓存文件失败: %w", err)
	}
	return nil
}

// Get     decide . 
//
// [pseudocode logic layer]( writemodule;  in/  rule semantics  VSL,  placeonlywritecontrol flowandrejectpath)
//
// control flow: 
//  1. RLock;   map[QuadKey]Entry. 
//  2.   in -> ("", false). 
//  3.  inafter     (   edi.e. miss, and ity    ): 
//     a.  base : entry.Version != s.Version ->   /  /domain basechangeed,      -> miss. 
//     b. TTL  : now >= entry.ExpiresAt -> edperiod -> miss. 
//  4.   alled -> (entry.Decision, true), pipeline data " again ". 
//
// error: read       (Open already  );  ity  after  restrict  (under  Set/Bump   keep ize). 
func (s *Store) Get(k QuadKey) (string, bool) {
	s.mu.RLock()
	e, ok := s.entries[k]
	s.mu.RUnlock()
	if !ok {
		return "", false
	}
	if e.Version != s.Version {
		s.invalidate(k)
		return "", false
	}
	if !time.Now().Before(e.ExpiresAt) {
		s.invalidate(k)
		return "", false
	}
	return e.Decision, true
}

// invalidate  itydelete  (write ). 
func (s *Store) invalidate(k QuadKey) {
	s.mu.Lock()
	delete(s.entries, k)
	s.mu.Unlock()
}

// Set write/overwrite     decide ,  curbefore policy  baseand TTL,  afterkeep ize. 
//
// [pseudocode logic layer]
//  1. Lock; now := time.Now(). 
//  2. upsert: entries[k] = Entry{Key:k, Decision, Version:s.Version, ExpiresAt: now+TTL}. 
//  3. persist()   (orig  rename). 
//  4.    -> returnbackerror(    ). 
func (s *Store) Set(k QuadKey, decision string) error {
	s.mu.Lock()
	s.entries[k] = Entry{
		Key:       k,
		Decision:  decision,
		Version:   s.Version,
		ExpiresAt: time.Now().Add(s.TTL),
	}
	err := s.persist()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return nil
}

// InvalidateSpace    domain safety cache( domain manifest   /heavy timecalluse). 
//
// [pseudocode logic layer]
//  1. Lock. 
//  2.    entries: Key.Space == space -> delete. 
//  3. persist()   . 
func (s *Store) InvalidateSpace(space string) error {
	s.mu.Lock()
	for k := range s.entries {
		if k.Space == space {
			delete(s.entries, k)
		}
	}
	err := s.persist()
	s.mu.Unlock()
	return err
}

// BumpPolicyVersion   /  /domain basechangeize -> safety  . 
//
// [pseudocode logic layer]
//  1. Lock. 
//  2. s.Version++(  entries     baseid, Get   base  pipe  safety  as miss). 
//  3. persist()   new baseid(entries keep but  onsafety  , thenat  ). 
func (s *Store) BumpPolicyVersion() error {
	s.mu.Lock()
	s.Version++
	err := s.persist()
	s.mu.Unlock()
	return err
}
