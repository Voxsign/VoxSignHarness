package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/contract"
)

func TestLoadContracts_Builtins(t *testing.T) {
	r, err := LoadContracts(t.TempDir()) // 空目录 → 仅内置
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "file", "search", "test", "run", "verify"} {
		c, ok := r.Get(name)
		if !ok {
			t.Fatalf("内置契约 %q 缺失", name)
		}
		if c.Version != "1.0" {
			t.Fatalf("内置契约 %q 版本应为 1.0，实际 %q", name, c.Version)
		}
		if c.Source != "builtin" {
			t.Fatalf("内置契约 %q source 应为 builtin，实际 %q", name, c.Source)
		}
	}
	if len(r.All()) != 6 {
		t.Fatalf("应恰好 6 个内置契约，实际 %d", len(r.All()))
	}
}

// 用例 9a：ValidateContract 拒绝缺字段契约。
func TestValidateContract_RejectsMissing(t *testing.T) {
	good := contract.ToolContract{
		Name: "demo", Version: "1.0",
		Caps:   []string{"run"},
		Params: map[string]string{"cmd": "string,required"},
		Risk:   map[string]string{"run": "low"},
	}
	if err := ValidateContract(good); err != nil {
		t.Fatalf("合法契约应通过: %v", err)
	}

	// 缺 name
	badName := good
	badName.Name = ""
	if err := ValidateContract(badName); err == nil {
		t.Fatal("缺 name 应拒绝")
	}
	// 缺 caps
	badCaps := good
	badCaps.Caps = nil
	if err := ValidateContract(badCaps); err == nil {
		t.Fatal("缺 caps 应拒绝")
	}
	// cap 缺 risk
	badRisk := good
	badRisk.Risk = map[string]string{}
	if err := ValidateContract(badRisk); err == nil {
		t.Fatal("cap 缺 risk 应拒绝")
	}
	// 非法 risk 级别
	badLevel := good
	badLevel.Risk = map[string]string{"run": "catastrophic"}
	if err := ValidateContract(badLevel); err == nil {
		t.Fatal("非法 risk 级别应拒绝")
	}
}

