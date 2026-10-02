// Package verify 实现独立校验器（verify 契约，M2 任务卡 #6 / 验收用例 10）。
//
// 核心理念（设计 v2 §6）：执行器完成动作后，校验器【自己】重新读一遍文件系统真实状态
// 与期望对比，绝不读执行器自报的 status——防止「执行器谎报成功」「执行者改测试自证」。
// 校验器的唯一输入是 Spec{Kind, Args, BaseDir}，结构上不存在「自报通道」这一事实即防线。
package verify

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// 校验结论取值（Result.Status）。
const (
	StatusPass         = "pass"         // 独立复核通过
	StatusFail         = "fail"         // 独立复核发现实际状态与期望不符
	StatusUnverifiable = "unverifiable" // 无法复核（命令找不到 / 文件读不出 / 参数非法）
	StatusPartial      = "partial"      // 部分满足（如 grep 命中但未覆盖全部期望）
)

// Spec 是一次校验请求。Kind 决定用哪条独立复核路径；Args 语义随 Kind 约定见下；
// BaseDir 为相对路径解析根（执行命令的工作目录 / 文件相对根）。
//
// Args 约定（冻结形状之外的语义实现，向后兼容可扩展）：
//   - test  : Args = 命令 argv（Args[0]=命令名，其余为参数）。校验器【自己】起子进程跑，
//     以【真实退出码】为准（0=pass，非 0=fail），stdout/stderr 作为证据。
//   - diff  : Args = [相对路径, 期望子串]。读该路径真实文件内容，必须【包含】期望子串才算 pass。
//   - grep  : Args = [模式, 相对路径...]（路径可空=递归扫 BaseDir 下常规文件）。
//     真实文件内容中能搜到模式即 pass。
//   - file  : Args = [相对路径, 期望子串?]。路径存在即 pass；给了期望子串则内容须包含。
type Spec struct {
	Kind    string   `json:"kind"`
	Args    []string `json:"args"`
	BaseDir string   `json:"base_dir"`
}

// Result 是独立校验结论。Evidence 为可落轨迹的真实证据片段（命令输出 / 文件内容摘录），
// Detail 为人话说明。
type Result struct {
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
	Detail   string `json:"detail"`
}

// Verifier 是无状态独立校验器；BaseDir 为未在 Spec 中覆盖时的默认相对根。
type Verifier struct {
	BaseDir string
}

// runTimeout 是单条校验命令的执行上限（独立校验不宜无限挂起）。
const runTimeout = 60 * time.Second

// Run 执行一次独立校验。
//
// 【伪代码逻辑层】（必写模块；语义/规则本体搬 VSL，此处只写控制流/分支/拒绝路径/异常处理）
//
// 模块职责：按 Spec.Kind 选择独立复核路径，亲自重跑命令 / 亲自读文件系统，对期望下结论。
// 结构防线：本函数签名里没有「执行器自报 status」入参——输入只有 Spec，故无从自证。
//
// 控制流：
//  0. base = Spec.BaseDir 非空 ? Spec.BaseDir : v.BaseDir（空则当前目录 "."）。
//  1. 按 Kind 分派：
//     a. Kind == "test"：
//     - Args 空 → unverifiable（拒绝凭空跑）。
//     - 起子进程 exec.CommandContext(ctx, Args[0], Args[1:]...)，cwd=base，ctx 超时=runTimeout。
//     - 捕获 combined output（stdout+stderr）。
//     - 拒绝路径：命令不存在（exec.ErrNotFound）→ unverifiable，Evidence=错误。
//     - 分支：exit == 0 → pass（证据=输出尾部）；exit != 0 → fail（真实失败，即使上游自称成功）。
//     - 超时 → fail（判为未通过，不洗白）。
//     b. Kind == "diff"：
//     - len(Args) < 2 → unverifiable（缺期望子串，无法比对）。
//     - path = 解析(base, Args[0])；读文件。
//     - 拒绝路径：文件不存在/不可读 → fail（真实状态=没有该文件）。
//     - 分支：内容 strings.Contains(内容, Args[1]) → pass；否则 fail（证据=实际内容摘录）。
//     c. Kind == "grep"：
//     - Args 空 → unverifiable（缺模式）。
//     - targets = Args[1:] 非空 ? Args[1:] : 递归收集 base 下常规文件。
//     - 逐文件读、rune 子串搜；命中即 pass（证据=首个命中 file:line）。
//     - 全部读完无命中 → fail。读单个文件失败跳过，不炸整次。
//     d. Kind == "file"：
//     - Args 空 → unverifiable（缺路径）。
//     - path 存在且为常规文件 → 进入内容判断；否则 fail。
//     - len(Args) >= 2 → 内容须包含 Args[1]，否则 fail。
//     e. 其它 Kind → unverifiable（未知校验类型，不猜）。
//
// 异常处理：所有「真实状态不符」一律 fail（对用户保守，不把可疑判成 pass）；
//
//	只有「连复核动作都做不起来」（参数缺 / 命令找不到 / 根目录非法）才给 unverifiable。
//	证据一律截断到 maxEvidence 字节，防回执爆炸。
func (v *Verifier) Run(spec Spec) (Result, error) {
	base := spec.BaseDir
	if strings.TrimSpace(base) == "" {
		base = v.BaseDir
	}
	if strings.TrimSpace(base) == "" {
		base = "."
	}

	switch spec.Kind {
	case "test":
		return v.runTest(base, spec.Args), nil
	case "diff":
		return v.runDiff(base, spec.Args), nil
	case "grep":
		return v.runGrep(base, spec.Args), nil
	case "file":
		return v.runFile(base, spec.Args), nil
	default:
		return Result{
			Status:   StatusUnverifiable,
			Evidence: "",
			Detail:   fmt.Sprintf("未知校验类型 %q（仅 test|diff|grep|file）", spec.Kind),
		}, nil
	}
}

