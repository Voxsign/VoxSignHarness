// blacklist_criteria_test.go —— 「这个改错了」动态黑名单判据（§5.1 第 5 件 / A10）。
//
// 默认门禁（无 tag）：本能力**已实现**，判据随默认门禁常跑。
// 钉住三件事：① 黑名单后改写查询被拦；② 路由用途不受影响（两用途分离）；
// ③ 落盘 + 重启恢复（"生效 + 落盘"缺一不可，等价是假设）。
package hotcache

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBlacklistStopsRewriteButKeepsRouting：黑名单只拦改写、不拦路由。
func TestBlacklistStopsRewriteButKeepsRouting(t *testing.T) {
	c := New("", 0, nil)
	c.PutAlias("爱ops", "aiops-portal", SourceUserTaught)

	// 前置：别名本来可以改写。
	if r, ok := c.LookupForRewrite("爱ops"); !ok || r.Canonical != "aiops-portal" {
		t.Fatalf("[BL-01 前置] 别名应可改写，got %+v ok=%v", r, ok)
	}
	if err := c.Blacklist("爱ops", "用户点〔这个改错了〕"); err != nil {
		t.Fatal(err)
	}
	// ① 改写被拦：不再返回可改写结果。
	if r, ok := c.LookupForRewrite("爱ops"); ok {
		t.Errorf("[BL-01] 黑名单后仍可改写：%+v", r)
	}
	// ② 路由不受影响（路由可用全部别名，见 rewrite_scope.go）。
	if r, ok := c.Lookup("爱ops"); !ok || r.Canonical != "aiops-portal" {
		t.Errorf("[BL-02] 黑名单误伤路由：%+v ok=%v", r, ok)
	}
}

// TestBlacklistPersistsAcrossRestart：落盘 + 重启恢复（"生效 + 落盘"）。
func TestBlacklistPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "blacklist.json")
	c := New("", 0, nil)
	c.SetBlacklistPath(p)
	if err := c.Blacklist("哎欧劈艾斯", "A10 验收：把哎欧劈艾斯改错成 aiops"); err != nil {
		t.Fatal(err)
	}
	// ① 落盘：文件真实存在（不是"看起来存在"）。
	if fi, err := os.Stat(p); err != nil || fi.Size() == 0 {
		t.Fatalf("[BL-03] 黑名单未落盘: err=%v size=%v", err, fi)
	}
	// ② 重启等价：同路径新建实例恢复。
	restarted := New("", 0, nil)
	restarted.SetBlacklistPath(p)
	if err := restarted.LoadBlacklist(); err != nil {
		t.Fatal(err)
	}
	bl := restarted.Blacklisted()
	if _, ok := bl["哎欧劈艾斯"]; !ok {
		t.Errorf("[BL-03] 重启后黑名单丢失：%v", bl)
	}
}

// TestBlacklistRejectsEmptyTerm：空词拒绝（否则黑名单变成"什么都能拦"）。
func TestBlacklistRejectsEmptyTerm(t *testing.T) {
	c := New("", 0, nil)
	if err := c.Blacklist("  ", "x"); err == nil {
		t.Errorf("[BL-04] 空词未被拒绝")
	}
}
