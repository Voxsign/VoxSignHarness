// blacklist_criteria_test.go -- "  modify " state name  data(§5.1   5   / A10). 
//
// default forbid(no tag): base  **already now**,  data default forbid  . 
//      : ①  name aftermodifywrite  be ; ② routebyuseway accept  ( usewaysplit ); 
// ③    + heavystart  ("occur  +   "    , etc is  ). 
package hotcache

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBlacklistStopsRewriteButKeepsRouting:  name only modifywrite,   routeby. 
func TestBlacklistStopsRewriteButKeepsRouting(t *testing.T) {
	c := New("", 0, nil)
	c.PutAlias("爱ops", "aiops-portal", SourceUserTaught)

	// before : diffnamebase  bymodifywrite. 
	if r, ok := c.LookupForRewrite("爱ops"); !ok || r.Canonical != "aiops-portal" {
		t.Fatalf("[BL-01 前置] 别名应可改写，got %+v ok=%v", r, ok)
	}
	if err := c.Blacklist("爱ops", "用户点〔这个改错了〕"); err != nil {
		t.Fatal(err)
	}
	// ① modifywritebe :  againreturnback modifywriteclose . 
	if r, ok := c.LookupForRewrite("爱ops"); ok {
		t.Errorf("[BL-01] 黑名单后仍可改写：%+v", r)
	}
	// ② routeby accept  (routeby usesafety diffname, see rewrite_scope.go). 
	if r, ok := c.Lookup("爱ops"); !ok || r.Canonical != "aiops-portal" {
		t.Errorf("[BL-02] 黑名单误伤路由：%+v ok=%v", r, ok)
	}
}

// TestBlacklistPersistsAcrossRestart:    + heavystart  ("occur  +   "). 
func TestBlacklistPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "blacklist.json")
	c := New("", 0, nil)
	c.SetBlacklistPath(p)
	if err := c.Blacklist("哎欧劈艾斯", "A10 验收：把哎欧劈艾斯改错成 aiops"); err != nil {
		t.Fatal(err)
	}
	// ①   : file  store ( is" raise store "). 
	if fi, err := os.Stat(p); err != nil || fi.Size() == 0 {
		t.Fatalf("[BL-03] 黑名单未落盘: err=%v size=%v", err, fi)
	}
	// ② heavystartetc : samepathnew  example  . 
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

// TestBlacklistRejectsEmptyTerm: emptywordreject( then name changebecome"  all  "). 
func TestBlacklistRejectsEmptyTerm(t *testing.T) {
	c := New("", 0, nil)
	if err := c.Blacklist("  ", "x"); err == nil {
		t.Errorf("[BL-04] 空词未被拒绝")
	}
}
