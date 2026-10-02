// Package tools 实现工具契约注册表（六内置契约 + REGISTER_TOOL 语音自举）与六工具执行器。
//
// 边界（冻结契约 §tools）：本包【不做】门禁——space_check 与 risk 裁决在 pipeline 层；
// 本包只负责：契约校验 / 注册落盘 / 按 caps 机械执行。未过门禁的动作 pipeline 根本不会传进来。
package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"voicesign-harness/contract"
)

// Registry 是工具契约注册表。Contracts 为 name→契约；dir 为语音注册落盘目录（未导出，
// 不改变冻结导出形状）。
type Registry struct {
	Contracts map[string]contract.ToolContract `json:"contracts"`
	dir       string
}

// builtinContracts 返回内置六契约 @1.0（git/file/search/test/run/verify）。
// risk 逐 cap 覆盖：不可逆动作标 irreversible，供 risk 包与人工确认裁决。
func builtinContracts() []contract.ToolContract {
	return []contract.ToolContract{
		{
			Name: "git", Version: "1.0", Source: "builtin",
			Caps:          []string{"status", "diff", "log", "commit", "checkout"},
			Params:        map[string]string{"args": "[]string,optional"},
			SideEffects:   []string{"读工作区/索引", "commit 不可逆(本地)"},
			AllowedSpaces: []string{"project", "sandbox"},
			Risk: map[string]string{
				"status": "none", "diff": "none", "log": "none",
				"commit": "irreversible", "checkout": "medium",
			},
		},
		{
			Name: "file", Version: "1.0", Source: "builtin",
			Caps:          []string{"read", "write", "append", "exists"},
			Params:        map[string]string{"path": "string,required", "content": "string,optional"},
			SideEffects:   []string{"write/append 修改文件（写前备份到 log_dir/backups）"},
			AllowedSpaces: []string{"project", "sandbox", "vault-notes"},
			Risk: map[string]string{
				"read": "none", "exists": "none", "append": "low", "write": "high",
			},
		},
		{
			Name: "search", Version: "1.0", Source: "builtin",
			Caps:          []string{"text", "symbol"},
			Params:        map[string]string{"pattern": "string,required"},
			SideEffects:   []string{"只读扫描"},
			AllowedSpaces: []string{"global", "project", "sandbox", "vault-notes", "vault-creds"},
			Risk: map[string]string{
				"text": "none", "symbol": "none",
			},
		},
		{
			Name: "test", Version: "1.0", Source: "builtin",
			Caps:          []string{"run"},
			Params:        map[string]string{"command": "[]string,required"},
			SideEffects:   []string{"跑测试（只读为主，可写临时产物）"},
			AllowedSpaces: []string{"project", "sandbox"},
			Risk:          map[string]string{"run": "low"},
		},
		{
			Name: "run", Version: "1.0", Source: "builtin",
			Caps:          []string{"exec"},
			Params:        map[string]string{"command": "[]string,required"},
			SideEffects:   []string{"任意命令（高风险，gate 在 pipeline）"},
			AllowedSpaces: []string{"project", "sandbox"},
			Risk:          map[string]string{"exec": "high"},
		},
		{
			Name: "verify", Version: "1.0", Source: "builtin",
			Caps:          []string{"run"},
			Params:        map[string]string{"kind": "string,required", "args": "[]string,optional"},
			SideEffects:   []string{"独立只读复核（读 fs/重跑命令，不改状态）"},
			AllowedSpaces: []string{"global", "project", "sandbox", "vault-notes", "vault-creds"},
			Risk:          map[string]string{"run": "none"},
		},
	}
}

// LoadContracts 装载注册表：先放内置六契约，再叠加 dir/*.contract.json（语音注册）。
// dir 缺失/为空 → 仅内置契约（不报错）。
func LoadContracts(dir string) (*Registry, error) {
	r := &Registry{Contracts: map[string]contract.ToolContract{}, dir: dir}
	for _, c := range builtinContracts() {
		r.Contracts[c.Name] = c
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, fmt.Errorf("读取契约目录 %s 失败: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".contract.json") {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			return nil, fmt.Errorf("读取契约文件 %s 失败: %w", e.Name(), rerr)
		}
		var c contract.ToolContract
		if jerr := json.Unmarshal(data, &c); jerr != nil {
			return nil, fmt.Errorf("解析契约文件 %s 失败: %w", e.Name(), jerr)
		}
		if err := ValidateContract(c); err != nil {
			return nil, fmt.Errorf("契约文件 %s 不合法: %w", e.Name(), err)
		}
		r.Contracts[c.Name] = c
	}
	return r, nil
}

