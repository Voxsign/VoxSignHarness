//go:build vhsg1

// g1_metric_test.go -- G1  inrate**baseline  **(first  , after  ize). 
//
//   : ① only  ,     ize; ②  in tgt observed / constructed; 
// ③ classify  (  diffname /    /  in  audio /     )--classify  splithasuse. 
package hotcache

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

type g1Case struct {
	In         string `json:"in"`
	Expect     string `json:"expect"`
	Class      string `json:"class"`
	Provenance string `json:"provenance"` // observed | constructed
}

// observed = base   outnowed change ; constructed =    . 
var g1Cases = []g1Case{
	{"爱ops", "aiops-portal", "alias", "observed"},
	{"研究obc", "research-opc", "alias", "observed"},
	{"沃克body", "workbuddy", "alias", "observed"},
	{"格罗克", "grok-bot", "alias", "observed"},
	{"哈尼斯", "harness", "alias", "observed"},
	{"哎ops", "aiops", "mixed", "observed"},
	{"P图做点com", "peterzou.com", "mixed", "observed"},
	{"哎欧劈艾斯", "aiops", "zh_pinyin", "observed"},
	{"哈你斯", "harness", "zh_pinyin", "constructed"},
	{"voice sound harnessnes", "voice-sign harness", "latin_near", "observed"},
	{"deept", "DeepSeek", "latin_near", "observed"},
}

func g1Cache(t *testing.T) *Cache {
	t.Helper()
	c := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	// serveservicediffname( end value,    remote)
	for alias, canon := range map[string]string{
		"爱ops": "aiops-portal", "研究obc": "research-opc", "沃克body": "workbuddy", "格罗克": "grok-bot",
		"哈尼斯": "harness", "voise sign": "voice-sign",
	} {
		c.PutAlias(alias, canon, "remote:/api/services")
	}
	// rule word(useat  /  classdiff objtgt)
	c.PutAlias("aiops", "aiops", "remote:/api/services")
	c.PutAlias("DeepSeek", "DeepSeek", "remote:/api/services")
	c.PutAlias("peterzou.com", "peterzou.com", "remote:/api/services")
	return c
}

func TestG1BaselineHitRate(t *testing.T) {
	c := g1Cache(t)
	type stat struct{ total, hit int }
	byClass := map[string]*stat{}
	byProv := map[string]*stat{}
	total, hits := 0, 0
	for _, tc := range g1Cases {
		res, ok := c.Lookup(tc.In)
		hit := ok && res.Canonical == tc.Expect
		route := ""
		if ok {
			route = res.Route
		}
		total++
		if hit {
			hits++
		}
		get := func(m map[string]*stat, k string) *stat {
			if m[k] == nil {
				m[k] = &stat{}
			}
			return m[k]
		}
		get(byClass, tc.Class).total++
		get(byProv, tc.Provenance).total++
		if hit {
			get(byClass, tc.Class).hit++
			get(byProv, tc.Provenance).hit++
		}
		b, _ := json.Marshal(map[string]any{
			"in": tc.In, "expect": tc.Expect, "got": res.Canonical, "route": route,
			"class": tc.Class, "provenance": tc.Provenance, "hit": hit,
		})
		fmt.Printf("G1CASE %s\n", b)
	}
	fmt.Printf("G1TOTAL {\"total\":%d,\"hit\":%d,\"rate\":%.3f}\n", total, hits, float64(hits)/float64(total))
	for k, v := range byClass {
		fmt.Printf("G1CLASS {\"class\":%q,\"total\":%d,\"hit\":%d,\"rate\":%.3f}\n", k, v.total, v.hit, float64(v.hit)/float64(v.total))
	}
	for k, v := range byProv {
		fmt.Printf("G1PROV {\"provenance\":%q,\"total\":%d,\"hit\":%d,\"rate\":%.3f}\n", k, v.total, v.hit, float64(v.hit)/float64(v.total))
	}
	if total == 0 {
		t.Fatal("输入集为空")
	}
}
