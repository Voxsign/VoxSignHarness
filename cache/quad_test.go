package cache

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSetGet_HitAndMiss(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cache.json")
	s, err := Open(p, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	k := QuadKey{Intent: "EDIT", Space: "project:demo", Perm: "write", Ref: "app.go"}
	if _, ok := s.Get(k); ok {
		t.Fatal("空缓存不应命中")
	}
	if err := s.Set(k, "approved_light"); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get(k)
	if !ok || got != "approved_light" {
		t.Fatalf("Set 后应命中，got=%q ok=%v", got, ok)
	}
	// 四元组缺一即 miss
	other := QuadKey{Intent: "EDIT", Space: "project:demo", Perm: "write", Ref: "other.go"}
	if _, ok := s.Get(other); ok {
		t.Fatal("不同 Ref 的四元组不应命中")
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cache.json")
	s1, _ := Open(p, time.Hour)
	k := QuadKey{Intent: "QUERY", Space: "global", Perm: "read", Ref: "x"}
	_ = s1.Set(k, "auto")

	s2, err := Open(p, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get(k)
	if !ok || got != "auto" {
		t.Fatalf("重开后应仍命中，got=%q ok=%v", got, ok)
	}
}

func TestTTLExpiry(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cache.json")
	s, _ := Open(p, 40*time.Millisecond)
	k := QuadKey{Intent: "NOTE", Space: "vault-notes", Perm: "append", Ref: "idea1"}
	_ = s.Set(k, "auto")
	if _, ok := s.Get(k); !ok {
		t.Fatal("刚写入应命中")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := s.Get(k); ok {
		t.Fatal("过期后不应命中")
	}
}

func TestBumpPolicyVersion_AllInvalidate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cache.json")
	s, _ := Open(p, time.Hour)
	k1 := QuadKey{Intent: "EDIT", Space: "a", Perm: "write", Ref: "1"}
	k2 := QuadKey{Intent: "EDIT", Space: "b", Perm: "write", Ref: "2"}
	_ = s.Set(k1, "x")
	_ = s.Set(k2, "y")
	if err := s.BumpPolicyVersion(); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(k1); ok {
		t.Fatal("Bump 后 k1 应失效")
	}
	if _, ok := s.Get(k2); ok {
		t.Fatal("Bump 后 k2 应失效")
	}
	// Bump 后新 Set 应能命中（新版本号）
	_ = s.Set(k1, "z")
	if got, ok := s.Get(k1); !ok || got != "z" {
		t.Fatalf("Bump 后新写入应命中，got=%q ok=%v", got, ok)
	}
}

func TestInvalidateSpace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cache.json")
	s, _ := Open(p, time.Hour)
	ka := QuadKey{Intent: "EDIT", Space: "project:a", Perm: "write", Ref: "1"}
	kb := QuadKey{Intent: "EDIT", Space: "project:b", Perm: "write", Ref: "2"}
	_ = s.Set(ka, "x")
	_ = s.Set(kb, "y")
	if err := s.InvalidateSpace("project:a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(ka); ok {
		t.Fatal("project:a 应被作废")
	}
	if _, ok := s.Get(kb); !ok {
		t.Fatal("project:b 不应受影响")
	}
}
