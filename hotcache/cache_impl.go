// cache_impl.go —— 三层缓存 + 关联度四路（K2..K6）的实现。
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
	meta    Snapshot // 仅用 Status/FetchedAt/Source/Note
	// blacklist 是「用户显式改错」的动态黑名单（term → 登记原因），只拦改写、不拦路由。
	blacklist map[string]string
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
		c.st = &state{hot: map[string]*Hotword{}, blacklist: map[string]string{}}
	}
	return c.st
}

// ---- 关联度四路（K3）----

// Lookup 依次尝试：精确 → 别名 → 拼音近音 → 编辑距离；热度加权；不足则给升级信号（K8）。
func (c *Cache) Lookup(term string) (Result, bool) { return c.lookupInternal(term, false) }

// lookupInternal 是 Lookup 的实现；skipGeneric=true 时跳过通用词别名（改写用途）。
func (c *Cache) lookupInternal(term string, skipGeneric bool) (Result, bool) {
	term = strings.TrimSpace(term)
	if term == "" {
		return Result{NeedEscalate: true}, false
	}
	st := c.Status()
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
		if skipGeneric && isGenericForRewrite(a.Alias) {
			continue
		}
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
	// ③b 混排规范化（G1 第②步）：中文取拼音、拉丁保留（小写），整体归一后比较。
	// **只做整体归一，不做拆词** ⇒ 纯拉丁串不受影响（防"治过头"）。
	// 冲突处理（防"错配"）：若某别名的**规范词自身**归一后等于该键 ⇒ 取它（最强信号）；
	// 否则若**多个不同 canonical** 共享同一键 ⇒ **不得匹配**（不猜）。
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
				canonicalSelf = a.Canonical // 规范词自身就归一成这个键 ⇒ 最强
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
			return Result{NeedEscalate: true, Status: st, Route: RouteMixed}, false // 多解 ⇒ 不猜
		}
	}

	// ④ 编辑距离 + **错配门槛**（Lead 裁决：先治错配，再治未命中）。
	//
	// 通用性质：最高候选与次高候选的**分数差 < 阈值** ⇒ **不得匹配**（宁可未命中，不猜）。
	// 与 VHS-ZHIJI-001 §2.1「宁可回问，不猜」同机制。
	// ⚠️ 两个阈值 **UNVALIDATED**（未标定）。
	const editMaxDistance = 1
	// ③ 拉丁近似：**有界放宽** —— 仅当输入足够长（≥8 字符）才允许 dist≤2。
	// ⚠️ UNVALIDATED：8 / 2 都是未标定的取值。放宽可能带来错配 ⇒ 由差距门槛与
	// "同距不同 canonical ⇒ 拒绝" 兜底。
	const longInputLen = 8
	const editMaxDistanceLong = 2
	const minScoreGap = 2
	// 相对相似度门槛：短串上一字符之差（哎ops vs 爱ops）不足以作为"改写"的证据。
	// ⚠️ UNVALIDATED：0.15 是我取的，未标定。
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
				continue // 别名与规范词相同 ⇒ 不重复计一次
			}
			seen[c] = true
			d := levenshtein(term, c)
			if float64(d) > maxEditRatio*termLen {
				continue // 相对差异过大 ⇒ 不作为候选（防"错配"）
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
			// 差距不足 ⇒ **不猜**：不给答案，交出升级信号
			return Result{NeedEscalate: true, Status: st, Route: RouteEdit}, false
		}
		// 同一距离上出现**不同 canonical** ⇒ 同样不猜
		for _, c := range cands[1:] {
			if c.dist == cands[0].dist && c.canonical != cands[0].canonical {
				return Result{NeedEscalate: true, Status: st, Route: RouteEdit}, false
			}
		}
		return Result{Canonical: cands[0].canonical, Score: 0.70, Route: RouteEdit, Source: cands[0].source, Status: st}, true
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
// Status 是**廉价**的状态读取器（NF-1 性能修复）。
//
// ⚠️ 为什么需要它（2026-10-03 · 三条证据链闭合）：
//
//	`rewrite_scope.go` 的两个**未命中**分支原先写 `c.Snapshot().Status` ——
//	为取**一个字符串字段**，`snapshotLocked` 会 `append([]Alias(nil), st.aliases...)`
//	**复制全部别名**。而 `recog/rewriter.go:90 pass1` 对每个位置试 5 个窗口长度
//	⇒ **5n 次全量拷贝** ⇒ 实测 8 字 743ms / 32 字 3530ms（traces 与长度扫描双证）。
//
// ⚠️ 本函数**保留 `state()` 的懒初始化副作用**（`state()` 会在 `c.st == nil` 时初始化），
//
//	因为那不是"副作用"，而是**既有行为**；换掉调用点时必须保住它。
func (c *Cache) Status() string {
	st := c.state() // ← 保留懒初始化
	st.mu.Lock()
	defer st.mu.Unlock()
	out := st.meta.Status
	if out == "" {
		out = StatusOK
	}
	if st.meta.FetchedAt != "" {
		if ts, err := time.Parse(time.RFC3339, st.meta.FetchedAt); err == nil && c.ttl > 0 &&
			c.now().UTC().Sub(ts) > c.ttl {
			out = StatusStale
		}
	}
	return out
}

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
