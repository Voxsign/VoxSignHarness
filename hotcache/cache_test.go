// cache_test.go -- default forbid(no tag):   semanticandclose    pt as. 
package hotcache

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func local(t *testing.T) *Cache { return New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil) }

func TestObserveAndExactLookup(t *testing.T) {
	c := local(t)
	c.Observe("报价单", "user")
	c.Observe("报价单", "user")
	r, ok := c.Lookup("报价单")
	if !ok || r.Route != RouteExact || r.Score < 0.99 {
		t.Fatalf("热词精确命中失败: %+v ok=%v", r, ok)
	}
	if got := len(c.Snapshot().Hotwords); got != 1 {
		t.Fatalf("热词表条数=%d", got)
	}
}

func TestUnknownIsNotAbsent(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, func(context.Context) (Snapshot, error) {
		return Snapshot{}, errors.New("boom")
	})
	snap := c.Refresh(context.Background())
	if snap.Status != StatusUnknown || snap.Note == "" {
		t.Fatalf("刷新失败必须是 unknown+原因: %+v", snap)
	}
	if r, _ := c.Lookup("随便一个词"); r.NeedEscalate != true {
		t.Errorf("未命中应给升级信号而非静默: %+v", r)
	}
}

func TestClearRebuildConsistent(t *testing.T) {
	c := local(t)
	c.PutAlias("爱ops", "aiops", "local")
	r1, _ := c.Lookup("爱ops")
	c.Clear()
	c.PutAlias("爱ops", "aiops", "local")
	r2, _ := c.Lookup("爱ops")
	if r1.Canonical != r2.Canonical || r1.Route != r2.Route {
		t.Fatalf("重建不一致: %+v vs %+v", r1, r2)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	fresh := New(c.l1Path, time.Minute, nil)
	if err := fresh.Load(); err != nil {
		t.Fatal(err)
	}
	if _, ok := fresh.Lookup("爱ops"); !ok {
		t.Error("L1 重载后丢失")
	}
}
