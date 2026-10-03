// cache_impl.go —— 三层缓存 + 关联度四路（K2..K6）的实现。
package hotcache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"voicesign-harness/asr"
)

type state struct {
	mu      sync.Mutex
	hot     map[string]*Hotword
	aliases []Alias
	meta    Snapshot // 仅用 Status/FetchedAt/Source/Note
}

// ---- Observe / PutAlias ----

// Observe 记一次热词出现（L0，永不出网）。
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

// PutAlias 登记别名（L0）；同时记录其拼音键以支持近音路由。
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
		c.st = &state{hot: map[string]*Hotword{}}
	}
	return c.st
}

// ---- 关联度四路（K3）----

// Lookup 依次尝试：精确 → 别名 → 拼音近音 → 编辑距离；热度加权；不足则给升级信号（K8）。
func (c *Cache) Lookup(term string) (Result, bool) {
	term = strings.TrimSpace(term)
	if term == "" {
		return Result{NeedEscalate: true}, false
	}
	st := c.Snapshot().Status
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()

	// ① 精确（热词 / 明文 canonical）
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
	// ② 别名
	for _, a := range s.aliases {
		if a.Alias == term {
			return Result{Canonical: a.Canonical, Score: 0.95, Route: RouteAlias, Source: a.Source, Status: st}, true
		}
	}
	// ③ 拼音近音（同一音节序列）
	if key, ok := asr.PinyinKey(term); ok {
		for _, a := range s.aliases {
			if a.PinyinKey != "" && a.PinyinKey == key {
				return Result{Canonical: a.Canonical, Score: 0.85, Route: RoutePinyin, Source: a.Source, Status: st}, true
			}
		}
	}
	// ④ 编辑距离（≤1，且长度差 ≤1）
	best, bestDist := Alias{}, 99
	for _, a := range s.aliases {
		for _, cand := range []string{a.Alias, a.Canonical} {
			if d := levenshtein(term, cand); d < bestDist {
				best, bestDist = a, d
			}
		}
	}
	if bestDist <= 1 {
		return Result{Canonical: best.Canonical, Score: 0.70, Route: RouteEdit, Source: best.Source, Status: st}, true
	}
	// 关联度不足 ⇒ 升级信号（K8），不是"不存在"
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

// ---- 刷新 / 快照 / 失效（K4/K5/K6/K7）----

// Refresh 从 L2 刷新：成功则覆盖别名并落 L1；失败 fail-open 标 unknown（保留本地，不清空）。
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
		// 不覆盖既有数据；标 unknown（K5：不是"没有"）
		s.meta.Status = StatusUnknown
		s.meta.Note = "L2 刷新失败（不是「没有」）：" + err.Error()
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

// Snapshot 返回当前快照；过期按 TTL 标 stale（K4）。
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

// Clear 清空三层（K6）。
func (c *Cache) Clear() {
	s := c.state()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hot = map[string]*Hotword{}
	s.aliases = nil
	s.meta = Snapshot{}
}

// Save 把 L1 快照落盘（可删可重建，K6）。
func (c *Cache) Save() error {
	if c.l1Path == "" {
		return nil
	}
	snap := c.Snapshot()
	if snap.Status == StatusStale {
		snap.Status = StatusOK // 落盘的是数据本身；新鲜度由 FetchedAt 判定
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

// Load 从 L1 恢复（缺失不报错 —— 缺失能降级不崩溃，K6）。
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
