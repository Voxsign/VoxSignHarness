//go:build vhsext

// ext_criteria_test.go -- ASR-EXT-005 §3.3     data(first )+ A5      . 
//
//   : go test -tags vhsext ./world
// firstuse**basely  close**(httptest)  data;   close has env    integrate  (seefileend). 
package world

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fakeSummary = `{"ok":true,"ts":"2026-10-03T00:00:00Z","zone":"inner","hosts":{
 "trelva":{"hostname":"VM-4-15-ubuntu","purpose":"trelva（生产中枢：研究引擎/Koyee/peterzou.com）","domain":"trelva.ai","status":"OK","cpu":70.8,"mem":17.0,"disk":84,"disk_free":12.4,"ports":[22,443,8080],"services":36,"env":"prod","role":"research","network":{"openai":"ok"}},
 "center":{"hostname":"VM-0-14-ubuntu","purpose":"center（voxsign.net 生产中心，含 deploy 脚本）","domain":"aiops.example.com","status":"OK","cpu":10.0,"mem":30.0,"disk":50,"disk_free":50,"ports":[22,443],"services":12,"env":"prod","role":"center"}}}`

const fakeCICD = `{"ok":true,"zone":"inner","status":"idle","current_tag":"release/runtime-2026-10-02-01","ledger":[{"ts":"2026-10-02T00:00:00Z","tag":"release/runtime-2026-10-01-01","status":"dry-run","note":"build+gate passed"}]}`

const fakePage = `<html><script>fetch('/api/summary');fetch("/api/cicd/status");fetch('/api/email/config')</script></html>`

// fakeGateway raise       close; fail=true time has /api/* returnback 500. 
type fakeGateway struct {
	srv   *httptest.Server
	hits  int64
	fail  bool
	delay time.Duration
}

func newFakeGateway(t *testing.T, fail bool) *fakeGateway {
	t.Helper()
	fg := &fakeGateway{fail: fail}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&fg.hits, 1)
		if fg.delay > 0 {
			time.Sleep(fg.delay)
		}
		if fg.fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/api/summary":
			_, _ = w.Write([]byte(fakeSummary))
		case "/api/cicd/status":
			_, _ = w.Write([]byte(fakeCICD))
		case "/":
			_, _ = w.Write([]byte(fakePage))
		default:
			_, _ = w.Write([]byte(fakePage)) // SPA  bot : and   close  
		}
	})
	fg.srv = httptest.NewServer(mux)
	t.Cleanup(fg.srv.Close)
	return fg
}

func fixedClock() time.Time { return time.Date(2026, 10, 3, 1, 2, 3, 0, time.UTC) }

func gw(t *testing.T, fg *fakeGateway) *Gateway {
	t.Helper()
	return NewGateway(fg.srv.URL, WithHTTPClient(fg.srv.Client()), WithClock(fixedClock))
}

// EXT-05: summary resolve become time, dependencies       host listtableand    source. 
func TestEXT05SummaryBecomesDependenciesWithSource(t *testing.T) {
	fg := newFakeGateway(t, false)
	deps := gw(t, fg).Dependencies(context.Background())
	if len(deps) == 0 {
		t.Fatal("[EXT-05] 没有产出任何依赖（P1 桩，先红）")
	}
	seen := map[string]Dependency{}
	for _, d := range deps {
		if d.Source.Endpoint == "" || d.Source.FetchedAt == "" {
			t.Errorf("[EXT-05] 依赖 %q 缺 source（端点/时间）: %+v", d.On, d.Source)
		}
		if d.Status == "" {
			t.Errorf("[EXT-05] 依赖 %q 缺 status", d.On)
		}
		seen[d.On] = d
	}
	for _, want := range []string{"trelva", "center"} {
		if _, ok := seen[want]; !ok {
			t.Errorf("[EXT-05] 依赖列表缺少实际主机 %q: %v", want, keys(seen))
		}
	}
}

