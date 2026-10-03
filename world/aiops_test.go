// aiops_test.go —— world 包的默认门禁单测（不带 tag，随 `go test ./...` 常跑）。
//
// 判据本体在 ext_criteria_test.go（vhsext）；这里守住单点行为：
// fail-open、unknown 不等于「没有」、只读、不猜路径、注册表容错。
package world

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testGateway(t *testing.T, h http.Handler) *Gateway {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewGateway(srv.URL, WithHTTPClient(srv.Client()), WithClock(func() time.Time {
		return time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	}))
}

func TestSummaryParsesHostsWithoutStoringRaw(t *testing.T) {
	g := testGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"ts":"T","zone":"inner","hosts":{"trelva":{"hostname":"VM","purpose":"生产中枢","ports":[22,443],"services":36}}}`))
	}))
	sum := g.Summary(context.Background())
	if sum.Status != StatusOK || len(sum.Hosts) != 1 || sum.Hosts[0].Key != "trelva" {
		t.Fatalf("解析失败: %+v", sum)
	}
	if sum.Source.Endpoint == "" || sum.Source.FetchedAt == "" {
		t.Fatalf("缺 source: %+v", sum.Source)
	}
	if sum.Zone != "inner" {
		t.Errorf("zone=%q", sum.Zone)
	}
}

func TestSummaryFailOpenOnStatusAndGarbage(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"500":    func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) },
		"垃圾JSON": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not json")) },
		"404":    func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nf", 404) },
	}
	for name, h := range cases {
		g := testGateway(t, h)
		sum := g.Summary(context.Background())
		if sum.Status != StatusUnknown {
			t.Errorf("[%s] status=%q，期望 unknown", name, sum.Status)
		}
		if sum.Note == "" {
			t.Errorf("[%s] 缺失败原因（A3 不得静默）", name)
		}
	}
}

func TestDependenciesFailOpenKeepsUnknownEntry(t *testing.T) {
	g := testGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) }))
	deps := g.Dependencies(context.Background())
	if len(deps) == 0 {
		t.Fatal("读失败后被静默丢弃")
	}
	if deps[0].Status != StatusUnknown {
		t.Errorf("status=%q，期望 unknown", deps[0].Status)
	}
}

// TestWhoHandlesUnknownNotAbsent：匹配不到时必须 unknown，且不得说"不存在"。
func TestWhoHandlesUnknownNotAbsent(t *testing.T) {
	g := testGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"hosts":{"trelva":{"purpose":"生产中枢"}}}`))
	}))
	got := g.WhoHandles(context.Background(), "量子计算机")
	if len(got) == 0 {
		t.Fatal("没有返回条目（应返回 unknown 条目）")
	}
	if got[0].Status != StatusUnknown {
		t.Errorf("status=%q，期望 unknown", got[0].Status)
	}
	for _, banned := range []string{"不存在", "not found", "没有这个"} {
		if strings.Contains(got[0].Note, banned) {
			t.Errorf("把未匹配表述成「%s」: %s", banned, got[0].Note)
		}
	}
	// 命中时应返回主机
	hit := g.WhoHandles(context.Background(), "生产")
	if len(hit) == 0 || hit[0].Kind != "host" {
		t.Errorf("命中主机失败: %+v", hit)
	}
}

// TestServiceRegistryFailOpen：注册表缺失/坏格式不改变已有状态、不 panic。
func TestServiceRegistryFailOpen(t *testing.T) {
	g := testGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"hosts":{}}`))
	}))
	if err := g.LoadServiceRegistry(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("缺失文件应返回 error（fail-open 但要留痕）")
	}
	if len(g.Services()) != 0 {
		t.Errorf("失败后不应有服务: %+v", g.Services())
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := g.LoadServiceRegistry(bad); err == nil {
		t.Error("坏 JSON 应返回 error")
	}
	good := filepath.Join(t.TempDir(), "ok.json")
	body, _ := json.Marshal(map[string]any{"services": []map[string]any{{"name": "cicd", "aliases": []string{"部署", "上线"}, "status": "partial", "type": "internal"}}})
	if err := os.WriteFile(good, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := g.LoadServiceRegistry(good); err != nil {
		t.Fatalf("合法注册表加载失败: %v", err)
	}
	if len(g.Services()) != 1 {
		t.Fatalf("服务数=%d", len(g.Services()))
	}
}

func TestDiscoverEndpoints(t *testing.T) {
	g := testGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<script>fetch('/api/summary'); fetch("/api/cicd/status")</script>`))
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	eps, err := g.DiscoverEndpoints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(eps, ",")
	if !strings.Contains(joined, "/api/summary") || !strings.Contains(joined, "/api/cicd/status") {
		t.Fatalf("发现结果: %v", eps)
	}
}

func TestCacheClearAndRebuildDeterministic(t *testing.T) {
	var hits int
	g := testGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"ok":true,"hosts":{"a":{"purpose":"x"}}}`))
	}))
	first := g.Summary(context.Background())
	_ = g.Summary(context.Background())
	if hits != 1 {
		t.Fatalf("第二次不应重复请求: hits=%d", hits)
	}
	g.ClearCache()
	rebuilt := g.Summary(context.Background())
	if hits != 2 {
		t.Fatalf("ClearCache 后应重新请求: hits=%d", hits)
	}
	if first.Hosts[0].Key != rebuilt.Hosts[0].Key || first.Source != rebuilt.Source {
		t.Fatalf("重建不一致: %+v vs %+v", first, rebuilt)
	}
}
