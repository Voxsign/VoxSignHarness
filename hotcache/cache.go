// Package hotcache -- cachei.e.  type(VHS-CACHE-001):   status + close  . 
//
// P1: basefilefirstgive** statusand **,  data K2..K6  cursafety .  nowsee cache_impl.go. 
//
//   (K1): 
//
//	L0 processin    wordtable / curbefore  diffname             --  sec ,   out 
//	L1 base keep  diffnamerouteby base /   listfast         --  sec , JSON file
//	L2  end     /api/services etcread-only value          -- only  newtimetrigger 
package hotcache

import (
	"context"
	"time"
)

//  state(K5). 
const (
	StatusOK      = "ok"
	StatusStale   = "stale"
	StatusUnknown = "unknown"
)

// Route isclose   in path(K3  route). 
const (
	RouteExact  = "exact"
	RouteAlias  = "alias"
	RoutePinyin = "pinyin"
	RouteEdit   = "edit"
	RouteMixed  = "mixed"
)

// Hotword is wordtable   (K9: owner ptname"    wordneed   "). 
type Hotword struct {
	Term      string `json:"term"`
	Heat      int    `json:"heat"`
	LastSeen  string `json:"last_seen"`
	Source    string `json:"source"`
	FetchedAt string `json:"fetched_at"`
}

// Alias isdiffnamerouteby   (  ASR change word). 
type Alias struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
	PinyinKey string `json:"pinyin_key,omitempty"` //  audioroutebyuse
	Source    string `json:"source"`
	FetchedAt string `json:"fetched_at"`
}

// Snapshot is L1 keep fast (   heavy , K6). 
type Snapshot struct {
	Hotwords  []Hotword `json:"hotwords"`
	Aliases   []Alias   `json:"aliases"`
	FetchedAt string    `json:"fetched_at"`
	Source    string    `json:"source"`
	Status    string    `json:"status"` // ok | stale | unknown
	Note      string    `json:"note,omitempty"`
}

// Result is  close    close . 
type Result struct {
	Canonical    string  `json:"canonical"`
	Score        float64 `json:"score"`
	Route        string  `json:"route"`
	Source       string  `json:"source"`
	Status       string  `json:"status"`        // ok | stale | unknown(edperiod    use, K4)
	NeedEscalate bool    `json:"need_escalate"` // close     ⇒   signal(K8)
}

// Cache is  cache. 
type Cache struct {
	l1Path  string
	ttl     time.Duration
	fetcher func(ctx context.Context) (Snapshot, error) // L2  end new
	now     func() time.Time
	st      *state
	// blacklistPath is"useuser formmodify " name    path(empty =    ). 
	blacklistPath string
}

// New   cache; l1Path asemptythen   ; fetcher asemptythenno end( basely). 
func New(l1Path string, ttl time.Duration, fetcher func(ctx context.Context) (Snapshot, error)) *Cache {
	return &Cache{l1Path: l1Path, ttl: ttl, fetcher: fetcher, now: time.Now}
}

// L1Path returnback L1   path(keep ize  use). 
func (c *Cache) L1Path() string { return c.l1Path }

//  as nowsee cache_impl.go. 
