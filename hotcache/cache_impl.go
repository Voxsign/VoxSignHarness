// cache_impl.go --   cache + close   route(K2..K6)  now. 
package hotcache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"voicesign-harness/asr"
)

type state struct {
	mu      sync.Mutex
	hot     map[string]*Hotword
	aliases []Alias
	meta    Snapshot // onlyuse Status/FetchedAt/Source/Note
	// blacklist is"useuser formmodify "  state name (term ->   origbecause), only modifywrite,   routeby. 
	blacklist map[string]string
}

// ---- Observe / PutAlias ----

// Observe     wordoutnow(L0,   out ). 
func (c *Cache) Observe(term, source string) {
	term = strings.TrimSpace(term)
	if term == "" {
		return
	}
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.hot[term]
	if h == nil {
		h = &Hotword{Term: term}
		s.hot[term] = h
	}
	h.Heat++
	h.Source = source
	now := c.now().UTC().Format(time.RFC3339)
	h.LastSeen = now
	h.FetchedAt = now
}

// PutAlias   diffname(L0); sametime  its audio by keep audiorouteby. 
func (c *Cache) PutAlias(alias, canonical, source string) {
	alias, canonical = strings.TrimSpace(alias), strings.TrimSpace(canonical)
	if alias == "" || canonical == "" {
		return
	}
	now := c.now().UTC().Format(time.RFC3339)
	pk, _ := asr.PinyinKey(alias)
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.aliases {
		if s.aliases[i].Alias == alias {
			s.aliases[i] = Alias{Alias: alias, Canonical: canonical, PinyinKey: pk, Source: source, FetchedAt: now}
			return
		}
	}
	s.aliases = append(s.aliases, Alias{Alias: alias, Canonical: canonical, PinyinKey: pk, Source: source, FetchedAt: now})
}

func (c *Cache) state() *state {
	if c.st == nil {
		c.st = &state{hot: map[string]*Hotword{}, blacklist: map[string]string{}}
	}
	return c.st
}

// ---- close   route(K3)----

// Lookup     :    -> diffname ->  audio audio ->     ; heat  ;   thengive  signal(K8). 
func (c *Cache) Lookup(term string) (Result, bool) { return c.lookupInternal(term, false) }