// maxEvidence 截断证据片段长度。
const maxEvidence = 2000

func truncateEvidence(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxEvidence {
		return s[:maxEvidence] + "…(截断)"
	}
	return s
}

// resolve 把相对路径锚定到 base 下，并做双重 containment 防逃逸（M3 #15）：
//  1. 词法层：Join+Clean 后必须仍在 absBase 内（防 ../ 穿越）；
//  2. 符号链接层：对结果做 EvalSymlinks，真实路径必须仍在 base 的真实路径内
//     （防 symlink 指向 base 外；目标不存在时取最深已存在祖先判定）。
//
// 任一逃逸 → error（调用方映射为 unverifiable，拒绝/不可校验，不静默放行）。
func resolve(base, p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("空路径")
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	absBase = filepath.Clean(absBase)

	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(absBase, p)
	}
	clean := filepath.Clean(abs)
	// 第一道：词法 containment
	if !withinBase(clean, absBase) {
		return "", fmt.Errorf("路径越界: %q 不在校验根 %q 内", p, absBase)
	}

	// 第二道：符号链接 containment
	realBase := evalSafe(absBase)
	real := evalSafe(clean)
	if !withinBase(real, realBase) {
		return "", fmt.Errorf("符号链接逃逸: %q 解析后越出校验根 %q", p, realBase)
	}
	return clean, nil
}

// withinBase 报告 p 是否等于 root 或位于 root 之下。
func withinBase(p, root string) bool {
	if p == root {
		return true
	}
	return strings.HasPrefix(p, root+string(os.PathSeparator))
}

// evalSafe EvalSymlinks 一个路径；失败（多为目标不存在）时返回其最深已存在祖先。
func evalSafe(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	// 目标不存在：向上找第一个真实存在的祖先，再解析其符号链接
	cur := p
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return cur
		}
		if real, err := filepath.EvalSymlinks(parent); err == nil {
			return real
		}
		cur = parent
	}
}

// runTest 亲自跑命令，以真实退出码为准。
func (v *Verifier) runTest(base string, args []string) Result {
	if len(args) == 0 {
		return Result{Status: StatusUnverifiable, Detail: "test 校验缺命令 argv"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = base
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	evidence := truncateEvidence(buf.String())
	if ctx.Err() == context.DeadlineExceeded {
		return Result{Status: StatusFail, Evidence: evidence, Detail: fmt.Sprintf("校验命令超时（>%s），判未通过", runTimeout)}
	}
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") || os.IsNotExist(err) {
			return Result{Status: StatusUnverifiable, Evidence: evidence, Detail: "校验命令不存在，无法复核: " + err.Error()}
		}
		return Result{Status: StatusFail, Evidence: evidence, Detail: fmt.Sprintf("真实退出码非 0：%v", err)}
	}
	return Result{Status: StatusPass, Evidence: evidence, Detail: "校验命令真实退出码为 0"}
}

