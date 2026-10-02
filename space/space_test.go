package space

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/contract"
)

// writeManifest 往 dir 写一个 manifest 文件并返回。
func writeManifest(t *testing.T, dir, name string, m *Manifest) *Manifest {
	t.Helper()
	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := r.Add(m); err != nil {
		t.Fatalf("Add: %v", err)
	}
	return m
}

func TestLoad_EmptyDirGivesBuiltins(t *testing.T) {
	dir := t.TempDir()
	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, want := range []string{"global", "project", "sandbox", "vault-notes", "vault-creds", "external"} {
		if _, ok := r.Get(want); !ok {
			t.Errorf("内置模板缺失: %s", want)
		}
	}
}

func TestLoad_MissingDirGivesBuiltins(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nope")
	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := r.Get("global"); !ok {
		t.Fatal("缺失目录应返回内置 global 模板")
	}
}

// 用例 5：project 域内 EDIT 放行。
func TestCheck_ProjectEditAllowed(t *testing.T) {
	dir := t.TempDir()
	proj := t.TempDir() // 真实存在的 scope 目录，避免漂移
	r, _ := Load(dir)
	r.Add(&Manifest{
		Name: "proj", Type: TypeProject,
		Scope: []string{proj}, Tools: []string{"file", "git", "read"},
		Perms: Perms{Read: true, Write: true},
	})
	in := CheckInput{
		Intent:   contract.Intent{Intent: contract.IntentEdit, Space: "proj"},
		Grant:    Grant{Authorized: true},
		ToolCaps: []string{"file"},
	}
	v := Check(r, in)
	if !v.Allowed || v.Reason != "" {
		t.Fatalf("用例5: Allowed=%v Reason=%q 期望放行", v.Allowed, v.Reason)
	}
}

// 用例 6：vault-creds 外发请求 → BOUNDARY_VIOLATION（不因确认放行）。
func TestCheck_VaultCredsExternalBlocked(t *testing.T) {
	dir := t.TempDir()
	r, _ := Load(dir)
	// 显式构造一个 vault-creds 域（只读、无 deploy/http）
	r.Add(&Manifest{
		Name: "vc", Type: TypeVaultCreds,
		Scope: []string{t.TempDir()}, Tools: []string{"read"},
		Perms: Perms{Read: true, Write: false},
	})
	// 即便带"确认放行"意图，space_check 仍越界拦截（CheckInput 无 confirm 输入 = 结构上不因确认放行）
	in := CheckInput{
		Intent:   contract.Intent{Intent: contract.IntentDeploy, Space: "vc"},
		Grant:    Grant{Authorized: true},
		ToolCaps: []string{"deploy"},
	}
	v := Check(r, in)
	if v.Allowed || v.Reason != "boundary_violation" {
		t.Fatalf("用例6: Allowed=%v Reason=%q 期望 boundary_violation", v.Allowed, v.Reason)
	}
}

// 用例 7：manifest 与实际路径漂移 → DetectDrift 检出、该域 Check 失效。
func TestCheck_DriftDetectedAndDenied(t *testing.T) {
	dir := t.TempDir()
	r, _ := Load(dir)
	r.Add(&Manifest{
		Name: "ghost", Type: TypeProject,
		Scope: []string{"/nonexistent/path/voicesign-xyz"}, Tools: []string{"file"},
		Perms: Perms{Read: true, Write: true},
	})
	drifts, err := r.DetectDrift()
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	found := false
	for _, d := range drifts {
		if d.Manifest == "ghost" {
			found = true
		}
	}
	if !found {
		t.Fatalf("用例7: DetectDrift 未检出 ghost 域漂移: %+v", drifts)
	}
	// 漂移域 Check 必须失效（回问重建路径）
	v := Check(r, CheckInput{
		Intent:   contract.Intent{Intent: contract.IntentEdit, Space: "ghost"},
		Grant:    Grant{Authorized: true},
		ToolCaps: []string{"file"},
	})
	if v.Allowed || v.Reason != "drift" {
		t.Fatalf("用例7: 漂移域 Allowed=%v Reason=%q 期望 drift", v.Allowed, v.Reason)
	}
}

func TestCheck_UnknownSpaceDenied(t *testing.T) {
	dir := t.TempDir()
	r, _ := Load(dir)
	v := Check(r, CheckInput{
		Intent: contract.Intent{Space: "no-such-space"},
		Grant:  Grant{Authorized: true},
	})
	if v.Allowed || v.Reason != "unknown_space" {
		t.Fatalf("Allowed=%v Reason=%q 期望 unknown_space（不自动切 global）", v.Allowed, v.Reason)
	}
}

func TestCheck_DefaultDenyNoGrant(t *testing.T) {
	dir := t.TempDir()
	r, _ := Load(dir)
	r.Add(&Manifest{
		Name: "p2", Type: TypeProject, Scope: []string{t.TempDir()},
		Tools: []string{"file"}, Perms: Perms{Read: true, Write: true},
	})
	v := Check(r, CheckInput{
		Intent:   contract.Intent{Intent: contract.IntentEdit, Space: "p2"},
		Grant:    Grant{Authorized: false}, // 本次未授权
		ToolCaps: []string{"file"},
	})
	if v.Allowed || v.Reason != "default_deny" {
		t.Fatalf("Allowed=%v Reason=%q 期望 default_deny", v.Allowed, v.Reason)
	}
}

