// kinds_parser_reverse_criteria_test.go —— ⑪a 的反例：**未登记的 Kind 常量必须被抓住**。
//
// 做法：把"新增常量但忘记登记"的形态直接喂给同一套机械提取逻辑（在临时源码上）。
package trajectory

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// extractKindConsts 与主判据同一逻辑（提取 Kind* 常量的字符串值）。
func extractKindConsts(t *testing.T, src string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "inline.go", src, 0)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	out := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if len(name.Name) < 4 || name.Name[:4] != "Kind" || i >= len(vs.Values) {
					continue
				}
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					v, _ := strconv.Unquote(lit.Value)
					out[name.Name] = v
				}
			}
		}
	}
	return out
}

// ⑪a 反例：**故意加一个未登记的 KindFoo ⇒ 必须被检出**
func TestUnregisteredKindConstIsDetected(t *testing.T) {
	// fixture 里用**真实已登记的值**（intent_source），否则"正例"本身就不成立
	src := "package p\n\nconst (\n\tKindReal = \"" + KindIntentSource + "\"\n\tKindFoo  = \"foo_unregistered\"\n)\n"
	got := extractKindConsts(t, src)
	if _, ok := got["KindFoo"]; !ok {
		t.Fatal("[⑪ 反例] 提取逻辑没抓到 KindFoo（判据会空过）")
	}
	if KnownKind(got["KindFoo"]) {
		t.Error("[⑪ 反例] KindFoo 竟被当作已登记 —— 反例无效")
	}
	// 正例：已登记的 kind 必须为 true
	if !KnownKind(got["KindReal"]) {
		t.Errorf("[⑪] 已登记 kind（%q）被判未登记", got["KindReal"])
	}
}
