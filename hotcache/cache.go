// Package hotcache —— 缓存即小模型（VHS-CACHE-001）：三层状态 + 关联度。
//
// P1：本文件先给**形状与桩**，判据 K2..K6 应当全红。实现见 cache_impl.go。
//
// 三层（K1）：
//
//	L0 进程内   热词表 / 当前会话别名             —— 纳秒级，绝不出网
//	L1 本机持久 别名路由副本 / 能力清单快照        —— 微秒级，JSON 文件
//	L2 远端     /api/services 等只读真值          —— 仅在刷新时触碰
package hotcache

import (
	"context"
	"time"
)

// 三态（K5）。
const (
	StatusOK      = "ok"
	StatusStale   = "stale"
	StatusUnknown = "unknown"
)

// Route 是关联度命中的路径（K3 四路）。
const (
	RouteExact  = "exact"
	RouteAlias  = "alias"
	RoutePinyin = "pinyin"
	RouteEdit   = "edit"
)

// Hotword 是热词表的一条（K9：Peter 点名"最近说的词要记得住"）。
type Hotword struct {
	Term      string `json:"term"`
	Heat      int    `json:"heat"`
	LastSeen  string `json:"last_seen"`
	Source    string `json:"source"`
	FetchedAt string `json:"fetched_at"`
}

// Alias 是别名路由的一条（含 ASR 变形词）。
type Alias struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
	PinyinKey string `json:"pinyin_key,omitempty"` // 近音路由用
	Source    string `json:"source"`
	FetchedAt string `json:"fetched_at"`
}

// Snapshot 是 L1 持久快照（可删可重建，K6）。
type Snapshot struct {
	Hotwords  []Hotword `json:"hotwords"`
	Aliases   []Alias   `json:"aliases"`
	FetchedAt string    `json:"fetched_at"`
	Source    string    `json:"source"`
	Status    string    `json:"status"` // ok | stale | unknown
	Note      string    `json:"note,omitempty"`
}

// Result 是一次关联度查询结果。
type Result struct {
	Canonical    string  `json:"canonical"`
	Score        float64 `json:"score"`
	Route        string  `json:"route"`
	Source       string  `json:"source"`
	Status       string  `json:"status"`        // ok | stale | unknown（过期不静默使用，K4）
	NeedEscalate bool    `json:"need_escalate"` // 关联度不足 ⇒ 升级信号（K8）
}

// Cache 是三层缓存。
type Cache struct {
	l0      Snapshot
	l1Path  string
	ttl     time.Duration
	fetcher func(ctx context.Context) (Snapshot, error) // L2 远端刷新
	now     func() time.Time
}

// New 构造缓存；l1Path 为空则不落盘；fetcher 为空则无远端（纯本地）。
func New(l1Path string, ttl time.Duration, fetcher func(ctx context.Context) (Snapshot, error)) *Cache {
	return &Cache{l1Path: l1Path, ttl: ttl, fetcher: fetcher, now: time.Now}
}

// Observe 记一次热词出现（L0）。
func (c *Cache) Observe(term, source string) {}

// PutAlias 登记别名（L0；Refresh 时来自 L2 的会落 L1）。
func (c *Cache) PutAlias(alias, canonical, source string) {}

// Lookup 做四路关联度匹配（K3）。
func (c *Cache) Lookup(term string) (Result, bool) { return Result{}, false }

// Refresh 从 L2 刷新并落 L1；失败 fail-open 标 unknown（K5）。
func (c *Cache) Refresh(ctx context.Context) Snapshot { return Snapshot{} }

// Snapshot 返回当前快照（含三态）。
func (c *Cache) Snapshot() Snapshot { return Snapshot{} }

// Clear 清空三层（K6）。
func (c *Cache) Clear() {}

// Save/Load 是 L1 持久化（K6：可删可重建）。
func (c *Cache) Save() error { return nil }
func (c *Cache) Load() error { return nil }