// lookupInternal is Lookup   now; skipGeneric=true time ed useworddiffname(modifywriteuseway). 
func (c *Cache) lookupInternal(term string, skipGeneric bool) (Result, bool) {
	term = strings.TrimSpace(term)
	if term == "" {
		return Result{NeedEscalate: true}, false
	}
	st := c.Snapshot().Status
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()

	// ①   ( word /    canonical)
	if h, ok := s.hot[term]; ok {
		score := 1.0 + float64(h.Heat)*0.01
		if score > 1.0 {
			score = 1.0
		}
		return Result{Canonical: term, Score: score, Route: RouteExact, Source: h.Source, Status: st}, true
	}
	for _, a := range s.aliases {
		if a.Canonical == term {
			return Result{Canonical: a.Canonical, Score: 1.0, Route: RouteExact, Source: a.Source, Status: st}, true
		}
	}
	// ② diffname
	for _, a := range s.aliases {
		if skipGeneric && isGenericForRewrite(a.Alias) {
			continue
		}
		if a.Alias == term {
			return Result{Canonical: a.Canonical, Score: 0.95, Route: RouteAlias, Source: a.Source, Status: st}, true
		}
	}
	// ③  audio audio(same audionode list)
	if key, ok := asr.PinyinKey(term); ok {
		for _, a := range s.aliases {
			if a.PinyinKey != "" && a.PinyinKey == key {
				return Result{Canonical: a.Canonical, Score: 0.85, Route: RoutePinyin, Source: a.Source, Status: st}, true
			}
		}
	}
	// ③b   rule ize(G1  ② ): in get audio,   keep ( write),  body  after  . 
	// **only  body  ,    word** ⇒      accept  (prevent" edhead"). 
	//   handle(prevent"  "): if diffname **rule word  **  afteretcat   ⇒ get (  signal); 
	//  thenif**   same canonical**   same   ⇒ **    **(  ). 
	if key, ok := MixedKey(term); ok {
		canonicalSelf := ""
		matches := map[string]string{}
		for _, a := range s.aliases {
			ak, ok1 := MixedKey(a.Alias)
			ck, ok2 := MixedKey(a.Canonical)
			hit := (ok1 && ak == key) || (ok2 && ck == key)
			if !hit {
				continue
			}
			if ok2 && ck == key {
				canonicalSelf = a.Canonical // rule word  then  become    ⇒   
			}
			matches[a.Canonical] = a.Source
		}
		if canonicalSelf != "" {
			return Result{Canonical: canonicalSelf, Score: 0.90, Route: RouteMixed, Source: matches[canonicalSelf], Status: st}, true
		}
		if len(matches) == 1 {
			for c, src := range matches {
				return Result{Canonical: c, Score: 0.90, Route: RouteMixed, Source: src, Status: st}, true
			}
		}
		if len(matches) > 1 {
			return Result{NeedEscalate: true, Status: st, Route: RouteMixed}, false //  resolve ⇒   
		}
	}

	// ④      + **    **(Lead  decide: first   , again   in). 
	//
	//  useity :     and     **splitnumdiff <  value** ⇒ **    **(    in,   ). 
	// and VHS-ZHIJI-001 §2.1"  clarification,   "same restrict. 
	// ⚠️    value **UNVALIDATED**( tgt ). 
	const editMaxDistance = 1
	// ③     : **hasboundary  ** -- onlycur in   (>=8 char )only allow dist<=2. 
	// ⚠️ UNVALIDATED: 8 / 2 allis tgt  getvalue.          ⇒ bydiff   and
	// "same  same canonical ⇒ reject"  bot. 
	const longInputLen = 8
	const editMaxDistanceLong = 2
	const minScoreGap = 2
	//  to     :   on char ofdiff( ops vs  ops)  by as"modifywrite"  data. 
	// ⚠️ UNVALIDATED: 0.15 is get ,  tgt . 
	const maxEditRatio = 0.15

	type cand struct {
		canonical string
		dist      int
		source    string
	}
	var cands []cand
	termLen := float64(len([]rune(term)))
	for _, a := range s.aliases {
		seen := map[string]bool{}
		for _, c := range []string{a.Alias, a.Canonical} {
			if c == "" || seen[c] {
				continue // diffnameandrule word same ⇒  heavy    
			}
			seen[c] = true
			d := levenshtein(term, c)
			if float64(d) > maxEditRatio*termLen {
				continue //  todiffdiffed  ⇒   as  (prevent"  ")
			}
			maxD := editMaxDistance
			if len([]rune(term)) >= longInputLen {
				maxD = editMaxDistanceLong
			}
			if d <= maxD {
				cands = append(cands, cand{canonical: a.Canonical, dist: d, source: a.Source})
			}
		}
	}
	if len(cands) > 0 {
		sort.Slice(cands, func(i, j int) bool { return cands[i].dist < cands[j].dist })
		if len(cands) > 1 && cands[1].dist-cands[0].dist < minScoreGap {
			// diff    ⇒ **  **:  give  ,  out  signal
			return Result{NeedEscalate: true, Status: st, Route: RouteEdit}, false
		}
		// same   onoutnow** same canonical** ⇒ samekind  
		for _, c := range cands[1:] {
			if c.dist == cands[0].dist && c.canonical != cands[0].canonical {
				return Result{NeedEscalate: true, Status: st, Route: RouteEdit}, false
			}
		}
		return Result{Canonical: cands[0].canonical, Score: 0.70, Route: RouteEdit, Source: cands[0].source, Status: st}, true
	}
	// close     ⇒   signal(K8),  is" store "
	return Result{NeedEscalate: true, Status: st}, false
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// ----  new / fast  /   (K4/K5/K6/K7)----

// Refresh from L2  new: become thenoverwritediffnameand  L1;    fail-open tgt unknown(keep basely,   empty). 
func (c *Cache) Refresh(ctx context.Context) Snapshot {
	s := c.state()
	now := c.now().UTC()
	if c.fetcher == nil {
		s.mu.Lock()
		s.meta.Status = StatusOK
		s.meta.FetchedAt = now.Format(time.RFC3339)
		s.meta.Source = "local-only"
		out := c.snapshotLocked(now)
		s.mu.Unlock()
		return out
	}
	snap, err := c.fetcher(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		//  overwrite hasnumdata; tgt unknown(K5:  is" has")
		s.meta.Status = StatusUnknown
		s.meta.Note = "L2 refresh failed (does not mean absent): " + err.Error()
		s.meta.FetchedAt = now.Format(time.RFC3339)
		return c.snapshotLocked(now)
	}
	for _, a := range snap.Aliases {
		pk, _ := asr.PinyinKey(a.Alias)
		if a.PinyinKey == "" {
			a.PinyinKey = pk
		}
		if a.FetchedAt == "" {
			a.FetchedAt = now.Format(time.RFC3339)
		}
		replaced := false
		for i := range s.aliases {
			if s.aliases[i].Alias == a.Alias {
				s.aliases[i] = a
				replaced = true
				break
			}
		}
		if !replaced {
			s.aliases = append(s.aliases, a)
		}
	}
	s.meta.Status = StatusOK
	s.meta.Note = ""
	s.meta.FetchedAt = now.Format(time.RFC3339)
	s.meta.Source = snap.Source
	return c.snapshotLocked(now)
}

// Snapshot returnbackcurbeforefast ; edperiodby TTL tgt stale(K4). 
func (c *Cache) Snapshot() Snapshot {
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	return c.snapshotLocked(c.now().UTC())
}

func (c *Cache) snapshotLocked(now time.Time) Snapshot {
	st := c.state()
	out := Snapshot{
		Status: st.meta.Status, Note: st.meta.Note,
		FetchedAt: st.meta.FetchedAt, Source: st.meta.Source,
	}
	for _, h := range st.hot {
		out.Hotwords = append(out.Hotwords, *h)
	}
	out.Aliases = append([]Alias(nil), st.aliases...)
	if out.Status == "" {
		out.Status = StatusOK
	}
	if out.FetchedAt != "" {
		if ts, err := time.Parse(time.RFC3339, out.FetchedAt); err == nil && c.ttl > 0 && now.Sub(ts) > c.ttl {
			out.Status = StatusStale
		}
	}
	return out
}

// Clear  empty  (K6). 
func (c *Cache) Clear() {
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hot = map[string]*Hotword{}
	s.aliases = nil
	s.meta = Snapshot{}
}

// Save pipe L1 fast   (   heavy , K6). 
func (c *Cache) Save() error {
	if c.l1Path == "" {
		return nil
	}
	snap := c.Snapshot()
	if snap.Status == StatusStale {
		snap.Status = StatusOK //    isnumdatabase ; new  by FetchedAt   
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(c.l1Path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return os.WriteFile(c.l1Path, b, 0o600)
}

// Load from L1   (      --         , K6). 
func (c *Cache) Load() error {
	if c.l1Path == "" {
		return nil
	}
	b, err := os.ReadFile(c.l1Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return err
	}
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range snap.Hotwords {
		h := snap.Hotwords[i]
		s.hot[h.Term] = &h
	}
	s.aliases = append([]Alias(nil), snap.Aliases...)
	s.meta.Status = StatusOK
	s.meta.FetchedAt = snap.FetchedAt
	s.meta.Source = snap.Source
	return nil
}
