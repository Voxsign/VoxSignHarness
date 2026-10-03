//go:build vhsext

// ext_criteria_test.go —— ASR-EXT-005 §3.3 的四条判据（先红）+ A5 的机械验证。
//
// 运行：go test -tags vhsext ./world
// 先用**本地假网关**（httptest）跑判据；真网关另有 env 守卫的集成测试（见文件末）。
package world

import (
	"context"
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
 "center":{"hostname":"VM-0-14-ubuntu","purpose":"center（voxsign.net 生产中心，含 deploy 脚本）","domain":"aiops.peterzou.com","status":"OK","cpu":10.0,"mem":30.0,"disk":50,"disk_free":50,"ports":[22,443],"services":12,"env":"prod","role":"center"}}}`

const fakeCICD = `{"ok":true,"zone":"inner","status":"idle","current_tag":"release/runtime-2026-10-02-01","ledger":[{"ts":"2026-10-02T00:00:00Z","tag":"release/runtime-2026-10-01-01","status":"dry-run","note":"build+gate passed"}]}`

const fakePage = `<html><script>fetch('/api/summary');fetch("/api/cicd/status");fetch('/api/email/config')</script></html>`

// fakeGateway 起一个可控的假网关；fail=true 时所有 /api/* 返回 500。
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
			_, _ = w.Write([]byte(fakePage)) // SPA 兜底页：与真实网关一致
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

// EXT-05：summary 解析成功时，dependencies 必须含实际 host 列表且每条带 source。
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

// EXT-06：端点不可达时不得标"不存在"，必须标 unknown（A3）。
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

// EXT-07：读接口 200 不得被解释为"我可以部署/写"（A4）。
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

// EXT-08：缓存可删可重建，重建后结果一致（A1）。
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

// A5：不猜路径 —— 从页面里发现真实 /api/* 路径。
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

// 真网关集成（默认跳过；VHS_AIOPS_LIVE=1 才跑）。
func TestLiveGatewayOptional(t *testing.T) {
	if os.Getenv("VHS_AIOPS_LIVE") != "1" {
		t.Skip("需要 VHS_AIOPS_LIVE=1 才跑真网关（默认跳过，避免测试依赖外网）")
	}
	base := "https://aiops.peterzou.com"
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