// 用例 9b：REGISTER_TOOL —— approved=false 不落盘；approved=true 落盘且下次可见。
func TestRegister_ToggleApproved(t *testing.T) {
	dir := t.TempDir()
	r, _ := LoadContracts(dir)
	c := contract.ToolContract{
		Name: "md2pdf", Version: "1.0",
		Caps:   []string{"convert"},
		Params: map[string]string{"in": "string,required"},
		Risk:   map[string]string{"convert": "low"},
	}

	// approved=false → error 且不落盘
	if err := r.Register(c, false); err == nil {
		t.Fatal("approved=false 应返回错误")
	}
	if _, err := os.Stat(filepath.Join(dir, "md2pdf.contract.json")); !os.IsNotExist(err) {
		t.Fatal("approved=false 不得落盘")
	}

	// approved=true → 落盘
	if err := r.Register(c, true); err != nil {
		t.Fatalf("approved=true 应注册成功: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "md2pdf.contract.json")); err != nil {
		t.Fatalf("approved=true 应落盘: %v", err)
	}

	// 重新 LoadContracts 应看到语音注册的契约
	r2, err := LoadContracts(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := r2.Get("md2pdf")
	if !ok {
		t.Fatal("重新装载后应看到 md2pdf")
	}
	if got.Source != "voice" {
		t.Fatalf("语音注册契约 source 应为 voice，实际 %q", got.Source)
	}
}

// 用例 9c：非法契约即使 approved=true 也拒绝落盘。
func TestRegister_InvalidRejectedEvenApproved(t *testing.T) {
	dir := t.TempDir()
	r, _ := LoadContracts(dir)
	bad := contract.ToolContract{Name: "broken"} // 缺 caps/params/risk
	if err := r.Register(bad, true); err == nil {
		t.Fatal("非法契约即使 approved=true 也应拒绝")
	}
	if _, err := os.Stat(filepath.Join(dir, "broken.contract.json")); !os.IsNotExist(err) {
		t.Fatal("非法契约不得落盘")
	}
}

func TestExecutor_TestPassAndFail(t *testing.T) {
	e := &Executor{BaseDir: t.TempDir()}
	reg, _ := LoadContracts(t.TempDir())
	c := reg.Contracts["test"]

	// 真命令退出 0 → OK
	recv, err := e.Exec("test", map[string]any{"command": []string{"/bin/sh", "-c", "true"}}, c)
	if err != nil || !recv.OK {
		t.Fatalf("sh -c true 应 OK，recv=%+v err=%v", recv, err)
	}
	// 真命令退出 1 → OK=false（执行器按真实退出码，不自证）
	recv, _ = e.Exec("test", map[string]any{"command": []string{"/bin/sh", "-c", "exit 1"}}, c)
	if recv.OK {
		t.Fatal("sh -c exit 1 应 OK=false")
	}
}

func TestExecutor_FileWriteBackupAndRead(t *testing.T) {
	dir := t.TempDir()
	logDir := t.TempDir()
	e := &Executor{BaseDir: dir}
	reg2, _ := LoadContracts(t.TempDir())
	c := reg2.Contracts["file"]

	// 先写初版
	recv, _ := e.Exec("file", map[string]any{"action": "write", "path": "a.txt", "content": "v1"}, c)
	if !recv.OK {
		t.Fatalf("首次写入失败: %+v", recv)
	}
	// 再改：应在 logDir/backups 留下 v1 备份
	recv, _ = e.Exec("file", map[string]any{"action": "write", "path": "a.txt", "content": "v2", "log_dir": logDir}, c)
	if !recv.OK {
		t.Fatalf("二次写入失败: %+v", recv)
	}
	entries, _ := os.ReadDir(filepath.Join(logDir, "backups"))
	if len(entries) != 1 {
		t.Fatalf("二次写入前应留 1 个备份，实际 %d", len(entries))
	}
	// 读回 v2
	recv, _ = e.Exec("file", map[string]any{"action": "read", "path": "a.txt"}, c)
	if !strings.Contains(recv.Stdout, "v2") {
		t.Fatalf("读回应为 v2，实际 %q", recv.Stdout)
	}
}

func TestExecutor_SearchAndVerify(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &Executor{BaseDir: dir}
	reg, _ := LoadContracts(t.TempDir())

	// search symbol
	recv, _ := e.Exec("search", map[string]any{"kind": "symbol", "pattern": "main"}, reg.Contracts["search"])
	if !recv.OK || !strings.Contains(recv.Stdout, "main.go") {
		t.Fatalf("search symbol 应命中 main.go: %+v", recv)
	}
	// verify: 文件真实包含 func main → pass
	recv, _ = e.Exec("verify", map[string]any{"kind": "file", "args": []string{"main.go", "func main"}}, reg.Contracts["verify"])
	if !recv.OK {
		t.Fatalf("verify 应 pass: %+v", recv)
	}
	// verify: 期望子串不存在 → fail
	recv, _ = e.Exec("verify", map[string]any{"kind": "diff", "args": []string{"main.go", "nonexistent"}}, reg.Contracts["verify"])
	if recv.OK {
		t.Fatalf("verify 对真实缺失应 fail: %+v", recv)
	}
}

// TestExecutorBackupPathReported（M4-2）：NOTE 追加 / EDIT 改写后，结构化输出必须带出
// 具体备份路径（BackupMarker 行），且该路径真实存在于 backups 目录；只读动作无该字段。
func TestExecutorBackupPathReported(t *testing.T) {
	dir := t.TempDir()
	logDir := t.TempDir()
	e := &Executor{BaseDir: dir}
	reg, _ := LoadContracts(t.TempDir())
	c := reg.Contracts["file"]

	// NOTE 追加：notes.md 先存在旧内容，再 append。
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("旧想法\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recv, _ := e.Exec("file", map[string]any{"action": "append", "path": "notes.md", "content": "新想法\n", "log_dir": logDir}, c)
	if !recv.OK {
		t.Fatalf("NOTE 追加失败: %+v", recv)
	}
	noteBackup := ParseBackupPath(recv.Stdout)
	if noteBackup == "" {
		t.Fatalf("NOTE 追加后 stdout 应含 BackupMarker，实际 %q", recv.Stdout)
	}
	if _, err := os.Stat(noteBackup); err != nil {
		t.Fatalf("结构化备份路径应真实存在: %s (%v)", noteBackup, err)
	}
	if !strings.Contains(noteBackup, filepath.Join(logDir, "backups")) {
		t.Fatalf("备份路径应落在 log_dir/backups 下: %s", noteBackup)
	}

	// EDIT 改写：同样带出备份路径。
	recv, _ = e.Exec("file", map[string]any{"action": "write", "path": "notes.md", "content": "覆写\n", "log_dir": logDir}, c)
	editBackup := ParseBackupPath(recv.Stdout)
	if editBackup == "" {
		t.Fatalf("EDIT 改写后应含备份路径，实际 %q", recv.Stdout)
	}
	if _, err := os.Stat(editBackup); err != nil {
		t.Fatalf("EDIT 备份路径应存在: %s", err)
	}
	if editBackup == noteBackup {
		t.Fatalf("两次备份应是不同文件，不能同名覆盖: note=%s edit=%s", noteBackup, editBackup)
	}

	// 只读动作：read / exists 不应出现备份字段。
	recv, _ = e.Exec("file", map[string]any{"action": "read", "path": "notes.md"}, c)
	if p := ParseBackupPath(recv.Stdout); p != "" {
		t.Fatalf("只读 read 不应带出备份路径，实际 %q", p)
	}
	recv, _ = e.Exec("file", map[string]any{"action": "exists", "path": "notes.md", "log_dir": logDir}, c)
	if p := ParseBackupPath(recv.Stdout); p != "" {
		t.Fatalf("exists 不应带出备份路径，实际 %q", p)
	}

	// 首次创建（无旧内容）→ 无备份，无字段。
	recv, _ = e.Exec("file", map[string]any{"action": "write", "path": "brand_new.md", "content": "first\n", "log_dir": logDir}, c)
	if p := ParseBackupPath(recv.Stdout); p != "" {
		t.Fatalf("首次创建无旧内容不应带出备份路径，实际 %q", p)
	}
}
