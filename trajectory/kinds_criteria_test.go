// kinds_criteria_test.go —— 判据⑪：kind 单一枚举 + 未登记必须报错（防"假绿空过"）。
package trajectory

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ⑪a **Kinds 必须覆盖源码里所有 kind 常量** —— 用 go/parser 机械提取，**不用手写列表**。
//
// 理由（Lead 2026-10-03）：手写列表 = 第二个真相源，**有人新增常量而忘记登记时它会静默过期，
// 而判据仍然绿**。这与"一个事实两处维护"同源。
func TestKindSingleSourceOfTruth(t *testing.T) {
	// **整包解析**（不只 kinds.go）：常量可能声明在同包其它文件里（如 trajectory.go）。
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("[⑪] 找不到包内 .go 文件: %v", err)
	}
	seen := 0
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue // 判据自身不参与（避免自指）
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatalf("[⑪] 解析 %s 失败: %v", path, perr)
		}
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
					if !strings.HasPrefix(name.Name, "Kind") {
						continue // 只看 kind 常量（Kind*）
					}
					if i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					val, _ := strconv.Unquote(lit.Value)
					seen++
					if !KnownKind(val) {
						t.Errorf("[⑪] 常量 %s=%q 未在 Kinds 中登记（新增常量忘记登记 ⇒ 判据必须红）",
							name.Name, val)
					}
				}
			}
		}
	}
	if seen < 11 {
		t.Errorf("[⑪] 只解析到 %d 个 kind 常量（少于 11）—— 判据可能没真正覆盖源码", seen)
	}
	if KindIntentSource != "intent_source" {
		t.Errorf("[⑪] KindIntentSource 值应为 intent_source，实际 %q", KindIntentSource)
	}
}

// ⑪b 未登记 kind ⇒ Write **必须报错**（不许静默写入）
func TestUnknownKindIsRejectedOnWrite(t *testing.T) {
	tr, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()
	if err := tr.Write(Entry{RequestID: "r1", Kind: "no_such_kind_xyz"}); err == nil {
		t.Error("[⑪] 未登记 kind 竟然写入成功（判据会静默空过）")
	}
	// 反例：已登记 kind ⇒ 必须成功
	if err := tr.Write(Entry{RequestID: "r1", Kind: KindIntentSource, Content: IntentSourceASR}); err != nil {
		t.Errorf("[⑪ 反例] 已登记 kind 被拒: %v", err)
	}
}

// ⑪c intent_source 值域必须是枚举（不许自由文本）
func TestIntentSourceValueDomain(t *testing.T) {
	if err := Validate(Entry{Kind: KindIntentSource, Content: "随便写的"}); err == nil {
		t.Error("[⑪] intent_source 接受了值域外的自由文本")
	}
	for _, v := range []string{IntentSourceASR, IntentSourceTextFallback} {
		if err := Validate(Entry{Kind: KindIntentSource, Content: v}); err != nil {
			t.Errorf("[⑪] 合法值 %q 被拒: %v", v, err)
		}
	}
}

// ⑪d 空 kind ⇒ 报错（不许"没有 kind 就当默认"）
func TestEmptyKindIsRejected(t *testing.T) {
	if err := Validate(Entry{}); err == nil {
		t.Error("[⑪] 空 kind 未报错")
	}
}
