package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useexample 10: modifyfinish    -> read  close ,  read  . 
// close keep : Run  in onlyhas Spec{Kind,Args,BaseDir},  has  "      status"  . 
// base    "    calledbecome , but       / file   modify"  scenario, verify     fail. 
func TestRun_DoesNotTrustExecutorClaim(t *testing.T) {
	v := &Verifier{BaseDir: t.TempDir()}

	//  scenario A:       OK=true, but     outcode=1(examplee.g.      disconnectlang). 
	// verify   heavy ,     fail, but is ing    pass. 
	res, err := v.Run(Spec{Kind: "test", Args: []string{"/bin/sh", "-c", "echo simulated failure; exit 1"}})
	if err != nil {
		t.Fatalf("Run 返回错误: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("期望执行器谎报成功时校验器判 fail，实际 %q（evidence=%s）", res.Status, res.Evidence)
	}

	//  scenario B:      "filealreadymodifybecomenew  ", but  onfilerootbase  new  . 
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("old content only"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, _ = v.Run(Spec{Kind: "diff", BaseDir: dir, Args: []string{"main.go", "new content"}})
	if res.Status != StatusFail {
		t.Fatalf("期望文件真实未改时判 fail，实际 %q", res.Status)
	}
}

func TestRun_CommandRealExitZero(t *testing.T) {
	v := &Verifier{}
	res, _ := v.Run(Spec{Kind: "test", Args: []string{"/bin/sh", "-c", "echo hello"}})
	if res.Status != StatusPass {
		t.Fatalf("真实退出码 0 应 pass，实际 %q evidence=%s", res.Status, res.Evidence)
	}
}

func TestRun_DiffMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.txt"), []byte("错误提示已改为中文"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := &Verifier{BaseDir: dir}
	res, _ := v.Run(Spec{Kind: "diff", Args: []string{"app.txt", "改为中文"}})
	if res.Status != StatusPass {
		t.Fatalf("diff 命中期望子串应 pass，实际 %q detail=%s", res.Status, res.Detail)
	}
}

func TestRun_FileAndGrep(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := &Verifier{BaseDir: dir}

	// file store i.e. pass
	res, _ := v.Run(Spec{Kind: "file", Args: []string{"a.go"}})
	if res.Status != StatusPass {
		t.Fatalf("file 存在应 pass，实际 %q", res.Status)
	}
	// file  store  -> fail
	res, _ = v.Run(Spec{Kind: "file", Args: []string{"nope.go"}})
	if res.Status != StatusFail {
		t.Fatalf("file 不存在应 fail，实际 %q", res.Status)
	}
	// grep  in -> pass
	res, _ = v.Run(Spec{Kind: "grep", Args: []string{"func Foo", "a.go"}})
	if res.Status != StatusPass {
		t.Fatalf("grep 命中应 pass，实际 %q evidence=%s", res.Status, res.Evidence)
	}
	// grep   in -> fail
	res, _ = v.Run(Spec{Kind: "grep", Args: []string{"func Bar", "a.go"}})
	if res.Status != StatusFail {
		t.Fatalf("grep 不命中应 fail，实际 %q", res.Status)
	}
}

func TestRun_Unverifiable(t *testing.T) {
	v := &Verifier{}
	cases := []Spec{
		{Kind: "weird"},                     //   classtype
		{Kind: "test"},                      //   argv
		{Kind: "diff", Args: []string{"x"}}, // diff  period   
		{Kind: "grep"},                      // grep   form
	}
	for _, s := range cases {
		res, _ := v.Run(s)
		if res.Status != StatusUnverifiable {
			t.Fatalf("Spec %+v 应 unverifiable，实际 %q", s, res.Status)
		}
	}
}

func TestRun_PathEscapingRejected(t *testing.T) {
	dir := t.TempDir()
	v := &Verifier{BaseDir: dir}
	// out-of-scopepath   fail/unverifiable,    readto BaseDir ofout
	res, _ := v.Run(Spec{Kind: "diff", Args: []string{"../etc/passwd", "root"}})
	if res.Status == StatusPass {
		t.Fatalf("越界路径不得 pass，实际 %q", res.Status)
	}
}

// ---- M3 #15 verify side idchainconnect     ----

// scope in symlink referto base out -> diff read   reject(unverifiable/fail,    pass). 
func TestRun_SymlinkEscapeRejected(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	// base infile + base out  file + referto    symlink
	if err := os.WriteFile(filepath.Join(base, "inside.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "evil.link")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), link); err != nil {
		t.Fatal(err)
	}
	v := &Verifier{BaseDir: base}
	res, _ := v.Run(Spec{Kind: "diff", Args: []string{"evil.link", "TOPSECRET"}})
	if res.Status == StatusPass {
		t.Fatalf("symlink 逃逸读 base 外文件不得 pass，实际 %q", res.Status)
	}
}

// scope in symlink referto base inobjtgt ->   (   ). 
func TestRun_InnerSymlinkAllowed(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "real.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "good.link")
	if err := os.Symlink("real.txt", link); err != nil {
		t.Fatal(err)
	}
	v := &Verifier{BaseDir: base}
	res, _ := v.Run(Spec{Kind: "diff", Args: []string{"good.link", "hello world"}})
	if res.Status != StatusPass {
		t.Fatalf("base 内指向 base 内的 symlink 应 pass，实际 %q: %s", res.Status, res.Detail)
	}
}

// grep    Walk: base in symlink obj referto base out ->  ed,    toout in . 
func TestRun_GrepSkipsEscapedSymlinkDir(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	// base in    file  form
	if err := os.WriteFile(filepath.Join(base, "a.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatal(err)
	}
	// outside in  filealso same form(if  be     inout filepath)
	if err := os.WriteFile(filepath.Join(outside, "leak.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "dirlink")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	v := &Verifier{BaseDir: base}
	res, _ := v.Run(Spec{Kind: "grep", Args: []string{"needle"}})
	if res.Status != StatusPass {
		t.Fatalf("base 内应能搜到 needle，实际 %q", res.Status)
	}
	//  data   use  obj   out file
	if strings.Contains(res.Evidence, "leak.txt") || strings.Contains(res.Detail, "leak.txt") {
		t.Fatalf("grep 不得跟随逃逸 symlink 目录读外部文件: evidence=%q", res.Evidence)
	}
}

// ..     Clean+Rel after   (prevent connect ed). 
func TestRun_DotDotTraversalStillRejected(t *testing.T) {
	base := t.TempDir()
	v := &Verifier{BaseDir: base}
	for _, p := range []string{"sub/../../etc/passwd", "..\\..\\windows"} {
		res, _ := v.Run(Spec{Kind: "file", Args: []string{p}})
		if res.Status == StatusPass {
			t.Fatalf(".. 穿越 %q 不得 pass", p)
		}
	}
}
