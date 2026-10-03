// VSL-v3 验证标准层的**真实产物**扫描（闭环 G-A 的最后一块）。
//
// 背景（一条真实发现）：`doccontract/verifylint.go` 实现了 V3-01…V3-08 八条规则，
// 但**真实仓库里没有任何定义块使用 `verify:` 字段** —— 唯一的例子在规范文档自己身上，
// 且 verifylint 此前只被它自己的测试调用。
//
// 也就是说，「8/8 通过」当时只说明**没有样本**，不说明标准被遵守。
// 这正是 CICD-BOUNDARY-001:389 那句：
//
//	低流量下"没有错误"可能只是没有样本。观察不足应暂停扩量，不能写成统计验证通过。
//
// 本文件把"空转"本身变成一条**红的断言**：
//   - 扫不到任何真实验证标准块 → 失败（不许把"没样本"当"通过"）
//   - 扫到的每一块 → 必须 8/8 通过
//
// 于是校验器要么有真实输入，要么显式报"没有样本"。
package doccontract

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// verifyBlockRe 抓 verify.md 里的 ```yaml 围栏块。
var verifyBlockRe = regexp.MustCompile("(?s)```ya?ml\\s*\\n(.*?)```")

// realVerifyDocs 返回仓库内所有**真实产物**的验证标准文件（相对模块根）。
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

// TestRealVerificationBlocksExistAndAreLintClean 真实验证标准块必须存在，且 8/8 通过。
//
// 第一条断言（存在性）是**反空转**：它让"没有样本"变成失败，而不是默认通过。
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