// runDiff 读真实文件内容对期望子串。
func (v *Verifier) runDiff(base string, args []string) Result {
	if len(args) < 2 {
		return Result{Status: StatusUnverifiable, Detail: "diff 校验需要 [路径, 期望子串]"}
	}
	p, err := resolve(base, args[0])
	if err != nil {
		return Result{Status: StatusUnverifiable, Detail: "diff 路径非法: " + err.Error()}
	}
	data, rerr := os.ReadFile(p)
	if rerr != nil {
		return Result{Status: StatusFail, Evidence: truncateEvidence(rerr.Error()), Detail: "真实文件读不到（不存在或不可读），判未通过: " + args[0]}
	}
	content := string(data)
	if strings.Contains(content, args[1]) {
		return Result{Status: StatusPass, Evidence: truncateEvidence(content), Detail: fmt.Sprintf("文件 %s 真实内容包含期望子串", args[0])}
	}
	return Result{Status: StatusFail, Evidence: truncateEvidence(content), Detail: fmt.Sprintf("文件 %s 真实内容【不包含】期望子串 %q", args[0], args[1])}
}

// runGrep 亲自在文件系统里搜模式。
func (v *Verifier) runGrep(base string, args []string) Result {
	if len(args) == 0 {
		return Result{Status: StatusUnverifiable, Detail: "grep 校验缺搜索模式"}
	}
	pattern := args[0]
	var targets []string
	if len(args) > 1 {
		for _, rel := range args[1:] {
			p, err := resolve(base, rel)
			if err != nil {
				continue
			}
			targets = append(targets, p)
		}
	} else {
		absBase, _ := filepath.Abs(base)
		realBase := evalSafe(absBase)
		// Walk 默认不跟随符号链接；此处仍显式校验：symlink 指向 base 外 → 跳过（选择=跳过，
		// 不中止整次 grep，防止一个逃逸 symlink 污染整次校验结果）。
		_ = filepath.Walk(absBase, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				if real, e := filepath.EvalSymlinks(path); e == nil && !withinBase(real, realBase) {
					if fi.IsDir() {
						return filepath.SkipDir
					}
					return nil // symlink 文件逃逸：跳过不读
				}
			}
			if fi.Mode().IsRegular() {
				targets = append(targets, path)
			}
			return nil
		})
	}
	for _, t := range targets {
		data, err := os.ReadFile(t)
		if err != nil {
			continue
		}
		if idx := strings.Index(string(data), pattern); idx >= 0 {
			line := 1 + strings.Count(string(data)[:idx], "\n")
			rel, _ := filepath.Rel(base, t)
			return Result{Status: StatusPass, Evidence: fmt.Sprintf("%s:%d: %s", rel, line, pattern), Detail: "在文件系统真实内容中搜到模式"}
		}
	}
	return Result{Status: StatusFail, Detail: fmt.Sprintf("在 %d 个真实文件中未搜到模式 %q", len(targets), pattern)}
}

// runFile 校验文件存在（可选内容断言）。
func (v *Verifier) runFile(base string, args []string) Result {
	if len(args) == 0 {
		return Result{Status: StatusUnverifiable, Detail: "file 校验缺路径"}
	}
	p, err := resolve(base, args[0])
	if err != nil {
		return Result{Status: StatusUnverifiable, Detail: "file 路径非法: " + err.Error()}
	}
	data, rerr := os.ReadFile(p)
	if rerr != nil {
		return Result{Status: StatusFail, Detail: "真实文件不存在或不可读: " + args[0]}
	}
	if len(args) >= 2 && args[1] != "" {
		if strings.Contains(string(data), args[1]) {
			return Result{Status: StatusPass, Evidence: truncateEvidence(string(data)), Detail: "文件存在且包含期望子串"}
		}
		return Result{Status: StatusFail, Evidence: truncateEvidence(string(data)), Detail: fmt.Sprintf("文件存在但内容不包含期望子串 %q", args[1])}
	}
	return Result{Status: StatusPass, Detail: "文件真实存在: " + args[0]}
}
