package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// nofilepath ->  returnbackin defaultword and   . 
func TestLoadDictionaryBuiltinDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dictionary.json") // file store 
	d, err := LoadDictionary(path)
	if err != nil {
		t.Fatalf("LoadDictionary 无文件应返回内置词典而非报错: %v", err)
	}
	if len(d.Terms) != 2 {
		t.Fatalf("builtin dict should have 2 entries, got %d", len(d.Terms))
	}
	for _, term := range d.Terms {
		if term.Source != "builtin" {
			t.Errorf("内置条目 %q 的 source 应为 builtin，实际 %q", term.Term, term.Source)
		}
	}
	//     
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("内置词典不应创建文件，stat=%v", err)
	}
}

func TestCorrectChineseSubstring(t *testing.T) {
	d := builtinDictionary()

	cases := []struct {
		in   string
		want string
		from string
	}{
	}
	for _, c := range cases {
		got, corr := d.Correct(c.in)
		if !strings.Contains(got, c.want) {
			t.Errorf("Correct(%q) = %q, 期望包含 %q", c.in, got, c.want)
		}
		if len(corr) == 0 || corr[0].From != c.from || corr[0].Rule != "dict" {
			t.Errorf("Correct(%q) corrections = %+v, 期望首条 from=%s rule=dict", c.in, corr, c.from)
		}
	}
}

// voxsign as  ASCII changebody,    write   andby token  boundary  . 
func TestCorrectEnglishTokenBoundary(t *testing.T) {
	d := builtinDictionary()

	got, corr := d.Correct("帮我打开 voxsign")
	if !strings.Contains(got, "VoxSign") {
		t.Errorf("Correct(voxsign) = %q, 期望含 VoxSign", got)
	}
	if len(corr) == 0 || corr[0].To != "VoxSign" {
		t.Errorf("corrections = %+v", corr)
	}

	//   write   : VOXSIGN also   
	got2, _ := d.Correct("VOXSIGN 很好")
	if !strings.Contains(got2, "VoxSign") {
		t.Errorf("Correct(VOXSIGN) = %q", got2)
	}

	//  finish  token     : xvoxsigny keepkeep change
	got3, corr3 := d.Correct("xvoxsigny")
	if strings.Contains(got3, "VoxSign") {
		t.Errorf("非 token 边界不应替换: %q", got3)
	}
	if len(corr3) != 0 {
		t.Errorf("非 token 边界不应记录纠错: %+v", corr3)
	}
}

func TestRenderNonEmpty(t *testing.T) {
	d := builtinDictionary()
	r := d.Render()
	if !strings.Contains(r, "VoxSign") || !strings.Contains(r, "center") {
		t.Errorf("Render 应含正确写法与变体: %q", r)
	}
	if !strings.Contains(r, "\n- ") {
		t.Errorf("Render 应一句话一行: %q", r)
	}
}

// AddTerm keep izeafterheavy   see. 
func TestAddTermPersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.json")
	d, err := LoadDictionary(path)
	if err != nil {
		t.Fatalf("LoadDictionary: %v", err)
	}
	before := len(d.Terms)

	err = d.AddTerm(Term{
		Term:     "DeepSeek",
		Variants: []string{"深度求索", "deepseek"},
		Category: "公司",
		Source:   "model",
	})
	if err != nil {
		t.Fatalf("AddTerm: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("AddTerm 应写回文件: %v", err)
	}

	// heavy 
	d2, err := LoadDictionary(path)
	if err != nil {
		t.Fatalf("重载: %v", err)
	}
	if len(d2.Terms) != before+1 {
		t.Fatalf("重载后应有 %d 条，实际 %d", before+1, len(d2.Terms))
	}
	last := d2.Terms[len(d2.Terms)-1]
	if last.Term != "DeepSeek" || last.Source != "model" {
		t.Errorf("新条目未正确持久化: %+v", last)
	}

	// newchangebody  correction
	got, corr := d2.Correct("用深度求索吧")
	if !strings.Contains(got, "DeepSeek") || len(corr) == 0 {
		t.Errorf("新变体未生效: %q %+v", got, corr)
	}
}
