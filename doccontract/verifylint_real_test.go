// VSL-v3   tgtapprove  **  artifact**  (   G-A   after  ). 
//
//  scenario(    sendnow): `doccontract/verifylint.go`  now V3-01…V3-08   rule, 
// but**      has  define  use `verify:` charseg** -- unique example  rule      on, 
// and verifylint  beforeonlybe      calluse. 
//
// alsothenis , "8/8  ed"curtimeonly  ** haskindbase**,    tgtapprovebe  . 
//  posis CICD-BOUNDARY-001:389  sent: 
//
//	   under" haserror"  onlyis haskindbase.       stop  ,   writebecome     ed. 
//
// basefilepipe"empty "base changebecome  **  disconnectlang**: 
//   -   to      tgtapprove  ->   ( allowpipe" kindbase"cur" ed")
//   -  to     ->    8/8  ed
//
// atisverify need has   in, need  form " haskindbase". 
package doccontract

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// verifyBlockRe   verify.md    ```yaml    . 
var verifyBlockRe = regexp.MustCompile("(?s)```ya?ml\\s*\\n(.*?)```")

// realVerifyDocs returnback  in has**  artifact**   tgtapprovefile( tomoduleroot). 
func realVerifyDocs(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "dist", ".git", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "verify.md" {
			rel, rerr := filepath.Rel(root, path)
			if rerr == nil {
				found = append(found, rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描 verify.md 失败: %v", err)
	}
	return found
}

// TestRealVerificationBlocksExistAndAreLintClean     tgtapprove   store , and 8/8  ed. 
//
//    disconnectlang(store ity)is**revempty **:   " haskindbase"changebecome  , but isdefault ed. 
func TestRealVerificationBlocksExistAndAreLintClean(t *testing.T) {
	root := moduleRoot(t)
	docs := realVerifyDocs(t, root)

	if len(docs) == 0 {
		t.Fatal("校验器空转：仓库里没有任何真实定义块使用 verify: 字段 ——\n" +
			"「8/8 通过」只说明没有样本，不说明标准被遵守（CICD-BOUNDARY-001:389）。\n" +
			"请至少为一份真实产物（如 eval/tasks/<task>/verify.md）写验证标准块。")
	}

	checked := 0
	for _, rel := range docs {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Errorf("读取 %s 失败: %v", rel, err)
			continue
		}
		blocks := verifyBlockRe.FindAllStringSubmatch(string(data), -1)
		if len(blocks) == 0 {
			t.Errorf("%s 里没有 ```yaml 验证标准块 —— 文件名对了但内容不是验证标准", rel)
			continue
		}
		for i, m := range blocks {
			checked++
			report, err := VerifyWith(m[1], Options{Root: root})
			if err != nil {
				t.Errorf("%s 第 %d 块解析失败: %v", rel, i+1, err)
				continue
			}
			if !report.Pass() {
				t.Errorf("%s 第 %d 块未通过 VSL-v3 校验，失败规则: %v\n%s",
					rel, i+1, report.FailedRules(), report.String())
			}
		}
	}
	t.Logf("真实验证标准块：%d 个文件 / %d 个块，全部通过 V3-01…V3-08", len(docs), checked)
}
