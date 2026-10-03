// store.go —— 技能元数据与内容的**本机内化**（SK-1..4 + SK-9/10）。
//
// 设计（Lead 2026-10-03）：
//
//	data/skills/index.json   元数据（带 fetched_at / ttl）——内化后**列技能不联网**
//	data/skills/<id>.json    单个技能 manifest ——**离线可读**
//
// 状态三态沿用本项目惯例：ok | stale | unknown（**失败 fail-open 标 unknown，绝不当"没有"**）。
package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Status 是三态（与 hotcache/route 同口径）。
const (
	StatusOK      = "ok"
	StatusStale   = "stale"
	StatusUnknown = "unknown"
)

// Index 是内化的元数据索引。
type Index struct {
	FetchedAt string  `json:"fetched_at"`
	Skills    []Skill `json:"skills"`
}

// Manifest 是内化的单个技能内容（原样保存服务侧返回，便于离线读取）。
type Manifest struct {
	ID        string          `json:"id"`
	Version   string          `json:"version"`
	FetchedAt string          `json:"fetched_at"`
	Source    string          `json:"source"`
	Raw       json.RawMessage `json:"raw"`
}

// Store 是内化存储。
type Store struct {
	Dir string
	TTL time.Duration
	Now func() time.Time
}

// NewStore 构造存储；TTL ≤0 ⇒ 默认 1 小时（**UNVALIDATED**）。
func NewStore(dir string, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &Store{Dir: dir, TTL: ttl, Now: time.Now}
}

func (s *Store) indexPath() string             { return filepath.Join(s.Dir, "index.json") }
func (s *Store) manifestPath(id string) string { return filepath.Join(s.Dir, id+".json") }

// SaveIndex 落盘元数据（带 fetched_at）。
func (s *Store) SaveIndex(skills []Skill) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	idx := Index{FetchedAt: s.Now().UTC().Format(time.RFC3339), Skills: skills}
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), b, 0o600)
}

// LoadIndex 读取元数据并给出三态；**缺失/损坏 ⇒ unknown**（不当"没有"）。
func (s *Store) LoadIndex() (Index, string, error) {
	b, err := os.ReadFile(s.indexPath())
	if err != nil {
		return Index{}, StatusUnknown, err
	}
	var idx Index
	if err := json.Unmarshal(b, &idx); err != nil {
		return Index{}, StatusUnknown, err
	}
	if t, err := time.Parse(time.RFC3339, idx.FetchedAt); err == nil {
		if s.Now().Sub(t) > s.TTL {
			return idx, StatusStale, nil // SK-3：过期标 stale，不静默用旧的
		}
	}
	return idx, StatusOK, nil
}

// SaveManifest 落盘单个技能内容。
func (s *Store) SaveManifest(m Manifest) error {
	if m.ID == "" {
		return fmt.Errorf("skill: manifest 缺 id")
	}
	if m.FetchedAt == "" {
		m.FetchedAt = s.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.manifestPath(m.ID), b, 0o600)
}

// LoadManifest 离线读取单个技能内容；缺失/损坏 ⇒ unknown（失败可见）。
func (s *Store) LoadManifest(id string) (Manifest, string, error) {
	b, err := os.ReadFile(s.manifestPath(id))
	if err != nil {
		return Manifest{}, StatusUnknown, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, StatusUnknown, err
	}
	return m, StatusOK, nil
}

// ---- SK-9：下线态覆盖（不按字面猜）----

// offlineStates 是**显式登记**的下线态。
var offlineStates = map[string]bool{"deprecated": true, "disabled": true, "paused": true, "retired": true}

// StateClass 把 state 归类：online / offline / unknown（**未登记 ⇒ unknown，不猜**）。
func StateClass(state string) string {
	if state == "" {
		return "unknown"
	}
	if offlineStates[state] {
		return "offline"
	}
	if state == "active" {
		return "online"
	}
	return "unknown"
}