// EXT-06: endpoint   time  tgt" store ",   tgt unknown(A3). 
func TestEXT06UnreachableIsUnknownNotAbsent(t *testing.T) {
	fg := newFakeGateway(t, true)
	g := gw(t, fg)
	sum := g.Summary(context.Background())
	if sum.Status != StatusUnknown {
		t.Errorf("[EXT-06] 读失败时 Summary.Status=%q，期望 %q（P1 桩为空 = 先红）", sum.Status, StatusUnknown)
	}
	if sum.Note == "" {
		t.Error("[EXT-06] 读失败没有留下原因（Note 为空 = 静默当没有）")
	}
	deps := g.Dependencies(context.Background())
	if len(deps) == 0 {
		t.Fatal("[EXT-06] 读失败后被静默丢弃（依赖列表为空），必须保留 unknown 条目")
	}
	banned := []string{"不存在", "没有这台", "not found", "absent", "nonexistent"}
	blob := ""
	for _, d := range deps {
		if d.Status != StatusUnknown {
			t.Errorf("[EXT-06] 依赖 %q 状态=%q，期望 unknown", d.On, d.Status)
		}
		blob += d.On + d.Note + d.For
	}
	for _, b := range banned {
		if strings.Contains(strings.ToLower(blob), strings.ToLower(b)) {
			t.Errorf("[EXT-06] 把读失败表述成「不存在」（命中 %q）—— 必须说 unknown", b)
		}
	}
}

// EXT-07: readconnect  200   beresolve as"  by  /write"(A4). 
func TestEXT07ReadSuccessIsNotWritePermission(t *testing.T) {
	fg := newFakeGateway(t, false)
	g := gw(t, fg)
	ctx := context.Background()
	if sum := g.Summary(ctx); sum.Status != StatusOK {
		t.Fatalf("[EXT-07] 前提失败：假网关应返回 ok，实际 %q（P1 桩 = 先红）", sum.Status)
	}
	if grants := g.Grants(ctx); len(grants) != 0 {
		t.Errorf("[EXT-07] 读到 200 却产生了权限授予: %v", grants)
	}
	for _, d := range g.Dependencies(ctx) {
		if d.Kind == "capability" || d.Kind == "grant" {
			t.Errorf("[EXT-07] 依赖 %q 的 kind=%q —— 读到的服务被当成了能力/授权", d.On, d.Kind)
		}
	}
	bounds := map[string]Boundary{}
	for _, b := range g.Boundaries(ctx) {
		bounds[b.ID] = b
	}
	for _, want := range []string{"no-deploy", "gateway-read-only"} {
		if _, ok := bounds[want]; !ok {
			t.Errorf("[EXT-07] boundaries 缺少 %q：%v", want, keysB(bounds))
		}
	}
}

// EXT-08: cache   heavy , heavy afterclose   (A1). 
func TestEXT08CacheClearableAndRebuildable(t *testing.T) {
	fg := newFakeGateway(t, false)
	g := gw(t, fg)
	ctx := context.Background()

	first := g.Summary(ctx)
	if first.Status != StatusOK {
		t.Fatalf("[EXT-08] 首次读取失败: %+v（P1 桩 = 先红）", first)
	}
	afterFirst := atomic.LoadInt64(&fg.hits)
	if afterFirst == 0 {
		t.Fatal("[EXT-08] 首次读取没有真的发请求")
	}
	_ = g.Summary(ctx)
	if got := atomic.LoadInt64(&fg.hits); got != afterFirst {
		t.Errorf("[EXT-08] 第二次读取又打了网络（缓存未生效）：hits %d → %d", afterFirst, got)
	}
	g.ClearCache()
	rebuilt := g.Summary(ctx)
	if atomic.LoadInt64(&fg.hits) <= afterFirst {
		t.Error("[EXT-08] ClearCache 后未重新抓取（缓存不可重建）")
	}
	if !reflect.DeepEqual(first, rebuilt) {
		t.Errorf("[EXT-08] 重建结果不一致：\n first=%+v\n rebuilt=%+v", first, rebuilt)
	}
}

// A5:   path -- from face sendnow   /api/* path. 
func TestA5DiscoverEndpointsFromPage(t *testing.T) {
	fg := newFakeGateway(t, false)
	eps, err := gw(t, fg).DiscoverEndpoints(context.Background())
	if err != nil {
		t.Fatalf("[A5] 发现端点失败: %v", err)
	}
	got := strings.Join(eps, ",")
	for _, want := range []string{"/api/summary", "/api/cicd/status"} {
		if !strings.Contains(got, want) {
			t.Errorf("[A5] 未从页面发现 %s（发现结果 %v）", want, eps)
		}
	}
}

