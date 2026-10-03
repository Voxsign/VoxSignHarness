//go:build vhsg1

// no_guess_criteria_test.go —— **错配门槛**（Lead 裁决：先治错配，再治未命中）。
//
// 通用性质：若最高候选与次高候选的**分数差 < 阈值** ⇒ **不得匹配**（宁可未命中，不猜）。
// 与 VHS-ZHIJI-001 §2.1「宁可回问，不猜」、JEV 的 ambiguous 是同一个机制。
// 阈值标 UNVALIDATED。
package hotcache

import (
	"path/filepath"
	"testing"
	"time"
)

// ① 具体判据：`哎ops` **不许**再配到 `aiops-portal`。
func TestNoGuessOhOpsNeverMismatches(t *testing.T) {
	c := g1Cache(t)
	res, ok := c.Lookup("哎ops")
	if ok && res.Canonical == "aiops-portal" {
		t.Fatalf("[错配门槛] 仍然错配到 aiops-portal（route=%s）—— 错配比未命中更危险", res.Route)
	}
	// 允许：正确命中 aiops；或明确不匹配（NeedEscalate）
	if ok && res.Canonical != "aiops" {
		t.Errorf("[错配门槛] 命中了一个既非期望也非不匹配的结果: %+v", res)
	}
	if !ok && !res.NeedEscalate {
		t.Errorf("[错配门槛] 未命中时必须给升级信号: %+v", res)
	}
}

// ② 通用性质：两个候选分数接近 ⇒ 不得匹配。
func TestNoGuessTieRefusesToMatch(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	// 构造两个与输入**距离相同**的候选：abc 与 abd（输入 abx，距离各 1）
	c.PutAlias("abc", "canon-abc", "remote")
	c.PutAlias("abd", "canon-abd", "remote")
	res, ok := c.Lookup("abx")
	if ok {
		t.Errorf("[错配门槛] 两个等距候选却给了答案（猜）: %+v", res)
	}
	if !res.NeedEscalate {
		t.Errorf("[错配门槛] 拒绝匹配时必须给升级信号: %+v", res)
	}
}

// ③ 不退化：唯一近邻仍应正常命中（否则门槛把能力也砍掉了）。
func TestNoGuessUniqueNeighborStillMatches(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	c.PutAlias("voice-sign", "voice-sign", "remote")
	res, ok := c.Lookup("voice-signn")
	if !ok || res.Canonical != "voice-sign" {
		t.Fatalf("[错配门槛] 唯一近邻被误杀（门槛过严）: %+v ok=%v", res, ok)
	}
}

// G1-② 混排规范化 + 防治过头。
func TestG1MixedNormalizationAndGuards(t *testing.T) {
	c := g1Cache(t)
	// 正例：哎ops → aiops（混排归一）
	res, ok := c.Lookup("哎ops")
	if !ok || res.Canonical != "aiops" {
		t.Errorf("[G1-②] 混排未归一到 aiops: %+v ok=%v", res, ok)
	}
	// 反例：纯拉丁串不得被误伤（voice-signn 仍走编辑距离命中 voice-sign）
	if r, ok := c.Lookup("voice-signn"); !ok || r.Canonical != "voice-sign" {
		t.Errorf("[G1-② 防过头] 纯拉丁串被误伤: %+v ok=%v", r, ok)
	}
	// 反例：`哎ops` 也不得重新错配到 aiops-portal
	if r, ok := c.Lookup("哎ops"); ok && r.Canonical == "aiops-portal" {
		t.Errorf("[G1-② 防回退] 又错配到 aiops-portal: %+v", r)
	}
}

// 通用判据（Lead 裁决）：**任何归一化，若归一结果对应多个 canonical ⇒ 不得匹配**
// （例外：其中一个 canonical 就是"规范词自身" ⇒ 它优先）。
// 理由："归一"天生多对一，所以"归一后必须唯一"是每条归一化规则的条件，不是某个词的修补。
func TestNormalizationMustBeUniqueOrRefuse(t *testing.T) {
	// ① 多解 ⇒ 拒绝：两个不同 canonical 归一到同一键（爱ops / aiops 同构）
	c := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	c.PutAlias("爱ops", "canon-A", "remote")  // 归一 → aiops
	c.PutAlias("aiops", "canon-B", "remote") // 归一 → aiops（自身）
	res, ok := c.Lookup("哎ops")              // 归一 → aiops
	if ok && res.Canonical != "canon-B" && res.Canonical != "" {
		// 允许：规范词自身（canon-B）优先；否则必须拒绝
		t.Errorf("[归一唯一性] 多解却给了非自身答案（猜）: %+v", res)
	}
	if !ok && !res.NeedEscalate {
		t.Errorf("[归一唯一性] 拒绝匹配时必须给升级信号: %+v", res)
	}

	// ② 例外：规范词自身命中 ⇒ 优先（不是"拒绝"）
	c2 := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	c2.PutAlias("aiops", "aiops", "remote")
	c2.PutAlias("爱ops", "aiops-portal", "remote")
	r2, ok2 := c2.Lookup("哎ops")
	if !ok2 || r2.Canonical != "aiops" {
		t.Errorf("[归一唯一性] 规范词自身优先未生效: %+v ok=%v", r2, ok2)
	}

	// ③ 无歧义 ⇒ 正常命中（防"一律拒绝"的过度保守）
	c3 := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	c3.PutAlias("哎ops", "aiops", "remote")
	r3, ok3 := c3.Lookup("哎ops")
	if !ok3 || r3.Canonical != "aiops" {
		t.Errorf("[归一唯一性] 唯一解被误拒（过度保守）: %+v ok=%v", r3, ok3)
	}
}

// G1-③ 拉丁近似容差 + 两类反例。
func TestG1LatinToleranceAndGuards(t *testing.T) {
	c := g1Cache(t)
	// 反例一：容差放大后 `哎ops` **不得**重新错配（脚本里已断言，这里再钉一次）
	if r, ok := c.Lookup("哎ops"); ok && r.Canonical == "aiops-portal" {
		t.Errorf("[G1-③ 防回退] 又错配到 aiops-portal: %+v", r)
	}
	// 反例二：**不相干的拉丁串不得被匹配**
	if r, ok := c.Lookup("zzzqqqxxyy"); ok {
		t.Errorf("[G1-③ 防治过头] 不相干的拉丁串被匹配: %+v", r)
	}
}
