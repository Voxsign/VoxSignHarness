// store.go --    numdataandin  **base inize**(SK-1..4 + SK-9/10). 
//
//   (Lead 2026-10-03): 
//
//	data/skills/index.json    numdata(  fetched_at / ttl)--inizeafter**list     **
//	data/skills/<id>.json         manifest --** line read**
//
// status state usebase obj example: ok | stale | unknown(**   fail-open tgt unknown,   cur" has"**). 
package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Status is state(and hotcache/route same path). 
const (
	StatusOK      = "ok"
	StatusStale   = "stale"
	StatusUnknown = "unknown"
)

// Index isinize  numdata  . 
type Index struct {
	FetchedAt string  `json:"fetched_at"`
	Skills    []Skill `json:"skills"`
}

// Manifest isinize     in (origkindkeepstoreserveservicesidereturnback, thenat linereadget). 
type Manifest struct {
	ID        string          `json:"id"`
	Version   string          `json:"version"`
	FetchedAt string          `json:"fetched_at"`
	Source    string          `json:"source"`
	Raw       json.RawMessage `json:"raw"`
}

// Store isinizestorestore. 
type Store struct {
	Dir string
	TTL time.Duration
	Now func() time.Time
}

// NewStore   storestore; TTL <=0 ⇒ default 1  time(**UNVALIDATED**). 
func NewStore(dir string, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &Store{Dir: dir, TTL: ttl, Now: time.Now}
}

func (s *Store) indexPath() string             { return filepath.Join(s.Dir, "index.json") }
func (s *Store) manifestPath(id string) string { return filepath.Join(s.Dir, id+".json") }

// SaveIndex    numdata(  fetched_at). 
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

// LoadIndex readget numdataandgiveout state; **  /   ⇒ unknown**( cur" has"). 
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
			return idx, StatusStale, nil // SK-3: edperiodtgt stale,    use  
		}
	}
	return idx, StatusOK, nil
}

// SaveManifest       in . 
func (s *Store) SaveManifest(m Manifest) error {
	if m.ID == "" {
		return fmt.Errorf("skill: manifest missing id")
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

// LoadManifest  linereadget    in ;   /   ⇒ unknown(   see). 
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

// ---- SK-9: underlinestateoverwrite( bycharface )----

// offlineStates is** form  ** underlinestate. 
var offlineStates = map[string]bool{"deprecated": true, "disabled": true, "paused": true, "retired": true}

// StateClass pipe state  class: online / offline / unknown(**    ⇒ unknown,   **). 
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
