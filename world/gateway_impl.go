// gateway_impl.go -- AIOps  closeread-onlyclientuserend  as now(EXT-05..EXT-08 / A1..A6). 
//
//   needpt: 
//   - **read-only**: only GET;  haswrite  ; Grants()  asempty(A4: read 200 != write limit). 
//   - **fail-open**:     /resolve   allreturnback Status=unknown + Note(origbecause),    panic, 
//       curbecome" has"(A3). 
//   - **onlycache numdata**: cachekeepstoreresolve after  Host/Ledger/Service close ,  keepstoreorigstart  body(A1). 
//   - **  ly **: onlyhas BaseURL   out   (A6). 
package world

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	pathSummary = "/api/summary"
	pathCICD    = "/api/cicd/status"
	pathRoot    = "/"

	maxBody = 4 << 20 //   readonlimit(prevent ity;  modifychange"onlycache numdata")

	// DefaultServiceRegistryPath isbaselyserveservicenote table default  (  numdata ). 
	// read tothen fail-open,      path. 
	DefaultServiceRegistryPath = ".aiops/service-registry-live.json"
)

// fetch send  read-only GET, returnback  bodyand Source(endpoint +  gettimetime). 
func (g *Gateway) fetch(ctx context.Context, path string) ([]byte, Source, error) {
	endpoint := strings.TrimRight(g.BaseURL, "/") + path
	src := Source{Endpoint: endpoint, FetchedAt: g.now().UTC().Format(time.RFC3339)}
	client := g.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, src, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, src, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, src, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, src, err
	}
	return body, src, nil
}

// Summary readget  list(cache  ).    fail-open. 
func (g *Gateway) Summary(ctx context.Context) Summary {
	g.lock.Lock()
	defer g.lock.Unlock()
	if g.cache.summary != nil {
		return *g.cache.summary
	}
	s := g.fetchSummaryLocked(ctx)
	g.cache.summary = &s
	return s
}

func (g *Gateway) fetchSummaryLocked(ctx context.Context) Summary {
	body, src, err := g.fetch(ctx, pathSummary)
	if err != nil {
		return Summary{Status: StatusUnknown, Source: src, Note: "读取失败（不是「没有」）：" + err.Error()}
	}
	var raw struct {
		Ok    *bool                      `json:"ok"`
		Err   string                     `json:"error"`
		TS    string                     `json:"ts"`
		Zone  string                     `json:"zone"`
		Hosts map[string]json.RawMessage `json:"hosts"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Summary{Status: StatusUnknown, Source: src, Note: "解析失败（不是「没有」）：" + err.Error()}
	}
	if raw.Ok != nil && !*raw.Ok {
		return Summary{Status: StatusUnknown, Source: src,
			Note: "网关自报 ok:false（HTTP 200 不等于成功）：" + raw.Err}
	}
	out := Summary{Status: StatusOK, Source: src, Zone: raw.Zone, TS: raw.TS}
	keys := make([]string, 0, len(raw.Hosts))
	for k := range raw.Hosts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var unparsed []string
	for _, k := range keys {
		var h Host
		if err := json.Unmarshal(raw.Hosts[k], &h); err != nil {
			unparsed = append(unparsed, k) //      : underfacekeep as unknown
			continue
		}
		h.Key = k
		out.Hosts = append(out.Hosts, h)
	}
	if len(unparsed) > 0 {
		out.UnparsedHosts = unparsed
		out.Note = "以下主机载荷无法解析，已保留为 unknown（未丢弃）：" + strings.Join(unparsed, ",")
	}
	return out
}

// CICD readget CI/CD   (cache  ).    fail-open. 
func (g *Gateway) CICD(ctx context.Context) CICD {
	g.lock.Lock()
	defer g.lock.Unlock()
	if g.cache.cicd != nil {
		return *g.cache.cicd
	}
	c := g.fetchCICDLocked(ctx)
	g.cache.cicd = &c
	return c
}

func (g *Gateway) fetchCICDLocked(ctx context.Context) CICD {
	body, src, err := g.fetch(ctx, pathCICD)
	if err != nil {
		return CICD{Status: StatusUnknown, Source: src, Note: "读取失败（不是「没有」）：" + err.Error()}
	}
	var raw struct {
		Ok         *bool    `json:"ok"`
		Err        string   `json:"error"`
		Status     string   `json:"status"`
		CurrentTag string   `json:"current_tag"`
		Ledger     []Ledger `json:"ledger"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return CICD{Status: StatusUnknown, Source: src, Note: "解析失败（不是「没有」）：" + err.Error()}
	}
	if raw.Ok != nil && !*raw.Ok {
		return CICD{Status: StatusUnknown, Source: src,
			Note: "网关自报 ok:false（HTTP 200 不等于成功）：" + raw.Err}
	}
	return CICD{Status: StatusOK, Source: src, CurrentTag: raw.CurrentTag, Ledger: raw.Ledger}
}

