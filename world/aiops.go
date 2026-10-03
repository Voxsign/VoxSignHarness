// Package world —— 外部世界模型：把 AIOps 网关接成「我依赖谁 / 我在哪 / 该找谁」的数据源。
//
// 依据 ASR-EXT-005（实测+指令）：`aiops.peterzou.com` 的 `/api/*` 是**只读**真 API，
// 内网 zone 无需 key。本包只读、不写、不调模型。
//
// 硬要求（ASR-EXT-005 §3.2）：
//
//	A1 只缓存元数据，不囤全量响应；缓存可重建可清除
//	A2 每条数据带 source（端点+抓取时间）+ status
//	A3 读失败 fail-open 且标 unknown（不得静默当"没有"）
//	A4 读接口 200 不得当成写权限的证明
//	A5 不猜路径 —— 需要新端点时读页面的 fetch（DiscoverEndpoints）
//	A6 单一出网配置（一个 base URL）
//
// 本文件只放**类型与构造**；行为在 gateway_impl.go。
package world

import (
	"net/http"
	"sync"
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
	// UnparsedHosts 记录**载荷无法解析**的主机 key：它们必须保留为 unknown，
	// 不得静默丢弃（A3：读失败不得当"没有"）。
	UnparsedHosts []string `json:"unparsed_hosts,omitempty"`
	Note          string   `json:"note,omitempty"` // 失败原因（A3：不得静默）
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

// cache 只存**元数据**（A1）；ClearCache 可清空、可重建。
type cache struct {
	summary  *Summary
	cicd     *CICD
	services []Service
}

// Gateway 是 AIOps 网关的只读客户端。
type Gateway struct {
	BaseURL string
	Client  *http.Client
	now     func() time.Time
	lock    sync.Mutex
	cache   cache
}

// Option 调整客户端（测试注入用；不影响 A6 的单一地址约束）。
type Option func(*Gateway)

// WithHTTPClient 注入 HTTP 客户端（测试用 httptest）。
func WithHTTPClient(c *http.Client) Option { return func(g *Gateway) { g.Client = c } }

// WithClock 注入时钟（让抓取时间可复现）。
func WithClock(fn func() time.Time) Option { return func(g *Gateway) { g.now = fn } }

// NewGateway 构造客户端。baseURL 是**唯一**出网配置（A6）。
func NewGateway(baseURL string, opts ...Option) *Gateway {
	g := &Gateway{BaseURL: baseURL, now: time.Now}
	for _, o := range opts {
		o(g)
	}
	return g
}
