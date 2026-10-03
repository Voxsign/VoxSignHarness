package hotcache

import (
	"fmt"
	"testing"
	"time"
)

// NF-1 常驻判据 · **线 A（hotcache 热词改写）的耗时**。
//
// ⚠️ 为什么这条必须存在（2026-10-03 覆盖地图）：
//
//	门禁里**原本没有任何耗时判据在看 hotcache** ——
//	唯一的 NF-1 判据是 `nf1_alloc_criteria_test.go` 的 `t.Skip`（已知未修 ⇒ 不拦），
//	而 `asr` 的 p99 判据测的是**线 B 的纯函数**（`go list -deps ./asr/` 证明 asr 不依赖 hotcache）
//	⇒ **NF-1 的 743ms 处于"无人看守"状态**。
//
// 判据内容：**真实调用形态**（`pass1` 用 2–6 字窗口）下，单次 `LookupForRewrite` 的上限。
//
//	· 修前实测：4 字查询 **约 80ms**（237 别名）⇒ **红**
//	· 修后预期：**约 0.6ms** ⇒ 绿
//
// ⚠️ 阈值 5ms 的取法：修前 80ms（超 16 倍）· 修后 0.6ms（余量 8 倍）⇒ 两侧都远离阈值，
//
//	**不落在噪声带里**（这是 `asr` 那条 p99 判据踩过的坑：把阈值放进尾部噪声带 ⇒ 随机报红）。
func TestNF1RewriteLatencyOnRealisticWindows(t *testing.T) {
	const nAlias = 237 // 远端 `/api/services` 实测条数
	c := New("", time.Hour, nil)
	for i := 0; i < nAlias; i++ {
		c.PutAlias(fmt.Sprintf("别名%04d测试", i), fmt.Sprintf("规范%04d名称", i), "test")
	}

	const budget = 5 * time.Millisecond
	// ⚠️ 输入选择是这条判据的**关键**（我第一版选错了）：
	//   · `pass1` 的窗口是 2–6 字 ⇒ 取 4 字，在窗口内，且**不会早退** ⇒ 必走第 ③b 遍
	//   · 而第 ③b 遍对**每条别名**调 `MixedKey`，`MixedKey` 对**每个中文字**调 `asr.PinyinKey`
	//     ⇒ 只有**字都在拼音表内**才会走满 ⇒ 若含表外字（如「的」）会**短路**，测不出问题
	//   ⇒ 故必须用**表内字**：`"输入内容"`（我第一版用 `"四个字的"` ⇒ 「的」表外 ⇒ 只 390µs ⇒ **判据假绿**）
	term := "输入内容"
	_, _ = c.LookupForRewrite(term) // 预热

	const iter = 5
	st := time.Now()
	for i := 0; i < iter; i++ {
		_, _ = c.LookupForRewrite(term)
	}
	per := time.Since(st) / iter

	// 反证用：确认该 term 真的会走满 ③b（否则判据无效）
	t.Logf("[NF1-LAT] ⚠️ 本判据的有效性前提：term 的字**都在拼音表内**（否则 MixedKey 短路 ⇒ 假绿）")
	t.Logf("[NF1-LAT] 别名 %d 条 · 查询 %q（%d 字）⇒ 单次 = **%v**（预算 %v）",
		nAlias, term, len([]rune(term)), per, budget)
	if per > budget {
		t.Fatalf("[NF1-LAT] 单次 %v **超过 %v 预算**（超 %.0f 倍）—— "+
			"热路径上每次 lookup 都这么贵 ⇒ `pass1` 的 5n 次调用会放大成秒级。",
			per, budget, float64(per)/float64(budget))
	}
}
