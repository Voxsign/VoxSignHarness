//go:build vhscache

// cache_criteria_test.go —— VHS-CACHE-001 K2..K6（先红）。
//
// 运行：go test -tags vhscache ./hotcache
package hotcache

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// fetcher 计数：证明"能本地答的绝不外调"（K2）。
func newTestCache(t *testing.T, fetchOK bool, calls *int) *Cache {
	t.Helper()
	fetcher := func(ctx context.Context) (Snapshot, error) {
		*calls++
		if !fetchOK {
			return Snapshot{}, errors.New("远端不可达")
		}
		return Snapshot{
			Status: StatusOK, Source: "remote:/api/services", FetchedAt: time.Now().UTC().Format(time.RFC3339),
			Aliases: []Alias{
				{Alias: "爱ops", Canonical: "aiops", Source: "remote"},
				{Alias: "哈尼斯", Canonical: "harness", Source: "remote"},
				{Alias: "汉尼斯", Canonical: "harness", Source: "remote"},
				{Alias: "voise sign", Canonical: "voice-sign", Source: "remote"},
			},
		}, nil
	}
	return New(filepath.Join(t.TempDir(), "cache.json"), time.Minute, fetcher)
}

// K2：热词/别名能本地答，绝不外调。
func TestK2LocalAnswersNeverCallRemote(t *testing.T) {
	calls := 0
	c := newTestCache(t, true, &calls)
	c.Refresh(context.Background()) // 一次刷新（L2）
	base := calls
	c.Observe("报价单", "user")
	if r, ok := c.Lookup("报价单"); !ok || r.Route != RouteExact {
		t.Errorf("[K2] 热词未本地命中: %+v ok=%v", r, ok)
	}
	if calls != base {
		t.Errorf("[K2] 本地查询触发了远端调用: %d → %d", base, calls)
	}
}

// K3：关联度四路各有用例（精确/别名/拼音近音/编辑距离）+ 热度加权。
func TestK3FourRelevanceRoutes(t *testing.T) {
	calls := 0
	c := newTestCache(t, true, &calls)
	c.Observe("报价单", "user")
	c.PutAlias("爱ops", "aiops", "local")
	c.PutAlias("哈尼斯", "harness", "local")
	c.PutAlias("voise sign", "voice-sign", "local")

	cases := []struct {
		in, want, route string
	}{
		{"报价单", "报价单", RouteExact},
		{"爱ops", "aiops", RouteAlias},
		{"哈你斯", "harness", RoutePinyin}, // 尼/你 同音（ha ni si）
		{"voice-sign", "voice-sign", RouteExact},
		{"voice-signn", "voice-sign", RouteEdit},
	}
	for _, tc := range cases {
		got, ok := c.Lookup(tc.in)
		if !ok {
			t.Errorf("[K3] %q 未命中任何一路", tc.in)
			continue
		}
		if got.Canonical != tc.want {
			t.Errorf("[K3] %q → %q，期望 %q（route=%s）", tc.in, got.Canonical, tc.want, got.Route)
		}
		if got.Route != tc.route {
			t.Errorf("[K3] %q 命中路径=%s，期望 %s", tc.in, got.Route, tc.route)
		}
	}
	// 热度加权：同分时热度高的优先
	c.Observe("报价单", "user")
	c.Observe("报价单", "user")
	r, _ := c.Lookup("报价单")
	if r.Score <= 0 {
		t.Errorf("[K3] 热度加权后分数应 > 0: %+v", r)
	}
}

// K4：每条带 fetched_at/source；过期标 stale，**不静默当新鲜**。
func TestK4StaleIsVisibleNotSilent(t *testing.T) {
	calls := 0
	c := newTestCache(t, true, &calls)
	c.now = func() time.Time { return time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC) }
	c.Refresh(context.Background())
	c.now = func() time.Time { return time.Date(2026, 10, 3, 2, 0, 0, 0, time.UTC) } // 2h 后，TTL=1m
	snap := c.Snapshot()
	if snap.Status != StatusStale {
		t.Errorf("[K4] 过期未标 stale: %+v", snap.Status)
	}
	r, ok := c.Lookup("爱ops")
	if !ok || r.Status != StatusStale {
		t.Errorf("[K4] 过期命中未把 stale 透出: %+v ok=%v", r, ok)
	}
}

// K5：刷新失败 fail-open 标 unknown，不得当"没有"。
func TestK5RefreshFailureIsUnknownNotAbsent(t *testing.T) {
	calls := 0
	c := newTestCache(t, false, &calls)
	snap := c.Refresh(context.Background())
	if snap.Status != StatusUnknown || snap.Note == "" {
		t.Errorf("[K5] 刷新失败未标 unknown+原因: %+v", snap)
	}
	// 不得把"远端失败"表达成"别名不存在"
	if _, ok := c.Lookup("爱ops"); ok {
		t.Log("本地确实无此别名（允许），但状态必须未知")
	}
	if got := c.Snapshot().Status; got != StatusUnknown {
		t.Errorf("[K5] 快照状态=%q，期望 unknown", got)
	}
}

// K6：缓存可删可重建，重建一致。
func TestK6ClearAndRebuildConsistent(t *testing.T) {
	calls := 0
	c := newTestCache(t, true, &calls)
	first := c.Refresh(context.Background())
	r1, _ := c.Lookup("汉尼斯")
	c.Clear()
	if got := c.Snapshot(); len(got.Aliases) != 0 {
		t.Errorf("[K6] Clear 后仍有别名: %+v", got.Aliases)
	}
	second := c.Refresh(context.Background())
	r2, _ := c.Lookup("汉尼斯")
	if first.Status != second.Status || !reflect.DeepEqual(r1, r2) {
		t.Errorf("[K6] 重建不一致: %+v/%+v vs %+v/%+v", first.Status, r1, second.Status, r2)
	}
	// L1 落盘后可重新加载
	if err := c.Save(); err != nil {
		t.Errorf("[K6] Save: %v", err)
	}
	fresh := New(c.l1Path, time.Minute, nil)
	if err := fresh.Load(); err != nil {
		t.Fatalf("[K6] Load: %v", err)
	}
	if _, ok := fresh.Lookup("汉尼斯"); !ok {
		t.Error("[K6] L1 重载后别名丢失（缓存不可重建）")
	}
}
