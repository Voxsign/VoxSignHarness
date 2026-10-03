package hotcache

import (
	"fmt"
	"testing"
	"time"
)

// NF1-ALLOC · `LookupForRewrite` 未命中时**不得**做与别名条数成正比的分配。
//
// 依据（Peter `docs/校准报告-产品与实现-L01.md` §6 修订项 1「NF-1 性能」）。
//
// 已坐实的根因（2026-10-03，三条证据链闭合）：
//
//	① `recog/rewriter.go:90 pass1`：每个位置试 **5 个窗口长度** ⇒ **5n 次** `LookupForRewrite`
//	② `hotcache/rewrite_scope.go:50/54`：两个**未命中**分支为取 `.Status` 一个字段
//	   调用 `c.Snapshot()`
//	③ `hotcache/cache_impl.go:299 Snapshot()` → `snapshotLocked`：
//	   `out.Aliases = append([]Alias(nil), st.aliases...)` ⇒ **复制全部别名**
//
// ⇒ **5n 次全量拷贝**（8 字 40 次 ≈ 743ms；32 字 160 次 ≈ 3530ms，均实测吻合）
//
// 本判据**用分配次数**而不是墙钟时间 ⇒ **不 flaky**（时间判据在 CI 上会抖，见 `TestPerformanceP99`）。
// 判定：别名条数 ×10 ⇒ 单次 `LookupForRewrite` 的分配**不得**随之线性增长。
func TestNF1LookupForRewriteAllocsDoNotScaleWithAliases(t *testing.T) {
	measure := func(nAliases int) float64 {
		c := New("", time.Hour, nil) // ← **用构造器**（`&Cache{}` 会缺 now 等字段 ⇒ panic）
		for i := 0; i < nAliases; i++ {
			// 造**不会命中**查询的别名（查询词是"未命中词"）
			c.PutAlias(fmt.Sprintf("alias%05d", i), fmt.Sprintf("canon%05d", i), "test")
		}
		// 查询一个必然未命中的词 ⇒ 走 isGenericForRewrite / isBlacklisted 分支
		const q = "qwertyuiop未命中词"
		return testing.AllocsPerRun(200, func() { _, _ = c.LookupForRewrite(q) })
	}

	small := measure(20)
	large := measure(200) // 别名条数 ×10

	// 若实现里做了与条数成正比的拷贝 ⇒ large 会显著大于 small（约 ×10 或至少 ×2）
	if large > small*2 && large-small > 5 {
		t.Errorf("[NF1-ALLOC] `LookupForRewrite` 的分配随别名条数增长："+
			"20 条 ⇒ %.1f 次分配 · 200 条 ⇒ %.1f 次分配（增 %.1f）"+
			" ⇒ 未命中路径仍在做**与条数成正比的拷贝**（根因：`Snapshot()` 复制 st.aliases）",
			small, large, large-small)
	}
	t.Logf("[NF1-ALLOC] 20 条 ⇒ %.1f 次分配 · 200 条 ⇒ %.1f 次分配（差 %.1f）✅",
		small, large, large-small)
}

// NF1-ALLOC-2 · `Status()` 廉价访问器必须**存在且不复制**（且保留 `state()` 的懒初始化）。
func TestNF1CheapStatusAccessorExists(t *testing.T) {
	c0 := New("", time.Hour, nil)
	// ① 懒初始化副作用必须保留（空 Cache 调 Status 不应 panic）
	got := c0.Status()
	if got == "" {
		got = StatusUnknown // 空 cache 允许返回空或 unknown，但**不得 panic**
	}
	// ② 分配必须 O(1)（与别名条数无关）
	c2 := New("", time.Hour, nil)
	for i := 0; i < 200; i++ {
		c2.PutAlias(fmt.Sprintf("a%05d", i), fmt.Sprintf("c%05d", i), "test")
	}
	allocs := testing.AllocsPerRun(500, func() { _ = c2.Status() })
	if allocs > 4 {
		t.Errorf("[NF1-ALLOC-2] `Status()` 分配 %.1f 次 ⇒ 不是廉价访问器"+
			"（应 O(1)；`Snapshot()` 会复制全部别名）", allocs)
	}
	t.Logf("[NF1-ALLOC-2] `Status()` 分配 %.1f 次 ✅（O(1)）", allocs)
}