func TestCheck_DefaultDenyNoWritePerm(t *testing.T) {
	dir := t.TempDir()
	r, _ := Load(dir)
	// vault-notes 模板本身可写；这里造一个只读域跑 EDIT
	r.Add(&Manifest{
		Name: "ro", Type: TypeGlobal, Scope: []string{t.TempDir()},
		Tools: []string{"read"}, Perms: Perms{Read: true, Write: false},
	})
	v := Check(r, CheckInput{
		Intent:   contract.Intent{Intent: contract.IntentEdit, Space: "ro"},
		Grant:    Grant{Authorized: true},
		ToolCaps: []string{"read"},
	})
	if v.Allowed || v.Reason != "default_deny" {
		t.Fatalf("Allowed=%v Reason=%q 期望 default_deny（只读域跑写意图）", v.Allowed, v.Reason)
	}
}

func TestCheck_CrossRefDeny(t *testing.T) {
	dir := t.TempDir()
	r, _ := Load(dir)
	r.Add(&Manifest{
		Name: "app", Type: TypeProject, Scope: []string{t.TempDir()},
		Tools: []string{"file", "read"}, Perms: Perms{Read: true, Write: true},
		CrossRefs: nil, // 未声明跨域引用
	})
	// 意图目标点名了 vault-notes，但 app 未在 cross_refs 声明
	v := Check(r, CheckInput{
		Intent: contract.Intent{
			Intent: contract.IntentEdit, Space: "app",
			Target: &contract.Target{Entity: "vault-notes"},
		},
		Grant:    Grant{Authorized: true},
		ToolCaps: []string{"file"},
	})
	if v.Allowed || v.Reason != "cross_ref_deny" {
		t.Fatalf("Allowed=%v Reason=%q 期望 cross_ref_deny", v.Allowed, v.Reason)
	}
}

func TestCheck_CrossRefAllowedWhenDeclared(t *testing.T) {
	dir := t.TempDir()
	r, _ := Load(dir)
	r.Add(&Manifest{
		Name: "app", Type: TypeProject, Scope: []string{t.TempDir()},
		Tools: []string{"file", "read"}, Perms: Perms{Read: true, Write: true},
		CrossRefs: []string{"vault-notes"},
	})
	v := Check(r, CheckInput{
		Intent: contract.Intent{
			Intent: contract.IntentQuery, Space: "app",
			Target: &contract.Target{Entity: "vault-notes"},
		},
		Grant:    Grant{Authorized: true},
		ToolCaps: []string{"read"},
	})
	if !v.Allowed {
		t.Fatalf("已声明 cross_refs 应放行: Reason=%q", v.Reason)
	}
}

func TestAdd_VersionBumpsAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	r, _ := Load(dir)
	m := &Manifest{Name: "v1", Type: TypeProject, Perms: Perms{Read: true, Write: true}, Tools: []string{"file"}}
	if err := r.Add(m); err != nil {
		t.Fatalf("Add#1: %v", err)
	}
	if m.Version != 1 {
		t.Fatalf("version=%d want 1", m.Version)
	}
	if err := r.Add(&Manifest{Name: "v1", Type: TypeProject, Perms: Perms{Read: true, Write: true}, Tools: []string{"file"}}); err != nil {
		t.Fatalf("Add#2: %v", err)
	}
	got, _ := r.Get("v1")
	if got.Version != 2 {
		t.Fatalf("version=%d want 2", got.Version)
	}
	if _, err := os.Stat(m.Path + ".bak"); err != nil {
		t.Fatalf("写前备份应存在: %v", err)
	}
}

func TestList_Sorted(t *testing.T) {
	dir := t.TempDir()
	r, _ := Load(dir)
	list := r.List()
	if len(list) < 6 {
		t.Fatalf("内置域应 >=6, got %d", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i-1] > list[i] {
			t.Fatalf("List 未排序: %v", list)
		}
	}
}

// 确保 data/spaces 样例 JSON 可被解析（机器可读原则）。
func TestSampleManifestsParse(t *testing.T) {
	dir := filepath.Join("..", "data", "spaces")
	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load data/spaces: %v", err)
	}
	for _, name := range []string{"voicesign-harness", "voxbuybot", "vault-notes", "vault-creds"} {
		m, ok := r.Get(name)
		if !ok {
			t.Fatalf("样例域缺失: %s", name)
		}
		if m.Type == "" {
			t.Fatalf("%s 缺 type", name)
		}
	}
}

// ---- M3 #15 路径边界硬化：ResolveScopePath 专项 ----

