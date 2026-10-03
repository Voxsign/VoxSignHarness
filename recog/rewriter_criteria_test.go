//go:build vhsrecog

// rewriter_criteria_test.go —— CACHE-001 K9 三条验收（先红 → 绿）：
//
//	① 含热词的 ASR 变形输入 → 输出**因热词而改变**
//	② 热词表为空时 → 输出与基线一致（变化确来自热词）
//	③ 清空热词表后 → 输出回退
package recog

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"voicesign-harness/asr"
	"voicesign-harness/hotcache"
)

func newRewriter(t *testing.T, withAlias bool) (*Rewriter, *hotcache.Cache) {
	t.Helper()
	c := hotcache.New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	if withAlias {
		c.PutAlias("爱ops", "aiops", "local")
		c.PutAlias("哈尼斯", "harness", "local")
	}
	return &Rewriter{Engine: asr.NewEngine(), Hot: c}, c
}

func TestK9HotwordChangesOutput(t *testing.T) {
	r, _ := newRewriter(t, true)
	got, corrs := r.Correct("把爱ops接上")
	if got != "把aiops接上" {
		t.Fatalf("[K9-①] 热词未改变输出: %q（corrections=%v）", got, corrs)
	}
	if len(corrs) == 0 || corrs[0].Route == "" {
		t.Fatalf("[K9-①] 改写无留痕: %+v", corrs)
	}
}

func TestK9EmptyCacheLeavesOutputUnchanged(t *testing.T) {
	r, _ := newRewriter(t, false)
	got, corrs := r.Correct("把爱ops接上")
	if got != "把爱ops接上" {
		t.Fatalf("[K9-②] 空缓存却改变了输出: %q", got)
	}
	if len(corrs) != 0 {
		t.Fatalf("[K9-②] 空缓存却产生改写: %+v", corrs)
	}
}

func TestK9ClearRevertsOutput(t *testing.T) {
	r, c := newRewriter(t, true)
	if got, _ := r.Correct("把爱ops接上"); got != "把aiops接上" {
		t.Fatalf("[K9-③] 前提失败: %q", got)
	}
	c.Clear()
	got, _ := r.Correct("把爱ops接上")
	if got != "把爱ops接上" {
		t.Fatalf("[K9-③] 清空缓存后输出未回退: %q", got)
	}
	_ = context.Background()
}

// ---- G2：用户临时教的词必须在纠错路径上真的改变输出（照 K9 三条）----

// 正例：教过 ⇒ 输出改变，且来源可审计为 user_taught。
func TestG2TaughtWordChangesOutput(t *testing.T) {
	r, c := newRewriter(t, false)
	if got, _ := r.Correct("把哎欧劈艾斯接上"); got != "把哎欧劈艾斯接上" {
		t.Fatalf("[G2-反例] 未教却改了输出: %q", got)
	}
	if err := c.Teach("哎欧劈艾斯", "aiops"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Correct("把哎欧劈艾斯接上"); got != "把aiops接上" {
		t.Fatalf("[G2-正例] 教过却未改变输出: %q", got)
	}
	taught := c.Taught()
	if len(taught) != 1 || taught[0].Source != hotcache.SourceUserTaught {
		t.Errorf("[G2] 教的词来源不可审计: %+v", taught)
	}
}

// 反例一：空表 ⇒ 输出与基线一致。
func TestG2EmptyTableUnchanged(t *testing.T) {
	r, _ := newRewriter(t, false)
	if got, corrs := r.Correct("把哎欧劈艾斯接上"); got != "把哎欧劈艾斯接上" || len(corrs) != 0 {
		t.Fatalf("[G2-反例一] 空表改变了输出: %q %+v", got, corrs)
	}
}

// 反例二：清空 ⇒ 输出回退（证明变化真的来自教的词）。
func TestG2ClearRevertsTaughtWord(t *testing.T) {
	r, c := newRewriter(t, false)
	if err := c.Teach("哎欧劈艾斯", "aiops"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Correct("把哎欧劈艾斯接上"); got != "把aiops接上" {
		t.Fatalf("[G2-反例二] 前提失败: %q", got)
	}
	c.Clear()
	if got, _ := r.Correct("把哎欧劈艾斯接上"); got != "把哎欧劈艾斯接上" {
		t.Fatalf("[G2-反例二] 清空后未回退: %q", got)
	}
}

// 教的词必须能落 L1 持久（否则"教了下次就忘"——这是 Peter 需求的要点）。
func TestG2TaughtWordPersists(t *testing.T) {
	r, c := newRewriter(t, false)
	if err := c.Teach("沃克body", "workbuddy"); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	fresh := hotcache.New(c.L1Path(), time.Minute, nil)
	if err := fresh.Load(); err != nil {
		t.Fatal(err)
	}
	r2 := &Rewriter{Engine: r.Engine, Hot: fresh}
	if got, _ := r2.Correct("沃克body在哪"); got != "workbuddy在哪" {
		t.Fatalf("[G2] 重载后教的词丢失: %q", got)
	}
}

// 教空词必须被拒（否则缓存被污染成"什么都能命中"）。
func TestG2TeachRejectsEmpty(t *testing.T) {
	_, c := newRewriter(t, false)
	if err := c.Teach("", "aiops"); err == nil {
		t.Error("[G2] 空 term 未被拒")
	}
	if err := c.Teach("x", ""); err == nil {
		t.Error("[G2] 空 canonical 未被拒")
	}
	if err := c.Teach("same", "same"); err == nil {
		t.Error("[G2] 同值未被拒")
	}
}