// Dependencies pipe summary   become" dependency "(EXT-05).   timekeep  unknown  obj(EXT-06). 
func (g *Gateway) Dependencies(ctx context.Context) []Dependency {
	sum := g.Summary(ctx)
	if sum.Status != StatusOK || len(sum.Hosts) == 0 {
		note := sum.Note
		if note == "" {
			note = "网关未返回主机清单"
		}
		return []Dependency{{
			On: g.BaseURL, Kind: "gateway", For: "主机 / 服务 / 状态",
			Status: StatusUnknown, Note: note, Source: sum.Source,
		}}
	}
	out := make([]Dependency, 0, len(sum.Hosts)+1)
	for _, h := range sum.Hosts {
		forWhat := h.Purpose
		if forWhat == "" {
			forWhat = h.Hostname
		}
		out = append(out, Dependency{
			On: h.Key, Kind: "host", For: forWhat, Status: StatusInferred,
			Note: "网关自报，未独立核实", Source: sum.Source,
		})
	}
	for _, k := range sum.UnparsedHosts {
		out = append(out, Dependency{
			On: k, Kind: "host", For: "（载荷无法解析）", Status: StatusUnknown,
			Note: "该主机载荷解析失败，已保留为 unknown（不是「没有」）", Source: sum.Source,
		})
	}
	out = append(out, Dependency{
		On: "aiops 网关（外部世界模型）", Kind: "gateway", For: "机器 / 服务 / 状态的只读真值源",
		Status: StatusOK, Note: "只读接口，不产生写权限", Source: sum.Source,
	})
	return out
}

// Boundaries returnbackand close close   boundary(EXT-07/A4). 
func (g *Gateway) Boundaries(context.Context) []Boundary {
	return []Boundary{
		{ID: "no-deploy", Claim: "本 harness 没有 deploy 工具契约：部署不可执行", Source: "tools/registry.go", Status: "verified"},
		{ID: "gateway-read-only", Claim: "AIOps 读接口 200 不构成写权限：读 ≠ 授权", Source: "world/gateway_impl.go", Status: "verified"},
	}
}

// Grants  asempty:  closereadto   in all produceoccurbase   (A4). 
func (g *Gateway) Grants(context.Context) []string { return nil }

// DiscoverEndpoints read face   outnow  /api/* path(A5:   path). 
func (g *Gateway) DiscoverEndpoints(ctx context.Context) ([]string, error) {
	body, _, err := g.fetch(ctx, pathRoot)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`/api/[A-Za-z0-9_./-]+`)
	set := map[string]bool{}
	for _, m := range re.FindAllString(string(body), -1) {
		m = strings.TrimRight(m, "./-")
		set[m] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// ClearCache  emptycache(A1).  emptyafteragain readget heavy , close  andfirst   . 
func (g *Gateway) ClearCache() {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.cache = cache{}
}

// Services returnbackalready   baselyserveservicenote table(   thenasempty). 
func (g *Gateway) Services() []Service {
	g.lock.Lock()
	defer g.lock.Unlock()
	out := make([]Service, len(g.cache.services))
	copy(out, g.cache.services)
	return out
}

// LoadServiceRegistry readbaselyserveservicenote table(  numdata ). 
// file  / formerror -> returnback error, but** modifychange**  alreadyhascache(fail-open, A3). 
func (g *Gateway) LoadServiceRegistry(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读服务注册表失败（fail-open，不是「没有」）: %w", err)
	}
	//   tail   in : onlyresolve     JSON value. 
	var raw struct {
		Services []Service `json:"services"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&raw); err != nil {
		return fmt.Errorf("服务注册表解析失败（fail-open）: %w", err)
	}
	if len(raw.Services) == 0 {
		return fmt.Errorf("服务注册表里没有 services（fail-open）")
	}
	g.lock.Lock()
	defer g.lock.Unlock()
	g.cache.services = raw.Services
	return nil
}

// DefaultServiceRegistry     default   serveservicenote table;      (fail-open). 
func (g *Gateway) DefaultServiceRegistry() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	if err := g.LoadServiceRegistry(filepath.Join(home, DefaultServiceRegistryPath)); err != nil {
		return false
	}
	return true
}

// WhoHandles answer"      ":    purpose + baselyserveservicenote table aliases   . 
//    totimereturnback Status=unknown   obj -- "   to" etcat" store "(A3). 
func (g *Gateway) WhoHandles(ctx context.Context, query string) []Dependency {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []Dependency{{On: g.BaseURL, Kind: "gateway", For: "该找谁", Status: StatusUnknown, Note: "查询为空"}}
	}
	sum := g.Summary(ctx)
	var out []Dependency
	for _, h := range sum.Hosts {
		if strings.Contains(strings.ToLower(h.Purpose), q) || strings.Contains(strings.ToLower(h.Domain), q) {
			out = append(out, Dependency{
				On: h.Key, Kind: "host", For: h.Purpose, Status: StatusInferred,
				Note: "主机 purpose 命中", Source: sum.Source,
			})
		}
	}
	for _, s := range g.Services() {
		hit := strings.Contains(strings.ToLower(s.Desc), q) || strings.Contains(strings.ToLower(s.Name), q)
		for _, a := range s.Aliases {
			if strings.Contains(strings.ToLower(a), q) {
				hit = true
				break
			}
		}
		if hit {
			out = append(out, Dependency{
				On: s.Name, Kind: "service", For: s.Entry, Status: StatusInferred,
				Note:   "服务注册表命中（本机快照，可能过期）: type=" + s.Type + " status=" + s.Status,
				Source: Source{Endpoint: "service-registry:" + DefaultServiceRegistryPath, FetchedAt: g.now().UTC().Format(time.RFC3339)},
			})
		}
	}
	if len(out) == 0 {
		return []Dependency{{
			On: query, Kind: "gateway", For: "该找谁", Status: StatusUnknown,
			Note:   "未匹配到（这不代表该对象没有）：主机清单 " + fmt.Sprint(len(sum.Hosts)) + " 台、注册表 " + fmt.Sprint(len(g.Services())) + " 条",
			Source: sum.Source,
		}}
	}
	return out
}
