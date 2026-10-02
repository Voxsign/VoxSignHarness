package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 用例 10：改完跑测试 → 读实际结果，不读自报。
// 结构保证：Run 的入参只有 Spec{Kind,Args,BaseDir}，没有任何「执行器自报 status」通道。
// 本测试构造「执行器自称成功，但命令真实失败 / 文件真实未改」的场景，校验器必须判 fail。
func TestRun_DoesNotTrustExecutorClaim(t *testing.T) {
	v := &Verifier{BaseDir: t.TempDir()}

	// 场景 A：执行器自报 OK=true，但命令真实退出码=1（例如跑了个必失败的断言）。
	// 校验器亲自重跑，必须判 fail，而不是顺着自报判 pass。
	res, err := v.Run(Spec{Kind: "test", Args: []string{"/bin/sh", "-c", "echo simulated failure; exit 1"}})
	if err != nil {
		t.Fatalf("Run 返回错误: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("期望执行器谎报成功时校验器判 fail，实际 %q（evidence=%s）", res.Status, res.Evidence)
	}

	// 场景 B：执行器自报「文件已改成新文案」，但磁盘上文件根本不含新文案。
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

	// file 存在即 pass
	res, _ := v.Run(Spec{Kind: "file", Args: []string{"a.go"}})
	if res.Status != StatusPass {
		t.Fatalf("file 存在应 pass，实际 %q", res.Status)
	}
	// file 不存在 → fail
	res, _ = v.Run(Spec{Kind: "file", Args: []string{"nope.go"}})
	if res.Status != StatusFail {
		t.Fatalf("file 不存在应 fail，实际 %q", res.Status)
	}
	// grep 命中 → pass
	res, _ = v.Run(Spec{Kind: "grep", Args: []string{"func Foo", "a.go"}})
	if res.Status != StatusPass {
		t.Fatalf("grep 命中应 pass，实际 %q evidence=%s", res.Status, res.Evidence)
	}
	// grep 不命中 → fail
	res, _ = v.Run(Spec{Kind: "grep", Args: []string{"func Bar", "a.go"}})
	if res.Status != StatusFail {
		t.Fatalf("grep 不命中应 fail，实际 %q", res.Status)
	}
}

func TestRun_Unverifiable(t *testing.T) {
	v := &Verifier{}
	cases := []Spec{
		{Kind: "weird"},                     // 未知类型
		{Kind: "test"},                      // 缺 argv
		{Kind: "diff", Args: []string{"x"}}, // diff 缺期望子串
		{Kind: "grep"},                      // grep 缺模式
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
	// 越界路径应判 fail/unverifiable，绝不能读到 BaseDir 之外
	res, _ := v.Run(Spec{Kind: "diff", Args: []string{"../etc/passwd", "root"}})
	if res.Status == StatusPass {
		t.Fatalf("越界路径不得 pass，实际 %q", res.Status)
	}
}

// ---- M3 #15 verify 侧符号链接逃逸专项 ----

// scope 内 symlink 指向 base 外 → diff 读它必须拒绝（unverifiable/fail，绝不 pass）。
func TestRun_SymlinkEscapeRejected(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	// base 内文件 + base 外诱饵文件 + 指向诱饵的 symlink
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

// scope 内 symlink 指向 base 内目标 → 放行（不误伤）。
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

// grep 递归 Walk：base 内 symlink 目录指向 base 外 → 跳过，不得搜到外部内容。
func TestRun_GrepSkipsEscapedSymlinkDir(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	// base 内一个普通文件含模式
	if err := os.WriteFile(filepath.Join(base, "a.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatal(err)
	}
	// outside 内一个文件也含同模式（若逃逸被跟随会误命中外部文件路径）
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
	// 证据不得引用逃逸目录里的外部文件
	if strings.Contains(res.Evidence, "leak.txt") || strings.Contains(res.Detail, "leak.txt") {
		t.Fatalf("grep 不得跟随逃逸 symlink 目录读外部文件: evidence=%q", res.Evidence)
	}
}

// .. 穿越在 Clean+Rel 后仍复查（防拼接绕过）。
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
