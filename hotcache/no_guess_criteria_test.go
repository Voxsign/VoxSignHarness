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