//   closeintegrate(default ed; VHS_AIOPS_LIVE=1 only ). 
func TestLiveGatewayOptional(t *testing.T) {
	if os.Getenv("VHS_AIOPS_LIVE") != "1" {
		t.Skip("需要 VHS_AIOPS_LIVE=1 才跑真网关（默认跳过，避免测试依赖外网）")
	}
	base := "https://aiops.example.com"
	g := NewGateway(base)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sum := g.Summary(ctx)
	if sum.Status != StatusOK {
		t.Skipf("真网关不可达（如实记为 unknown，不冒充已验证）: %s", sum.Note)
	}
	t.Logf("live summary: hosts=%d zone=%s", len(sum.Hosts), sum.Zone)
	if len(sum.Hosts) == 0 {
		t.Error("live summary 解析出 0 台主机")
	}
}

func keys(m map[string]Dependency) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keysB(m map[string]Boundary) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// EXT-09: HTTP 200 +   in ok:false   becurbecomebecome (   §1.1    A3-2). 
func TestEXT09PayloadOkFalseIsNotSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/summary":
			fmt.Fprint(w, `{"ok":false,"error":"upstream degraded","hosts":{}}`)
		case "/api/cicd/status":
			fmt.Fprint(w, `{"ok":false,"error":"ledger unavailable"}`)
		default:
			fmt.Fprint(w, fakePage)
		}
	}))
	defer srv.Close()
	g := NewGateway(srv.URL, WithHTTPClient(srv.Client()), WithClock(fixedClock))
	if sum := g.Summary(context.Background()); sum.Status != StatusUnknown || sum.Note == "" {
		t.Errorf("[EXT-09] ok:false 的 summary 被当成成功: %+v", sum)
	}
	if c := g.CICD(context.Background()); c.Status != StatusUnknown || c.Note == "" {
		t.Errorf("[EXT-09] ok:false 的 cicd 被当成成功: %+v", c)
	}
}

// EXT-10:    host resolve     by unknown outnow dependency , and    
// (   §1.1    A3-1:     cur" has"). 
func TestEXT10BadHostNotSilentlyDropped(t *testing.T) {
	payload := `{"ok":true,"zone":"inner","hosts":{"good":{"hostname":"VM-1","purpose":"正常机器"},"broken":{"hostname":12345,"purpose":{"x":1}}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/summary" {
			fmt.Fprint(w, payload)
			return
		}
		fmt.Fprint(w, fakePage)
	}))
	defer srv.Close()
	g := NewGateway(srv.URL, WithHTTPClient(srv.Client()), WithClock(fixedClock))

	dep := g.Dependencies(context.Background())
	found := false
	for _, d := range dep {
		if d.On == "broken" {
			found = true
			if d.Status != StatusUnknown {
				t.Errorf("[EXT-10] 坏 host 的状态=%q，期望 unknown", d.Status)
			}
			if d.Note == "" {
				t.Error("[EXT-10] 坏 host 的依赖没有原因说明")
			}
		}
	}
	if !found {
		t.Errorf("[EXT-10] 坏 host 被静默丢弃，依赖里没有它的 unknown 记录: %+v", dep)
	}
	sum := g.Summary(context.Background())
	if !strings.Contains(sum.Note, "broken") {
		t.Errorf("[EXT-10] Note 未说明少了谁: %q", sum.Note)
	}
}

// EXT-11:  end"voicecalled"       asbase   (A4   **   **). 
//
//   referout Grants()   nil   A4  data      =   safesafety . 
//   giveout  **has   path**    API:    endvoicecalled(i.e. is  store  basely  name)
// all  produceoccur  ;   "    " now     . 
func TestEXT11RemoteClaimNeverGrants(t *testing.T) {
	for _, remote := range []string{"deploy", "http", "admin", "run", "git", "file", "search", "test", "verify", "*"} {
		if local, ok := MapRemoteCapability(remote); ok || local != "" {
			t.Errorf("[EXT-11] 远端声称 %q 被映射为本机授权 %q —— 读 200 ≠ 写权限", remote, local)
		}
	}
	if got := len(NewGateway("http://x").Grants(context.Background())); got != 0 {
		t.Errorf("[EXT-11] Grants 不为空: %d", got)
	}
}
