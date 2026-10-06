// Package doccontract is L0"    ity" :   **rule        ,  code is   has**. 
//
//  data: docs/VSL-  -  opensendrule  origthen.md:19"rule  obj = voice  +    disconnectlang; no     obj =  done". 
// this packagepipe sent changebecome      , but is id. 
//
// only "rule ity  "--onlyhasrule /frozen/    only **  **   name; 
//   class,   class   outnow  TestXxx onlyis  ,   become  . 
package doccontract

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// normativeDocs is"      name" rule ity  ( tomoduleroot). 
var normativeDocs = []string{
	"docs/SPEC-v1-可执行规格书.md",
	"docs/SPEC-v2-可执行规格书.md",
	"docs/INTERFACE-FREEZE-M2.md",
	"docs/SSE-v1-事件流契约.md",
	"docs/VSL-v1-描述定义语言规范.md",
	"docs/VSL-v2-判断语义层.md",
	"docs/VSL-大域定义-Harness三域.md",
	"docs/VSL-共识-未来开发规范五原则.md",
}

var (
	// testFuncRe   code   store     numname. 
	testFuncRe = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)`)
	// pinnedRe     be use    name(  write  TestFoo_*  be become TestFoo_). 
	pinnedRe = regexp.MustCompile(`Test[A-Za-z][A-Za-z0-9_]*`)
)

// moduleRoot returnbackmodulerootobj : go test by obj as  obj , thusrootobj isits obj . 
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("向上找不到 go.mod，模块根判定失败: %v", err)
	}
	return root
}

// collectTestFuncs   safety  *Test  numname( ed dist/ artifactobj ). 
func collectTestFuncs(t *testing.T, root string) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "dist" || name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, m := range testFuncRe.FindAllStringSubmatch(string(data), -1) {
			got[m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描测试函数失败: %v", err)
	}
	return got
}

// collectPinned fromrule ity        use, returnback name -> outplacelisttable. 
func collectPinned(t *testing.T, root string) map[string][]string {
	t.Helper()
	pinned := map[string][]string{}
	for _, rel := range normativeDocs {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("规范性文档缺失 %s: %v（若已改名，请同步本测试的 normativeDocs）", rel, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range pinnedRe.FindAllString(line, -1) {
				loc := rel + ":" + itoa(i+1)
				pinned[m] = appendUnique(pinned[m], loc)
			}
		}
	}
	return pinned
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func appendUnique(xs []string, v string) []string {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}

// satisfied      nameis befull :    in, or asbefore  in(    write  TestFoo_*). 
func satisfied(pinned string, actual map[string]bool) bool {
	if actual[pinned] {
		return true
	}
	for name := range actual {
		if strings.HasPrefix(name, pinned) {
			return true
		}
	}
	return false
}

// TestNormativeDocsPinnedValidatorsExist rule           ,  code   store . 
//
//     is"no     obj =  done":   write   namebut  now, orer nowbemodifyname/delete, 
// all      , but isetcto dayhas sendnow"   its   ". 
func TestNormativeDocsPinnedValidatorsExist(t *testing.T) {
	root := moduleRoot(t)
	actual := collectTestFuncs(t, root)
	pinned := collectPinned(t, root)

	if len(pinned) == 0 {
		t.Fatal("没有从规范文档里抽出任何验证器名——正则或文档结构可能已变，请检查本测试")
	}

	var missing []string
	for name, locs := range pinned {
		if !satisfied(name, actual) {
			missing = append(missing, name+"  ←  "+strings.Join(locs, ", "))
		}
	}
	sort.Strings(missing)

	t.Logf("规范性文档钉死验证器 %d 个；仓库现有测试函数 %d 个", len(pinned), len(actual))
	if len(missing) > 0 {
		t.Errorf("以下验证器被规范文档钉死，但代码里找不到（%d 个）：\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// TestNormativeDocsArePresent rule   base   store and empty. 
// preventstop"  be /bemodifyname"  onface     changebecomeempty (  ). 
func TestNormativeDocsArePresent(t *testing.T) {
	root := moduleRoot(t)
	for _, rel := range normativeDocs {
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			t.Errorf("规范性文档不存在: %s (%v)", rel, err)
			continue
		}
		if info.Size() < 200 {
			t.Errorf("规范性文档内容过少（%d 字节），可能被清空: %s", info.Size(), rel)
		}
	}
}