// Get 按名取契约。
func (r *Registry) Get(name string) (contract.ToolContract, bool) {
	c, ok := r.Contracts[name]
	return c, ok
}

// All 按名排序返回全部契约（确定性顺序，便于测试与渲染）。
func (r *Registry) All() []contract.ToolContract {
	names := make([]string, 0, len(r.Contracts))
	for n := range r.Contracts {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]contract.ToolContract, 0, len(names))
	for _, n := range names {
		out = append(out, r.Contracts[n])
	}
	return out
}

var validRiskLevels = map[string]bool{
	"none": true, "low": true, "medium": true, "high": true, "irreversible": true,
}

// ValidateContract 校验契约字段完整性：name/version/caps/params 必填，risk 覆盖每个 cap。
func ValidateContract(c contract.ToolContract) error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("契约 name 必填")
	}
	if strings.TrimSpace(c.Version) == "" {
		return fmt.Errorf("契约 %q version 必填", c.Name)
	}
	if len(c.Caps) == 0 {
		return fmt.Errorf("契约 %q 至少声明一个 cap", c.Name)
	}
	if len(c.Params) == 0 {
		return fmt.Errorf("契约 %q 至少声明一个 param", c.Name)
	}
	if len(c.Risk) == 0 {
		return fmt.Errorf("契约 %q 必须声明 risk 映射", c.Name)
	}
	for _, cap := range c.Caps {
		lvl, ok := c.Risk[cap]
		if !ok {
			return fmt.Errorf("契约 %q 的 cap %q 缺少 risk 分级", c.Name, cap)
		}
		if !validRiskLevels[lvl] {
			return fmt.Errorf("契约 %q 的 cap %q risk 级别 %q 非法（none|low|medium|high|irreversible）", c.Name, cap, lvl)
		}
	}
	return nil
}

// riskGrade 从契约 risk 映射粗分一个注册风险等级（供人工确认前展示；最终 gate 在 pipeline）。
func riskGrade(c contract.ToolContract) string {
	for _, lvl := range c.Risk {
		if lvl == "irreversible" {
			return "human"
		}
	}
	for _, lvl := range c.Risk {
		if lvl == "high" {
			return "strong"
		}
	}
	return "light"
}

// Register 走 REGISTER_TOOL 自举流程。
//
// 【伪代码逻辑层】（必写模块；风险分级矩阵语义搬 VSL，此处只写控制流/拒绝路径）
//
// 控制流：
//  1. ValidateContract(c)：缺 name/version/caps/params 或某 cap 缺 risk → 拒绝，不落盘。
//  2. riskGrade(c)：含 irreversible → human；否则含 high → strong；否则 light。
//     （仅用于回执/展示；是否放行由 pipeline 的 risk 裁决与人工确认决定。）
//  3. 人工确认闸：approved == false → 返回错误【且绝不写盘】（用例 9：未批准不落盘）。
//  4. approved == true：
//     a. 补 Source="voice"、RegisteredAt=当前时间戳。
//     b. 写 <dir>/<name>.contract.json（原子写）。
//     c. 成功后才 upsert 进内存 Contracts（先落盘成功再改内存，防半态）。
//     拒绝路径：dir 为空（注册表非 LoadContracts 而来）→ 拒绝落盘；写盘失败 → 回滚内存。
func (r *Registry) Register(c contract.ToolContract, approved bool) error {
	if err := ValidateContract(c); err != nil {
		return fmt.Errorf("契约校验未过，拒绝注册: %w", err)
	}
	grade := riskGrade(c)
	if !approved {
		return fmt.Errorf("契约 %q 风险等级 %s 未经人工确认（approved=false），拒绝落盘", c.Name, grade)
	}
	if strings.TrimSpace(r.dir) == "" {
		return fmt.Errorf("注册表未绑定契约目录（非 LoadContracts 装载），无法落盘 %q", c.Name)
	}
	c.Source = "voice"
	c.RegisteredAt = time.Now().Format(time.RFC3339)
	path := filepath.Join(r.dir, c.Name+".contract.json")
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return fmt.Errorf("创建契约目录失败: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化契约失败: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("写契约临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("提交契约文件失败: %w", err)
	}
	r.Contracts[c.Name] = c
	return nil
}
