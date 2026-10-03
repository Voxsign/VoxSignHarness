//go:build vhscache

// services_live_test.go —— L2 真实刷新**真跑**（只读 /api/services，无需 key）。
// 默认跳过；VHS_CACHE_LIVE=1 才跑。
package hotcache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveServicesRefreshAndVariants(t *testing.T) {
	if os.Getenv("VHS_CACHE_LIVE") != "1" {
		t.Skip("需要 VHS_CACHE_LIVE=1（默认跳过，避免测试依赖外网）")
	}
	endpoint := "https://aiops.peterzou.com/api/services"
	c := New(filepath.Join(t.TempDir(), "cache.json"), time.Hour, HTTPFetcher(endpoint, "", 10*time.Second))
	snap := c.Refresh(context.Background())
	if snap.Status != StatusOK {
		t.Fatalf("真实刷新失败（如实报 unknown）: %+v", snap)
	}
	if len(snap.Aliases) == 0 {
		t.Fatal("别名数为 0 —— 解析可能失效")
	}
	t.Logf("live /api/services: 别名 %d 条，source=%s，fetched_at=%s", len(snap.Aliases), snap.Source, snap.FetchedAt)

	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded := New(c.l1Path, time.Hour, nil)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Lookup("aops"); !ok {
		t.Error("L1 落盘重载后 aops 丢失")
	}

	// 逐个验证 Peter 点名的 ASR 变形词
	for _, v := range []string{"爱ops", "研究obc", "沃克body", "格罗克"} {
		r, ok := c.Lookup(v)
		if !ok {
			t.Logf("变形词 %q: **未命中**（需补别名或拼音覆盖）", v)
			continue
		}
		t.Logf("变形词 %q → %q（route=%s score=%.2f）", v, r.Canonical, r.Route, r.Score)
	}
}
