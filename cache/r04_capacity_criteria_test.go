package cache

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// R-04 · 容量上限（判据先红）：`Store` 必须有 **max 条目上限**，超限时**淘汰**。
//
// 依据（Peter `docs/校准报告-产品与实现-L01.md` §6 修订项 2）：
//
//	「R-04 膨胀：只 TTL 过期不够，需 **max 条目/容量上限**（超限淘汰策略）。」
//
// 修前实测：`Set` 只做 upsert，**无任何上限检查**；且 TTL 是**惰性**的（`Get` 时才判过期）
// ⇒ **过期条目也留在 map 里** ⇒ 内存与落盘文件持续增长。
func TestR04MaxEntriesIsEnforced(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "quad.json"), time.Hour) // TTL 很长 ⇒ 排除「过期」这个因素
	if err != nil {
		t.Fatal(err)
	}
	const maxEntries = 10
	st.SetMaxEntries(maxEntries)
	if got := st.MaxEntries(); got != maxEntries {
		t.Fatalf("[R-04] SetMaxEntries(%d) 后 MaxEntries()=%d ⇒ 上限没记上", maxEntries, got)
	}

	// 写 5 倍上限的条目
	for i := 0; i < maxEntries*5; i++ {
		k := QuadKey{Space: "s", Intent: fmt.Sprintf("i%03d", i), Perm: "p", Ref: "r"}
		if err := st.Set(k, "allow"); err != nil {
			t.Fatal(err)
		}
	}

	if n := st.Len(); n > maxEntries {
		t.Errorf("[R-04] 写入 %d 条后 Len()=**%d** > 上限 %d ⇒ **上限未生效**（无界膨胀）",
			maxEntries*5, n, maxEntries)
	}
	if n := st.Len(); n == 0 {
		t.Errorf("[R-04] Len()=0 ⇒ 淘汰把全部条目都清了（不应如此）")
	}
}

// R-04 补充：**过期条目必须被清理**（TTL 惰性 ⇒ 否则过期条目永远占着 map）。
func TestR04ExpiredEntriesAreSwept(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "quad.json"), 20*time.Millisecond) // 极短 TTL
	if err != nil {
		t.Fatal(err)
	}
	st.SetMaxEntries(1000) // 上限设大 ⇒ 只考察「过期清理」
	for i := 0; i < 50; i++ {
		k := QuadKey{Space: "s", Intent: fmt.Sprintf("e%03d", i), Perm: "p", Ref: "r"}
		if err := st.Set(k, "allow"); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(60 * time.Millisecond) // 全部过期
	st.SweepExpired()
	if n := st.Len(); n != 0 {
		t.Errorf("[R-04] 全部过期后 SweepExpired() 仍剩 **%d** 条 ⇒ 过期条目不清理，会无界堆积", n)
	}
}

// R-04 补充：`Set` 触发的自动淘汰**不得**删掉仍在有效期内的条目，除非真的超上限。
func TestR04EvictionPrefersExpired(t *testing.T) {
	dir := t.TempDir()
	// ⚠️ 2026-10-03 修正（**CI 红过一次**：`TestR04EvictionPrefersExpired`）：
	//   原用 **TTL=50ms + sleep 70ms** ⇒ **只留 30ms 余量**，而判据要"塞 30 条"。
	//   而 `Set` **每次都调 `evictLocked()`**，后者**无条件先 `sweepExpiredLocked()`**
	//   ⇒ 若"塞 30 条"在**慢 CI** 上耗时接近 50ms ⇒ **最早塞的 new 条目在检查前已过期、被清掉**
	//     ⇒ `Get(new00)` 失败 ⇒ 判据报"未过期条目被误删"（**而实现其实是对的**）。
	//   ⇒ 改为 **TTL=2s + sleep 2.5s**：塞入耗时（微秒级）与 TTL 差 **5 个数量级** ⇒ 免疫环境速度。
	//   ⚠️ 这是**判据缺陷**，不是实现缺陷 —— R-04 的淘汰语义（先清过期、再按最早 ExpiresAt）正确。
	st, err := Open(filepath.Join(dir, "quad.json"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	st.SetMaxEntries(100)
	// 先塞 30 条会过期的
	for i := 0; i < 30; i++ {
		k := QuadKey{Space: "s", Intent: fmt.Sprintf("old%02d", i), Perm: "p", Ref: "r"}
		_ = st.Set(k, "allow")
	}
	time.Sleep(2500 * time.Millisecond) // 这 30 条过期（TTL=2s ⇒ 余量 500ms，且远大于塞入耗时）
	// 再塞 30 条新的（未超上限 100）
	for i := 0; i < 30; i++ {
		k := QuadKey{Space: "s", Intent: fmt.Sprintf("new%02d", i), Perm: "p", Ref: "r"}
		_ = st.Set(k, "allow")
	}
	// 过期的应被清掉，而新的 30 条必须都在
	if n := st.Len(); n > 30 {
		t.Errorf("[R-04] 过期条目未被清理：Len()=%d（期望 <=30，即只剩新的 30 条）", n)
	}
	for i := 0; i < 30; i++ {
		k := QuadKey{Space: "s", Intent: fmt.Sprintf("new%02d", i), Perm: "p", Ref: "r"}
		if _, ok := st.Get(k); !ok {
			t.Fatalf("[R-04] 未过期的新条目 new%02d 被误删 ⇒ 淘汰策略过激", i)
		}
	}
}
