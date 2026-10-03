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