// 构造一个 scope 目录 + 目录外的诱饵目录 + 指向诱饵的符号链接。
func setupSymlinkScope(t *testing.T) (scopeRoot, outside string) {
	t.Helper()
	root := t.TempDir()
	scopeRoot = filepath.Join(root, "scope")
	outside = filepath.Join(root, "outside")
	if err := os.Mkdir(scopeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	// scope 内一个真实文件
	if err := os.WriteFile(filepath.Join(scopeRoot, "real.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	// outside 内一个诱饵文件
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	return scopeRoot, outside
}

// scope 内 symlink 指向 scope 内目标 → 放行。
func TestResolveScopePath_InnerSymlinkAllowed(t *testing.T) {
	scopeRoot, _ := setupSymlinkScope(t)
	// scope 内 symlink → scope 内 real.txt
	link := filepath.Join(scopeRoot, "inner.link")
	if err := os.Symlink("real.txt", link); err != nil {
		t.Fatal(err)
	}
	got, ok := ResolveScopePath(scopeRoot, "inner.link")
	if !ok {
		t.Fatal("scope 内指向 scope 内的 symlink 应放行")
	}
	if !strings.Contains(got, "inner.link") {
		t.Fatalf("got=%s", got)
	}
}

// scope 内 symlink 指向 scope 外 → 拒绝（符号链接逃逸）。
func TestResolveScopePath_SymlinkEscapeRejected(t *testing.T) {
	scopeRoot, outside := setupSymlinkScope(t)
	link := filepath.Join(scopeRoot, "evil.link")
	// 指向 scope 外的 secret.txt
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), link); err != nil {
		t.Fatal(err)
	}
	if _, ok := ResolveScopePath(scopeRoot, "evil.link"); ok {
		t.Fatal("symlink 逃逸出 scope 必须拒绝")
	}
}

// 嵌套 symlink 目录逃逸 → 拒绝。
func TestResolveScopePath_NestedSymlinkDirEscape(t *testing.T) {
	scopeRoot, outside := setupSymlinkScope(t)
	// scope/dirlink -> outside（嵌套：scope/dirlink/secret.txt 逃逸）
	link := filepath.Join(scopeRoot, "dirlink")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, ok := ResolveScopePath(scopeRoot, filepath.Join("dirlink", "secret.txt")); ok {
		t.Fatal("嵌套 symlink 目录逃逸必须拒绝")
	}
}

// .. 穿越 → 拒绝。
func TestResolveScopePath_DotDotTraversal(t *testing.T) {
	scopeRoot, _ := setupSymlinkScope(t)
	if _, ok := ResolveScopePath(scopeRoot, "../outside/secret.txt"); ok {
		t.Fatal(".. 穿越必须拒绝")
	}
	if _, ok := ResolveScopePath(scopeRoot, "sub/../../outside/secret.txt"); ok {
		t.Fatal("嵌套 .. 穿越必须拒绝")
	}
}

// 不存在的新目标（新建场景）→ 祖先链不逃逸即放行。
func TestResolveScopePath_NonexistentTargetAncestor(t *testing.T) {
	scopeRoot, _ := setupSymlinkScope(t)
	got, ok := ResolveScopePath(scopeRoot, "newdir/newfile.txt")
	if !ok {
		t.Fatal("scope 内新建目标应放行（祖先链不逃逸）")
	}
	if strings.Contains(got, "..") {
		t.Fatalf("返回路径应已 Clean: %s", got)
	}
}

// 绝对目标落在 scope 内 → 放行；落在 scope 外 → 拒绝。
func TestResolveScopePath_AbsoluteTarget(t *testing.T) {
	scopeRoot, outside := setupSymlinkScope(t)
	if _, ok := ResolveScopePath(scopeRoot, filepath.Join(scopeRoot, "real.txt")); !ok {
		t.Fatal("scope 内绝对路径应放行")
	}
	if _, ok := ResolveScopePath(scopeRoot, filepath.Join(outside, "secret.txt")); ok {
		t.Fatal("scope 外绝对路径应拒绝")
	}
}

// drift 检查符号链接感知：scope 真实路径被删除（EvalSymlinks 失败）→ 该域漂移失效。
func TestDrift_SymlinkAware(t *testing.T) {
	dir := t.TempDir()
	r, _ := Load(dir)
	real := filepath.Join(t.TempDir(), "realscope")
	os.Mkdir(real, 0o755)
	r.Add(&Manifest{Name: "sym", Type: TypeProject, Scope: []string{real}, Perms: Perms{Read: true, Write: true}})

	// 初始无漂移
	if drifts, _ := r.DetectDrift(); len(drifts) != 0 {
		t.Fatalf("初始不应有漂移: %+v", drifts)
	}
	// 把 scope 目录删掉（EvalSymlinks 解析失败）→ 漂移
	if err := os.RemoveAll(real); err != nil {
		t.Fatal(err)
	}
	drifts, err := r.DetectDrift()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range drifts {
		if d.Manifest == "sym" {
			found = true
		}
	}
	if !found {
		t.Fatalf("scope 路径删除后应检出 drift: %+v", drifts)
	}
}
