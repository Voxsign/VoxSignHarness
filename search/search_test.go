package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"foo.go":         "package foo\n\nfunc Hello() {}\n\ntype User struct{}\n\nconst Max = 1\n\nvar Name = \"x\"\n",
		"bar.go":         "package bar\n\nfunc Hello() int { return 1 }\n",
		"vendor/skip.go": "package skip\n\nfunc VendorFunc() {}\n",
		"note.txt":       "这里提到 Hello 字样\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFindSymbol_FuncTypeConstVar(t *testing.T) {
	root := writeTree(t)
	hits, err := FindSymbol("Hello", Options{Roots: []string{root}, Ignore: []string{"vendor"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("Hello 应命中 2 处（foo.go + bar.go，vendor 被忽略），实际 %d: %+v", len(hits), hits)
	}
	for _, h := range hits {
		if h.Kind != "func" {
			t.Fatalf("Hello 命中种类应为 func，实际 %q", h.Kind)
		}
		if strings.Contains(h.File, "vendor") {
			t.Fatalf("vendor 应被 ignore 跳过，却命中 %s", h.File)
		}
	}

	// type
	hits, _ = FindSymbol("User", Options{Roots: []string{root}})
	if len(hits) != 1 || hits[0].Kind != "type" {
		t.Fatalf("User 应命中 1 处 type，实际 %+v", hits)
	}
	// const + var
	if hits, _ := FindSymbol("Max", Options{Roots: []string{root}}); len(hits) != 1 || hits[0].Kind != "const" {
		t.Fatalf("Max 应命中 const，实际 %+v", hits)
	}
	if hits, _ := FindSymbol("Name", Options{Roots: []string{root}}); len(hits) != 1 || hits[0].Kind != "var" {
		t.Fatalf("Name 应命中 var，实际 %+v", hits)
	}
}

func TestFindSymbol_IgnoresVendorWhenNotFiltered(t *testing.T) {
	root := writeTree(t)
	// 不传 ignore：vendor/skip.go 也会被扫到（它是 .go）
	hits, _ := FindSymbol("VendorFunc", Options{Roots: []string{root}})
	if len(hits) != 1 {
		t.Fatalf("未过滤 ignore 时应命中 VendorFunc，实际 %+v", hits)
	}
}

func TestFindText_SubstringAndIgnore(t *testing.T) {
	root := writeTree(t)
	hits, err := FindText("Hello", Options{Roots: []string{root}, Ignore: []string{"vendor"}})
	if err != nil {
		t.Fatal(err)
	}
	// foo.go(1 行) + bar.go(1 行) + note.txt(1 行) = 3 处
	if len(hits) != 3 {
		t.Fatalf("Hello 文本应命中 3 处，实际 %d: %+v", len(hits), hits)
	}
	for _, h := range hits {
		if h.Kind != "text" {
			t.Fatalf("文本命中 kind 应为 text，实际 %q", h.Kind)
		}
	}
}

func TestFindEmptyInputs(t *testing.T) {
	if _, err := FindSymbol("", Options{}); err == nil {
		t.Fatal("空符号名应报错")
	}
	if _, err := FindText("", Options{}); err == nil {
		t.Fatal("空模式应报错")
	}
}
