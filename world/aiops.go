// Package world —— 外部世界模型：把 AIOps 网关接成「我依赖谁 / 我在哪 / 该找谁」的数据源。
//
// 依据 ASR-EXT-005（实测+指令）：`aiops.peterzou.com` 的 `/api/*` 是**只读**真 API，
// 内网 zone 无需 key。本包只读、不写、不调模型。
//
// P1（判据先于实现）：本文件先定义**形状与桩**，EXT-05..EXT-08 应当为红。
// 实现见 gateway_impl.go。
//
// 硬要求（ASR-EXT-005 §3.2）：
//
//	A1 只缓存元数据，不囤全量响应；缓存可重建可清除
//	A2 每条数据带 source（端点+抓取时间）+ status
//	A3 读失败 fail-open 且标 unknown（不得静默当"没有"）
//	A4 读接口 200 不得当成写权限的证明
//	A5 不猜路径 —— 需要新端点时读页面的 fetch（DiscoverEndpoints）
//	A6 单一出网配置（一个 base URL）
package world

import (
	"context"
	"net/http"
	"time"
)

// 三态（对齐 VHS-PROJMODEL-001 F2）。
const (
	StatusOK       = "ok"
	StatusUnknown  = "unknown"
	StatusInferred = "inferred" // 网关自报，未独立核实
)

// Source 记录每条数据的来源与抓取时间（A2）。
type Source struct {
	Endpoint  string `json:"endpoint"`
	FetchedAt string `json:"fetched_at"`
}

// Host 是一台机器（AIOps `/api/summary` 的 hosts[*]）。
type Host struct {
	Key      string            `json:"key"` // map 键（如 trelva）
	Hostname string            `json:"hostname"`
	Purpose  string            `json:"purpose"`
	Domain   string            `json:"domain"`
	Status   string            `json:"status"`
	CPU      float64           `json:"cpu"`
	Mem      float64           `json:"mem"`
	Disk     float64           `json:"disk"`
	DiskFree float64           `json:"disk_free"`
	Ports    []int             `json:"ports"`
	Services int               `json:"services"`
	Env      string            `json:"env"`
	Role     string            `json:"role"`
	Region   map[string]string `json:"region,omitempty"`
	Network  map[string]string `json:"network,omitempty"`
}

// Summary 是 `/api/summary` 的**元数据快照**（不保存原始响应体，A1）。
type Summary struct {
	Status string `json:"status"` // ok | unknown
	Source Source `json:"source"`
	Zone   string `json:"zone,omitempty"`
	TS     string `json:"ts,omitempty"`
	Hosts  []Host `json:"hosts"`
	Note   string `json:"note,omitempty"` // 失败原因（A3：不得静默）
}

// Ledger 是 CI/CD 台账的一条。
type Ledger struct {
	TS     string `json:"ts"`
	Tag    string `json:"tag"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

// CICD 是 `/api/cicd/status` 的元数据快照。
type CICD struct {
	Status     string   `json:"status"`
	Source     Source   `json:"source"`
	CurrentTag string   `json:"current_tag"`
	Ledger     []Ledger `json:"ledger"`
	Note       string   `json:"note,omitempty"`
}

// Dependency 是「我依赖谁」的一条（可直接并入项目模型 dependencies）。
type Dependency struct {
	On     string `json:"on"`
	Kind   string `json:"kind"` // host | gateway | service（**永不**是 capability/grant，A4）
	For    string `json:"for"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
	Source Source `json:"source"`
}

// Boundary 是「我做不到什么」的一条。
type Boundary struct {
	ID     string `json:"id"`
	Claim  string `json:"claim"`
	Source string `json:"source"`
	Status string `json:"status"`
}

// Service 是本地服务注册表里的一条（可选数据源；缺失时 fail-open unknown）。
type Service struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Entry   string   `json:"entry"`
	Status  string   `json:"status"`
	Desc    string   `json:"desc"`
	Type    string   `json:"type"`
}

// Gateway 是 AIOps 网关的只读客户端。
type Gateway struct {
	BaseURL string
	Client  *http.Client
	now     func() time.Time
	cache   cache
}

// cache 只存**元数据**（A1）；ClearCache 可清空、可重建。
type cache struct {
	summary  *Summary
	cicd     *CICD
	services []Service
}

// NewGateway 构造客户端。baseURL 是**唯一**出网配置（A6）。
func NewGateway(baseURL string, opts ...Option) *Gateway {
	g := &Gateway{BaseURL: baseURL, now: time.Now}
	for _, o := range opts {
		o(g)
	}
	return g
}

// Option 调整客户端（测试用注入，不影响 A6 的单一地址约束）。
type Option func(*Gateway)

// WithHTTPClient 注入 HTTP 客户端（测试用 httptest）。
func WithHTTPClient(c *http.Client) Option { return func(g *Gateway) { g.Client = c } }

// WithClock 注入时钟（让抓取时间可复现）。
func WithClock(fn func() time.Time) Option { return func(g *Gateway) { g.now = fn } }

// Summary 读取主机清单（缓存感知）。失败 fail-open，返回 Status=unknown + Note。
func (g *Gateway) Summary(ctx context.Context) Summary { return Summary{} }

// CICD 读取 CI/CD 台账。失败 fail-open。
func (g *Gateway) CICD(ctx context.Context) CICD { return CICD{} }

// Dependencies 把 summary 映射成「我依赖谁」。失败时必须产出 unknown 条目（A3）。
func (g *Gateway) Dependencies(ctx context.Context) []Dependency { return nil }

// Boundaries 返回与网关相关的硬边界（至少含 no-deploy 与"读≠写"，A4）。
func (g *Gateway) Boundaries(ctx context.Context) []Boundary { return nil }

// Grants 返回"本机因网关读到的内容而获得的能力" —— 恒为空（A4：读不产生权限）。
func (g *Gateway) Grants(ctx context.Context) []string { return nil }

// DiscoverEndpoints 读首页/配置页里真实出现的 /api/* 路径（A5：不猜路径）。
func (g *Gateway) DiscoverEndpoints(ctx context.Context) ([]string, error) { return nil, nil }

// ClearCache 清空缓存（A1）。清空后再次读取应重建出相同结果。
func (g *Gateway) ClearCache() {}

// WhoHandles 回答「这件事该找谁」：在主机 purpose 与本地服务注册表 aliases 里找匹配。
// 找不到时必须返回 Status=unknown 的条目，而不是"没有"（A3）。
func (g *Gateway) WhoHandles(ctx context.Context, query string) []Dependency { return nil }
