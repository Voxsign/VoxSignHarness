// kinds_parser_reverse_criteria_test.go -- ⑪a  revexample: **     Kind     be  **. 
//
//   : pipe"newadd  but    "  state connect givesame     get  (  time codeon). 
package trajectory

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// extractKindConsts and  datasame   ( get Kind*    char  value). 
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

// ⑪a revexample: **thus         KindFoo ⇒   be out**
func TestUnregisteredKindConstIsDetected(t *testing.T) {
	// fixture  use**  already   value**(intent_source),  then"posexample"base then become 
	src := "package p\n\nconst (\n\tKindReal = \"" + KindIntentSource + "\"\n\tKindFoo  = \"foo_unregistered\"\n)\n"
	got := extractKindConsts(t, src)
	if _, ok := got["KindFoo"]; !ok {
		t.Fatal("[⑪ 反例] 提取逻辑没抓到 KindFoo（判据会空过）")
	}
	if KnownKind(got["KindFoo"]) {
		t.Error("[⑪ 反例] KindFoo 竟被当作已登记 —— 反例无效")
	}
	// posexample: already    kind   as true
	if !KnownKind(got["KindReal"]) {
		t.Errorf("[⑪] 已登记 kind（%q）被判未登记", got["KindReal"])
	}
}
